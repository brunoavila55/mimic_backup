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

func TestExtractBlockMikroTikMenu(t *testing.T) {
	config := `/system ntp client
set enabled=yes primary-ntp=1.1.1.1
/ip service
set telnet disabled=yes`

	ctxRe, _ := getCompiledRegex(`/system ntp client`)
	loc := ctxRe.FindStringIndex(config)

	extracted := extractBlock(config, loc[0])
	expected := `/system ntp client
set enabled=yes primary-ntp=1.1.1.1`

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

func TestEvaluateRule(t *testing.T) {
	tests := []struct {
		name   string
		rule   models.SecurityRule
		config string
		want   bool
	}{
		{"forbidden pattern found", models.SecurityRule{Name: "telnet", MatchType: "contains", RegexPattern: `(?m)^telnet server enable$`, Penalty: 10}, "telnet server enable", true},
		{"required pattern present", models.SecurityRule{Name: "ntp", MatchType: "not_contains", RegexPattern: `(?m)^ntp server`, Penalty: 10}, "ntp server 1.1.1.1", false},
		{"required context missing", models.SecurityRule{Name: "ssh", MatchType: "not_contains", ContextBlock: `^system services`, RegexPattern: `ssh`, Penalty: 10}, "interfaces {}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateRule(tt.rule, tt.config)
			if err != nil {
				t.Fatal(err)
			}
			if got.Violated != tt.want {
				t.Fatalf("Violated = %v, want %v", got.Violated, tt.want)
			}
		})
	}
}

func TestValidateRuleRejectsInvalidInput(t *testing.T) {
	rule := models.SecurityRule{Name: "bad", MatchType: "contains", RegexPattern: "[", Penalty: 10}
	if err := ValidateRule(rule); err == nil {
		t.Fatal("expected invalid regex to be rejected")
	}
}

func TestValidateRuleRejectsBlankPattern(t *testing.T) {
	rule := models.SecurityRule{Name: "bad", MatchType: "contains", RegexPattern: "   ", Penalty: 10}
	if err := ValidateRule(rule); err == nil {
		t.Fatal("expected blank regex to be rejected")
	}
}
