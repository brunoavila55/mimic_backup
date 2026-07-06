package audit

import (
	"mimic/internal/models"
	"regexp"
	"log"

	"gorm.io/gorm"
	"sync"
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

			matched := re.MatchString(configText)
			triggerViolation := false

			if rule.MatchType == "not_contains" && !matched {
				triggerViolation = true
			} else if (rule.MatchType == "contains" || rule.MatchType == "") && matched {
				triggerViolation = true
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
