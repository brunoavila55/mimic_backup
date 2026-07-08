package audit

import (
	"fmt"
	"log"
	"mimic/internal/models"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// Evaluation is the deterministic result of applying one rule to a configuration.
// Keeping this logic outside RunAudit ensures previews and tests use the same engine.
type Evaluation struct {
	Violated       bool
	PatternMatched bool
	ContextFound   bool
}

func ValidateRule(rule models.SecurityRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	if rule.MatchType != "contains" && rule.MatchType != "not_contains" {
		return fmt.Errorf("invalid match condition")
	}
	if rule.Penalty < 0 || rule.Penalty > 100 {
		return fmt.Errorf("penalty must be between 0 and 100")
	}
	if _, err := regexp.Compile(rule.RegexPattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	if rule.ContextBlock != "" {
		if _, err := regexp.Compile(rule.ContextBlock); err != nil {
			return fmt.Errorf("invalid context regex: %w", err)
		}
	}
	return nil
}

func EvaluateRule(rule models.SecurityRule, configText string) (Evaluation, error) {
	if err := ValidateRule(rule); err != nil {
		return Evaluation{}, err
	}

	result := Evaluation{ContextFound: true}
	targetText := configText
	if rule.ContextBlock != "" {
		ctxRe, _ := getCompiledRegex(rule.ContextBlock)
		loc := ctxRe.FindStringIndex(configText)
		if loc == nil {
			result.ContextFound = false
			targetText = ""
		} else {
			targetText = extractBlock(configText, loc[0])
		}
	}

	re, _ := getCompiledRegex(rule.RegexPattern)
	result.PatternMatched = re.MatchString(targetText)
	if rule.MatchType == "not_contains" {
		result.Violated = !result.PatternMatched
	} else {
		result.Violated = result.PatternMatched
	}
	return result, nil
}

var (
	regexCache = make(map[string]*regexp.Regexp)
	regexMutex sync.RWMutex
)

func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	regexMutex.RLock()
	re, exists := regexCache[pattern]
	regexMutex.RUnlock()

	if exists {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	regexMutex.Lock()
	regexCache[pattern] = re
	regexMutex.Unlock()

	return re, nil
}

// extractBlock extracts a chunk of text starting from the match location.
// It stops when it encounters a line that is completely empty, or a line that
// does not start with whitespace (e.g., exiting a block's indentation level).
func extractBlock(config string, startIdx int) string {
	// Find the start of the line where the match occurred
	lineStart := 0
	for i := startIdx; i >= 0; i-- {
		if config[i] == '\n' {
			lineStart = i + 1
			break
		}
	}

	remaining := config[lineStart:]
	lines := strings.Split(remaining, "\n")

	var block []string
	if len(lines) > 0 {
		block = append(block, lines[0]) // Include the first line (the context match)
	}

	// Read subsequent lines until we hit a blank line or a line without leading whitespace
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimRight(line, "\r\t ")

		if trimmed == "" {
			break // Blank line terminates block
		}

		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break // Left the indentation level
		}

		block = append(block, line)
	}

	return strings.Join(block, "\n")
}

// RunAudit evaluates the configuration against all applicable security rules and calculates a security score.
func RunAudit(db *gorm.DB, node *models.Node, backupVersion int, configText string) []models.SecurityViolation {
	var rules []models.SecurityRule
	if err := db.Where("enabled = ? AND (vendor = ? OR vendor = '*') AND (target_group = ? OR target_group = '*')", true, node.Vendor, node.Group).Find(&rules).Error; err != nil {
		return nil
	}

	// Fetch old violations to compute diff
	var oldViolations []models.SecurityViolation
	db.Unscoped().Where("node_id = ?", node.ID).Find(&oldViolations)
	oldViolationMap := make(map[uint]bool)
	for _, v := range oldViolations {
		oldViolationMap[v.RuleID] = true
	}

	var exceptions []models.NodeRuleException
	db.Where("node_id = ?", node.ID).Find(&exceptions)
	exceptionMap := make(map[uint]bool)
	for _, ex := range exceptions {
		exceptionMap[ex.RuleID] = true
	}

	var violations []models.SecurityViolation
	score := 100

	for _, rule := range rules {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Audit] Recovered from panic while evaluating rule %d on node %d: %v", rule.ID, node.ID, r)
				}
			}()

			if exceptionMap[rule.ID] {
				return // Skip whitelisted rules entirely for this node
			}

			result, err := EvaluateRule(rule, configText)
			if err != nil {
				log.Printf("[Audit] Skipping invalid rule %d: %v", rule.ID, err)
				return
			}

			if result.Violated {
				score -= rule.Penalty
				violations = append(violations, models.SecurityViolation{
					NodeID:        node.ID,
					RuleID:        rule.ID,
					BackupVersion: backupVersion,
				})
			}
		}()
	}

	// Keep continuing violations so CreatedAt remains the first detection time.
	activeRuleIDs := make([]uint, 0, len(violations))
	for i := range violations {
		activeRuleIDs = append(activeRuleIDs, violations[i].RuleID)
		var existing models.SecurityViolation
		if err := db.Where("node_id = ? AND rule_id = ?", node.ID, violations[i].RuleID).First(&existing).Error; err == nil {
			existing.BackupVersion = backupVersion
			db.Save(&existing)
			violations[i] = existing
		} else {
			db.Create(&violations[i])
		}
	}
	if len(activeRuleIDs) == 0 {
		db.Unscoped().Where("node_id = ?", node.ID).Delete(&models.SecurityViolation{})
	} else {
		db.Unscoped().Where("node_id = ? AND rule_id NOT IN ?", node.ID, activeRuleIDs).Delete(&models.SecurityViolation{})
	}

	if score < 0 {
		score = 0
	}

	node.SecurityScore = score
	db.Save(node)

	// Determine newly introduced violations
	var newViolations []models.SecurityViolation
	for _, v := range violations {
		if !oldViolationMap[v.RuleID] {
			// Populate the Rule object so it can be used in the alert message
			for _, r := range rules {
				if r.ID == v.RuleID {
					v.Rule = r
					break
				}
			}
			newViolations = append(newViolations, v)
		}
	}

	return newViolations
}
