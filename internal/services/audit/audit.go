package audit

import (
	"mimic/internal/models"
	"regexp"

	"gorm.io/gorm"
)

// RunAudit evaluates the configuration against all applicable security rules and calculates a security score.
func RunAudit(db *gorm.DB, node *models.Node, backupVersion int, configText string) {
	var rules []models.SecurityRule
	db.Where("vendor = ? OR vendor = '*'", node.Vendor).Find(&rules)

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
		if exceptionMap[rule.ID] {
			continue // Skip whitelisted rules entirely for this node
		}

		re, err := regexp.Compile(rule.RegexPattern)
		if err != nil || rule.RegexPattern == "" {
			continue
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
	}

	if score < 0 {
		score = 0
	}

	node.SecurityScore = score
	db.Save(node)

	for _, v := range violations {
		db.Create(&v)
	}
}
