package vendors

import (
	"fmt"
	"strings"
)

type Driver interface {
	GetPrepCommands() []string
	GetBackupCommand() string
	NormalizeConfig(raw string) string
}

var registry = make(map[string]Driver)

func Register(vendor string, driver Driver) {
	registry[strings.ToLower(vendor)] = driver
}

func GetDriver(vendor string) (Driver, error) {
	d, ok := registry[strings.ToLower(vendor)]
	if !ok {
		return nil, fmt.Errorf("no ssh driver registered for vendor %s", vendor)
	}
	return d, nil
}
