package audit

import (
	"mimic/internal/models"
	"testing"
)

func TestExtractBlock(t *testing.T) {
	config := `interface GigabitEthernet0/1
 description WAN
 ip address dhcp
!
interface GigabitEthernet0/2
 description LAN`

	ctxRe, _ := getCompiledRegex("interface GigabitEthernet0/1")
	loc := ctxRe.FindStringIndex(config)
	
	extracted := extractBlock(config, loc[0])
	expected := `interface GigabitEthernet0/1
 description WAN
 ip address dhcp`

	if extracted != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, extracted)
	}
}

func TestContextBlockMissing(t *testing.T) {
	// If context block is missing, it should be treated as pattern not found in that block.
	configText := `
/ip address
add address=10.0.0.1/24 interface=ether1
`
	
	// Case 1: MatchType = "contains" (e.g. searching for a vulnerability inside NTP)
	ruleContains := models.SecurityRule{
		ContextBlock: `/system ntp client`,
		RegexPattern: `enabled=yes`,
		MatchType:    "contains",
	}

	// Case 2: MatchType = "not_contains" (e.g. ensuring NTP is configured)
	ruleNotContains := models.SecurityRule{
		ContextBlock: `/system ntp client`,
		RegexPattern: `enabled=yes`,
		MatchType:    "not_contains",
	}

	// We simulate the RunAudit loop for just these rules without DB
	
	evaluate := func(rule models.SecurityRule) bool {
		targetText := configText
		contextMissing := false

		if rule.ContextBlock != "" {
			ctxRe, _ := getCompiledRegex(rule.ContextBlock)
			loc := ctxRe.FindStringIndex(configText)
			if loc != nil {
				targetText = extractBlock(configText, loc[0])
			} else {
				targetText = ""
				contextMissing = true
			}
		}

		triggerViolation := false

		if contextMissing {
			if rule.MatchType == "not_contains" {
				triggerViolation = true
			}
		} else {
			re, _ := getCompiledRegex(rule.RegexPattern)
			matched := re.MatchString(targetText)
			if rule.MatchType == "not_contains" && !matched {
				triggerViolation = true
			} else if (rule.MatchType == "contains" || rule.MatchType == "") && matched {
				triggerViolation = true
			}
		}

		return triggerViolation
	}

	if evaluate(ruleContains) == true {
		t.Errorf("Expected ruleContains to NOT trigger a violation when context block is missing")
	}

	if evaluate(ruleNotContains) == false {
		t.Errorf("Expected ruleNotContains to trigger a violation when context block is missing")
	}
}
