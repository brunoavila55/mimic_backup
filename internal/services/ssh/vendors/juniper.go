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

func (d *JuniperDriver) GetPrepCommands() []string {
	return nil
}

func (d *JuniperDriver) GetBackupCommand() string {
	return "show configuration | no-more"
}

func (d *JuniperDriver) NormalizeConfig(raw string) string {
	config := strings.TrimSpace(raw)
	re := regexp.MustCompile(`(?m)^##.*\r?\n?`)
	config = re.ReplaceAllString(config, "")
	return strings.TrimSpace(config)
}

func (d *JuniperDriver) RequiresPTY() bool {
	return false
}
