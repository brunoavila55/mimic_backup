package vendors

import (
	"regexp"
	"strings"
)

type HuaweiDriver struct{}

func init() {
	Register("huawei", &HuaweiDriver{})
}

func (d *HuaweiDriver) GetPrepCommands() []string {
	return []string{"screen-length 0 temporary"}
}

func (d *HuaweiDriver) GetBackupCommand() string {
	return "display current-configuration"
}

func (d *HuaweiDriver) NormalizeConfig(raw string) string {
	config := strings.TrimSpace(raw)
	re := regexp.MustCompile(`(?m)^display current-configuration.*\r?\n?`)
	config = re.ReplaceAllString(config, "")
	return strings.TrimSpace(config)
}

func (d *HuaweiDriver) RequiresPTY() bool {
	return true
}
