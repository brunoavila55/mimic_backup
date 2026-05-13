package vendors

import (
	"strings"
)

type UbiquitiDriver struct{}

func init() {
	Register("ubiquiti", &UbiquitiDriver{})
	Register("ubiquiti_airos", &UbiquitiDriver{})
}

func (d *UbiquitiDriver) GetBackupCommand() string {
	return "cat /tmp/system.cfg"
}

func (d *UbiquitiDriver) NormalizeConfig(raw string) string {
	return strings.TrimSpace(raw)
}
