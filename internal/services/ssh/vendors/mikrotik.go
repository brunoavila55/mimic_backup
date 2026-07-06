package vendors

import (
	"regexp"
	"strings"
)

type MikroTikDriver struct{}

func init() {
	Register("mikrotik", &MikroTikDriver{})
	Register("mikrotik_routeros", &MikroTikDriver{})
}

func (d *MikroTikDriver) GetPrepCommands() []string {
	return nil
}

func (d *MikroTikDriver) GetBackupCommand() string {
	return "/export"
}

func (d *MikroTikDriver) NormalizeConfig(raw string) string {
	config := strings.TrimSpace(raw)
	re := regexp.MustCompile(`(?m)^#.*(?:by RouterOS|software id|model =|serial number|build-time:).*\r?\n?`)
	config = re.ReplaceAllString(config, "")
	return strings.TrimSpace(config)
}

func (d *MikroTikDriver) RequiresPTY() bool {
	return false
}
