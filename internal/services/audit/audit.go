package audit

import (
	"mimic/internal/models"
	"log"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm"
)

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
	if err := db.Where("vendor = ? OR vendor = '*'", node.Vendor).Find(&rules).Error; err != nil {
		return nil
	}

	// Fetch old violations to compute diff
	var oldViolations []models.SecurityViolation
	db.Unscoped().Where("node_id = ?", node.ID).Find(&oldViolations)
	oldViolationMap := make(map[uint]bool)
	for _, v := range oldViolations {
		oldViolationMap[v.RuleID] = true
	}

	// Clear previous violations for this node
	// Unscoped because we don't want soft-deletes lingering around for violations (or at least, we hard delete them to keep it clean)
	db.Unscoped().Where("node_id = ?", node.ID).Delete(&models.SecurityViolation{})

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

			re, err := getCompiledRegex(rule.RegexPattern)
			if err != nil || rule.RegexPattern == "" {
				return
			}

			targetText := configText
			contextMissing := false

			// If ContextBlock is provided, scope the targetText down to that block
			if rule.ContextBlock != "" {
				ctxRe, err := getCompiledRegex(rule.ContextBlock)
				if err == nil {
					loc := ctxRe.FindStringIndex(configText)
					if loc != nil {
						targetText = extractBlock(configText, loc[0])
					} else {
						// Context block is completely missing from the config
						targetText = ""
						contextMissing = true
					}
				}
			}

			triggerViolation := false

			if contextMissing {
				// User confirmation applied: if the block is missing, the pattern is inherently not found.
				// For 'contains' -> no match -> no violation.
				// For 'not_contains' -> no match -> IS A VIOLATION (e.g. NTP not configured at all).
				if rule.MatchType == "not_contains" {
					triggerViolation = true
				}
			} else {
				matched := re.MatchString(targetText)
				if rule.MatchType == "not_contains" && !matched {
					triggerViolation = true
				} else if (rule.MatchType == "contains" || rule.MatchType == "") && matched {
					triggerViolation = true
				}
			}

			if triggerViolation {
				score -= rule.Penalty
				violations = append(violations, models.SecurityViolation{
					NodeID:        node.ID,
					RuleID:        rule.ID,
					BackupVersion: backupVersion,
				})
			}
		}()
	}

	if score < 0 {
		score = 0
	}

	if len(violations) > 0 {
		db.Create(&violations)
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
