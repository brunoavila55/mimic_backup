package vendors

import (
	"regexp"
	"strings"
)

type CiscoDriver struct{}

func init() {
	Register("cisco", &CiscoDriver{})
	Register("cisco_ios", &CiscoDriver{})
	Register("cisco_nxos", &CiscoDriver{})
}

func (d *CiscoDriver) GetPrepCommands() []string {
	return []string{"terminal length 0"}
}

func (d *CiscoDriver) GetBackupCommand() string {
	return "show running-config"
}

func (d *CiscoDriver) NormalizeConfig(raw string) string {
	config := strings.TrimSpace(raw)
	re := regexp.MustCompile(`(?m)^! (?:Last configuration change at|NVRAM config last updated at).*\r?\n?`)
	config = re.ReplaceAllString(config, "")
	return strings.TrimSpace(config)
}

func (d *CiscoDriver) RequiresPTY() bool {
	return true
}
