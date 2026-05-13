package vendors

import (
	"regexp"
	"strings"
)

type JuniperDriver struct{}

func init() {
	Register("juniper", &JuniperDriver{})
	Register("juniper_junos", &JuniperDriver{})
}

func (d *JuniperDriver) GetBackupCommand() string {
	return "show configuration"
}

func (d *JuniperDriver) NormalizeConfig(raw string) string {
	config := strings.TrimSpace(raw)
	re := regexp.MustCompile(`(?m)^##.*\r?\n?`)
	config = re.ReplaceAllString(config, "")
	return strings.TrimSpace(config)
}
