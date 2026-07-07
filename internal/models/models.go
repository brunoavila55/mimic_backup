package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Username string `gorm:"uniqueIndex"`
	Password string
	Email    string
	Role     string // Administrator, Viewer
	Avatar   string
}

type Credential struct {
	gorm.Model
	Name     string
	Username string
	Password string // encrypted via AES-GCM
	Port     int    `gorm:"default:22"`
	Nodes    []Node `gorm:"foreignKey:CredentialID"`
}

type BackupRoutine struct {
	gorm.Model
	Name        string
	Description string
	Frequency   string // "1", "6", "12", "24", "168"
	BackupHour  string // "HH:MM"
	BackupDay   string // "0"-"6" (Monday-Sunday)
	Enabled     bool   `gorm:"default:true"`
	Nodes       []Node `gorm:"foreignKey:RoutineID"`
}

type AccessAgent struct {
	gorm.Model
	Name     string
	Username string
	Password string
	Nodes    []Node `gorm:"foreignKey:AccessAgentID"`
}

type Node struct {
	gorm.Model
	Name                 string
	Vendor               string
	IP                   string
	AccessAgentID        *uint
	AccessAgent          *AccessAgent
	CredentialID         *uint
	Credential           *Credential
	Username             string
	Password             string
	Port                 int    `gorm:"default:22"`
	Enabled              bool   `gorm:"default:true"`
	Group                string `gorm:"default:'General'"`
	Tags                 string
	ScheduleType         string `gorm:"default:'individual'"` // 'individual', 'routine'
	RoutineID            *uint
	Routine              *BackupRoutine
	Frequency            string `gorm:"default:'24'"`
	BackupHour           string
	BackupDay            string
	LastStatus           string `gorm:"default:'never';index"`
	LastError            string
	LastBackupAt         *time.Time `gorm:"index"`
	NextBackupAt         *time.Time `gorm:"index"`
	IsOnline             bool       `gorm:"default:false"`
	VerifyHostKey        bool       `gorm:"default:true"`
	SSHPublicFingerprint string
	SSHPrivateKey        string
	AlertSnoozeUntil     *time.Time `gorm:"index"`
	SecurityScore        int        `gorm:"default:100;index"`
	Backups              []NodeBackup `gorm:"foreignKey:NodeID"`
	Violations           []SecurityViolation `gorm:"foreignKey:NodeID"`
	Exceptions           []NodeRuleException `gorm:"foreignKey:NodeID"`
}

func (n *Node) IsSnoozed() bool {
	return n.AlertSnoozeUntil != nil && time.Now().Before(*n.AlertSnoozeUntil)
}

type NodeBackup struct {
	gorm.Model
	NodeID    uint
	Node      Node
	Version   int
	Config    string
	Hash      string
	Status        string
	Error         string
	Exported      bool `gorm:"default:false;index"`
	DiffAdditions int  `gorm:"default:0"`
	DiffDeletions int  `gorm:"default:0"`
	CreatedAt     time.Time
}

type SftpSettings struct {
	gorm.Model
	Host             string
	Port             int `gorm:"default:22"`
	Username         string
	Password         string
	Path             string
	Enabled          bool   `gorm:"default:false"`
	SyncTime         string `gorm:"default:'23:00'"`
	LastExportAt     *time.Time
	LastExportStatus string `gorm:"default:'never'"`
	LastExportError  string
}

type SystemLog struct {
	gorm.Model
	Level    string // info, warning, error, success
	Category string // backup, export, auth, system
	Message  string `json:"message"`
	Details  string
}

type AlertRule struct {
	gorm.Model
	Name           string `json:"name"`
	TargetGroup    string `json:"target_group" gorm:"default:'Global'"` // 'Global' matches all
	Enabled        bool   `json:"enabled" gorm:"default:true"`
	Provider       string `json:"provider"` // "webhook" or "telegram"
	WebhookURL     string `json:"webhook_url"`
	TelegramToken  string `json:"telegram_token"`
	TelegramChatID string `json:"telegram_chat_id"`
	AlertOnDiff    bool   `json:"alert_on_diff"`
	AlertOnFailure bool   `json:"alert_on_failure"`
	AlertOnSecurity bool  `json:"alert_on_security"`
}

type SecurityRule struct {
	gorm.Model
	Name         string `json:"name"`
	Description  string `json:"description"`
	Vendor       string `json:"vendor" gorm:"default:'*'"` // '*' for any vendor
	RegexPattern string `json:"regex_pattern"`
	ContextBlock string `json:"context_block"` // Optional regex to scope the search block
	MatchType    string `json:"match_type" gorm:"default:'contains'"`
	Penalty      int    `json:"penalty" gorm:"default:10"`
	Severity     string `json:"severity" gorm:"default:'Warning'"` // Critical, Warning, Info
}

type GoldenConfig struct {
	gorm.Model
	Name           string `json:"name"`
	Vendor         string `json:"vendor" gorm:"default:'*'"`
	TargetGroup    string `json:"target_group" gorm:"default:'*'"`
	ConfigTemplate string `json:"config_template"`
}

type SecurityViolation struct {
	gorm.Model
	NodeID        uint
	Node          Node
	RuleID        uint
	Rule          SecurityRule
	BackupVersion int
}

type NodeRuleException struct {
	gorm.Model
	NodeID uint `gorm:"uniqueIndex:idx_node_rule"`
	Node   Node
	RuleID uint `gorm:"uniqueIndex:idx_node_rule"`
	Rule   SecurityRule
	Reason string
}
