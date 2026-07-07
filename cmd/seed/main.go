package main

import (
	"log"
	"mimic/internal/models"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=123456 dbname=mimic_db port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	rules := []models.SecurityRule{
		// Authentication / Access
		{Name: "SNMP Community Pública (RO)", Vendor: "*", Severity: "Warning", MatchType: "contains", RegexPattern: `community\s+public`, Penalty: 10, Description: "Detects if SNMP community is set to public"},
		{Name: `SNMP Community "private"`, Vendor: "*", Severity: "Critical", MatchType: "contains", RegexPattern: `community\s+private`, Penalty: 25, Description: "Detects if SNMP community is set to private"},
		{Name: "Telnet Habilitado (MikroTik)", Vendor: "mikrotik", Severity: "Critical", MatchType: "contains", RegexPattern: `/ip service.*telnet.*disabled=no`, Penalty: 20, Description: "Detects if Telnet service is enabled on MikroTik"},
		{Name: "Telnet Habilitado (Cisco)", Vendor: "cisco", Severity: "Critical", MatchType: "contains", RegexPattern: `transport input.*telnet`, Penalty: 20, Description: "Detects if Telnet is enabled on Cisco VTY lines"},
		{Name: "Telnet Habilitado (Huawei)", Vendor: "huawei", Severity: "Critical", MatchType: "contains", RegexPattern: `telnet server enable`, Penalty: 20, Description: "Detects if Telnet server is enabled on Huawei"},
		{Name: "Telnet Habilitado (Juniper)", Vendor: "juniper", Severity: "Critical", MatchType: "contains", RegexPattern: `set system services telnet`, Penalty: 20, Description: "Detects if Telnet service is enabled on Juniper"},
		{Name: "Senha em Texto Plano (Cisco)", Vendor: "cisco", Severity: "Critical", MatchType: "contains", RegexPattern: `password 0 `, Penalty: 25, Description: "Detects passwords stored in plain text"},
		{Name: "Enable Password Fraco (Cisco)", Vendor: "cisco", Severity: "Warning", MatchType: "contains", RegexPattern: `enable password`, Penalty: 15, Description: "Detects use of enable password instead of enable secret"},
		{Name: "Senha não Criptografada (Juniper)", Vendor: "juniper", Severity: "Critical", MatchType: "contains", RegexPattern: `set system login user .* authentication plain-text-password`, Penalty: 25, Description: "Detects plain-text authentication configured for a user"},
		{Name: "SNMP RW Habilitado (Juniper)", Vendor: "juniper", Severity: "Critical", MatchType: "contains", RegexPattern: `set snmp community .* authorization read-write`, Penalty: 25, Description: "Detects SNMP Read-Write communities"},
		{Name: "HTTP Admin sem HTTPS (MikroTik)", Vendor: "mikrotik", Severity: "Warning", MatchType: "contains", RegexPattern: `/ip service.*www.*disabled=no`, Penalty: 10, Description: "Detects HTTP admin interface enabled"},
		{Name: "HTTP Server Ativo (Cisco)", Vendor: "cisco", Severity: "Warning", MatchType: "contains", RegexPattern: `ip http server`, Penalty: 10, Description: "Detects HTTP server enabled on Cisco"},
		{Name: "API/Winbox sem Restrição de IP (MikroTik)", Vendor: "mikrotik", Severity: "Warning", MatchType: "not_contains", RegexPattern: `/ip service.*api.*address=`, Penalty: 15, Description: "Detects API access without IP restrictions"},

		// Network / Exposure
		{Name: "VTY sem ACL (Cisco)", Vendor: "cisco", Severity: "Critical", MatchType: "not_contains", RegexPattern: `line vty.*access-class`, Penalty: 20, Description: "Detects VTY lines without access-class restrictions"},
		{Name: "SNMP v1/v2c em Uso", Vendor: "*", Severity: "Warning", MatchType: "contains", RegexPattern: `snmp-server community|community.*ro|community.*rw`, Penalty: 10, Description: "Detects usage of older SNMP versions"},
		{Name: "Firewall Input sem Drop Final (MikroTik)", Vendor: "mikrotik", Severity: "Critical", MatchType: "not_contains", RegexPattern: `chain=input.*action=drop`, Penalty: 20, Description: "Detects absence of a final drop rule on the input chain"},

		// Logging / Auditing
		{Name: "Sem Syslog Remoto", Vendor: "*", Severity: "Warning", MatchType: "not_contains", RegexPattern: `logging \d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|logging trap`, Penalty: 15, Description: "Detects if remote syslog is not configured"},
		{Name: "Sem NTP Configurado", Vendor: "*", Severity: "Warning", MatchType: "not_contains", RegexPattern: `ntp server|ntp-server|set system ntp server`, Penalty: 10, Description: "Detects if NTP server is not configured"},
		{Name: "SNMP Community Padrão Huawei", Vendor: "huawei", Severity: "Warning", MatchType: "contains", RegexPattern: `snmp-agent community read public`, Penalty: 10, Description: "Detects default public read community on Huawei"},
	}

	for _, rule := range rules {
		var existing models.SecurityRule
		if err := db.Where("name = ?", rule.Name).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := db.Create(&rule).Error; err != nil {
					log.Printf("Failed to create rule %s: %v", rule.Name, err)
				} else {
					log.Printf("Created rule: %s", rule.Name)
				}
			} else {
				log.Printf("Error checking for rule %s: %v", rule.Name, err)
			}
		} else {
			log.Printf("Rule already exists: %s", rule.Name)
		}
	}

	log.Println("Seed completed successfully.")
}
