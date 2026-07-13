package handlers

import (
	"fmt"
	"log"
	"mimic/internal/models"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const (
	silentThresholdHours    = 48
	dashboardAttentionLimit = 6
)

type DashboardHandler struct {
	DB *gorm.DB
}

type DashboardStats struct {
	TotalNodes           int64
	ActiveNodes          int64
	HealthyNodes         int64
	FailedNodes          int64
	SilentNodes          int64
	AttentionNodes       int64
	ScheduledNodes       int64
	SecurityRiskNodes    int64
	AverageSecurityScore int64
	PendingExportNodes   int64
	Backups24h           int64
	Successful24h        int64
	Failed24h            int64
	Changes24h           int64
	HealthRate           int64
	SuccessRate24h       int64
	ScheduleCoverage     int64
}

type DashboardAttentionItem struct {
	Node     models.Node
	Severity string
	Label    string
	Title    string
	Detail   string
	Context  string
}

type DashboardTrendDay struct {
	Label         string
	Date          string
	Successful    int64
	Failed        int64
	Total         int64
	SuccessRate   int64
	SuccessHeight int64
	FailureHeight int64
}

type DashboardSFTPStatus struct {
	Configured bool
	Enabled    bool
	State      string
	Tone       string
	Detail     string
}

type DashboardSnapshot struct {
	Stats            DashboardStats
	Attention        []DashboardAttentionItem
	Trend            []DashboardTrendDay
	Upcoming         []models.Node
	RecentChanges    []models.NodeBackup
	RecentActivity   []models.SystemLog
	SFTP             DashboardSFTPStatus
	LastSuccessfulAt *time.Time
	UpdatedAt        time.Time
}

type dashboardNodeAggregate struct {
	TotalNodes           int64
	ActiveNodes          int64
	HealthyNodes         int64
	FailedNodes          int64
	SilentNodes          int64
	ScheduledNodes       int64
	SecurityRiskNodes    int64
	AverageSecurityScore int64
}

type dashboardBackupAggregate struct {
	Backups24h    int64 `gorm:"column:backups_24h"`
	Successful24h int64 `gorm:"column:successful_24h"`
	Failed24h     int64 `gorm:"column:failed_24h"`
	Changes24h    int64 `gorm:"column:changes_24h"`
}

type dashboardTrendRow struct {
	Day    string
	Status string
	Count  int64
}

func percent(part, total int64) int64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	value := (part*100 + total/2) / total
	if value > 100 {
		return 100
	}
	return value
}

func dashboardAttentionItem(node models.Node, now time.Time) DashboardAttentionItem {
	item := DashboardAttentionItem{Node: node, Severity: "warning", Label: "Stale", Title: "Backup coverage is stale"}
	if node.LastStatus == "error" {
		item.Severity = "danger"
		item.Label = "Failed"
		item.Title = "Latest backup failed"
		item.Detail = strings.TrimSpace(node.LastError)
		if item.Detail == "" {
			item.Detail = "The latest backup did not complete successfully."
		}
	} else if node.LastBackupAt == nil {
		item.Label = "No backup"
		item.Title = "First backup still pending"
		item.Detail = "This active node has not produced a successful configuration snapshot."
	} else {
		item.Detail = fmt.Sprintf("Last successful backup was %s ago.", shortDuration(now.Sub(*node.LastBackupAt)))
	}

	item.Context = node.Vendor
	if strings.TrimSpace(node.Group) != "" {
		item.Context += " · " + node.Group
	}
	return item
}

func shortDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	hours := int(duration.Hours())
	if hours < 1 {
		minutes := int(duration.Minutes())
		if minutes < 1 {
			return "less than a minute"
		}
		return fmt.Sprintf("%dm", minutes)
	}
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}

func buildDashboardTrend(now time.Time, rows []dashboardTrendRow) []DashboardTrendDay {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
	days := make([]DashboardTrendDay, 7)
	dayIndex := make(map[string]int, 7)
	for i := range days {
		day := start.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		dayIndex[key] = i
		days[i] = DashboardTrendDay{Label: day.Format("Mon"), Date: day.Format("02 Jan")}
	}

	for _, row := range rows {
		index, ok := dayIndex[row.Day]
		if !ok {
			continue
		}
		switch row.Status {
		case "success":
			days[index].Successful += row.Count
		case "error":
			days[index].Failed += row.Count
		}
	}

	var maxCount int64
	for i := range days {
		days[i].Total = days[i].Successful + days[i].Failed
		days[i].SuccessRate = percent(days[i].Successful, days[i].Total)
		if days[i].Total > maxCount {
			maxCount = days[i].Total
		}
	}
	if maxCount == 0 {
		return days
	}
	for i := range days {
		days[i].SuccessHeight = percent(days[i].Successful, maxCount)
		days[i].FailureHeight = percent(days[i].Failed, maxCount)
		if days[i].Successful > 0 && days[i].SuccessHeight < 6 {
			days[i].SuccessHeight = 6
		}
		if days[i].Failed > 0 && days[i].FailureHeight < 6 {
			days[i].FailureHeight = 6
		}
	}
	return days
}

func (h *DashboardHandler) loadDashboard(now time.Time) (DashboardSnapshot, error) {
	var snapshot DashboardSnapshot
	snapshot.UpdatedAt = now
	threshold := now.Add(-silentThresholdHours * time.Hour)
	last24h := now.Add(-24 * time.Hour)
	trendStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)

	var nodes dashboardNodeAggregate
	if err := h.DB.Model(&models.Node{}).Select(`
		COUNT(*) AS total_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE) AS active_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE AND last_status = 'success' AND last_backup_at >= ?) AS healthy_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE AND last_status = 'error') AS failed_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE AND (last_status IS NULL OR last_status <> 'error') AND (last_backup_at IS NULL OR last_backup_at < ?)) AS silent_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE AND next_backup_at > ?) AS scheduled_nodes,
		COUNT(*) FILTER (WHERE enabled = TRUE AND security_score < 70) AS security_risk_nodes,
		COALESCE(ROUND(AVG(security_score) FILTER (WHERE enabled = TRUE)), 0) AS average_security_score
	`, threshold, threshold, now).Scan(&nodes).Error; err != nil {
		return snapshot, fmt.Errorf("load fleet summary: %w", err)
	}

	var backups dashboardBackupAggregate
	if err := h.DB.Model(&models.NodeBackup{}).Select(`
		COUNT(*) FILTER (WHERE created_at >= ?) AS backups_24h,
		COUNT(*) FILTER (WHERE created_at >= ? AND status = 'success') AS successful_24h,
		COUNT(*) FILTER (WHERE created_at >= ? AND status = 'error') AS failed_24h,
		COUNT(*) FILTER (WHERE created_at >= ? AND status = 'success' AND (diff_additions > 0 OR diff_deletions > 0)) AS changes_24h
	`, last24h, last24h, last24h, last24h).Scan(&backups).Error; err != nil {
		return snapshot, fmt.Errorf("load backup summary: %w", err)
	}

	snapshot.Stats = DashboardStats{
		TotalNodes:           nodes.TotalNodes,
		ActiveNodes:          nodes.ActiveNodes,
		HealthyNodes:         nodes.HealthyNodes,
		FailedNodes:          nodes.FailedNodes,
		SilentNodes:          nodes.SilentNodes,
		AttentionNodes:       nodes.FailedNodes + nodes.SilentNodes,
		ScheduledNodes:       nodes.ScheduledNodes,
		SecurityRiskNodes:    nodes.SecurityRiskNodes,
		AverageSecurityScore: nodes.AverageSecurityScore,
		Backups24h:           backups.Backups24h,
		Successful24h:        backups.Successful24h,
		Failed24h:            backups.Failed24h,
		Changes24h:           backups.Changes24h,
		HealthRate:           percent(nodes.HealthyNodes, nodes.ActiveNodes),
		SuccessRate24h:       percent(backups.Successful24h, backups.Backups24h),
		ScheduleCoverage:     percent(nodes.ScheduledNodes, nodes.ActiveNodes),
	}

	if err := h.DB.Model(&models.NodeBackup{}).
		Where("status = ? AND exported = ?", "success", false).
		Distinct("node_id").
		Count(&snapshot.Stats.PendingExportNodes).Error; err != nil {
		return snapshot, fmt.Errorf("load export queue: %w", err)
	}

	var attentionNodes []models.Node
	if err := h.DB.Where(`enabled = TRUE AND (
		last_status = 'error' OR
		((last_status IS NULL OR last_status <> 'error') AND (last_backup_at IS NULL OR last_backup_at < ?))
	)`, threshold).
		Order("CASE WHEN last_status = 'error' THEN 0 ELSE 1 END").
		Order("last_backup_at ASC NULLS FIRST").
		Limit(dashboardAttentionLimit).
		Find(&attentionNodes).Error; err != nil {
		return snapshot, fmt.Errorf("load attention queue: %w", err)
	}
	for _, node := range attentionNodes {
		snapshot.Attention = append(snapshot.Attention, dashboardAttentionItem(node, now))
	}

	if err := h.DB.Where("enabled = ? AND next_backup_at > ?", true, now).
		Order("next_backup_at ASC").Limit(5).Find(&snapshot.Upcoming).Error; err != nil {
		return snapshot, fmt.Errorf("load upcoming backups: %w", err)
	}

	if err := h.DB.Preload("Node").
		Where("status = ? AND (diff_additions > 0 OR diff_deletions > 0)", "success").
		Order("created_at DESC").Limit(6).Find(&snapshot.RecentChanges).Error; err != nil {
		return snapshot, fmt.Errorf("load recent changes: %w", err)
	}

	if err := h.DB.Order("created_at DESC").Limit(6).Find(&snapshot.RecentActivity).Error; err != nil {
		return snapshot, fmt.Errorf("load recent activity: %w", err)
	}

	var trendRows []dashboardTrendRow
	timezone := strings.TrimSpace(os.Getenv("TZ"))
	if timezone == "" {
		timezone = "UTC"
	}
	if err := h.DB.Model(&models.NodeBackup{}).
		Select("TO_CHAR(created_at AT TIME ZONE ?, 'YYYY-MM-DD') AS day, status, COUNT(*) AS count", timezone).
		Where("created_at >= ? AND status IN ?", trendStart, []string{"success", "error"}).
		Group("day, status").Order("day ASC").Scan(&trendRows).Error; err != nil {
		return snapshot, fmt.Errorf("load backup trend: %w", err)
	}
	snapshot.Trend = buildDashboardTrend(now, trendRows)

	var lastSuccessful models.NodeBackup
	if err := h.DB.Select("created_at").Where("status = ?", "success").Order("created_at DESC").First(&lastSuccessful).Error; err == nil {
		value := lastSuccessful.CreatedAt
		snapshot.LastSuccessfulAt = &value
	} else if err != gorm.ErrRecordNotFound {
		return snapshot, fmt.Errorf("load last successful backup: %w", err)
	}

	var settings models.SftpSettings
	if err := h.DB.First(&settings).Error; err != nil && err != gorm.ErrRecordNotFound {
		return snapshot, fmt.Errorf("load SFTP status: %w", err)
	}
	configured := strings.TrimSpace(settings.Host) != "" &&
		strings.TrimSpace(settings.Username) != "" &&
		strings.TrimSpace(settings.Password) != ""
	snapshot.SFTP = DashboardSFTPStatus{Configured: configured, Enabled: settings.Enabled}
	switch {
	case !configured:
		snapshot.SFTP.State = "Not configured"
		snapshot.SFTP.Tone = "muted"
		snapshot.SFTP.Detail = "Remote delivery is not set up"
	case !settings.Enabled:
		snapshot.SFTP.State = "Paused"
		snapshot.SFTP.Tone = "warning"
		snapshot.SFTP.Detail = "Configuration exists, automatic sync is disabled"
	case settings.LastExportStatus == "error" || settings.LastExportStatus == "partial":
		snapshot.SFTP.State = "Needs review"
		snapshot.SFTP.Tone = "danger"
		snapshot.SFTP.Detail = "The latest remote delivery did not complete"
	default:
		snapshot.SFTP.State = "Operational"
		snapshot.SFTP.Tone = "success"
		snapshot.SFTP.Detail = "Remote delivery is enabled"
	}

	return snapshot, nil
}

func (h *DashboardHandler) GetDashboard(c *fiber.Ctx) error {
	snapshot, err := h.loadDashboard(time.Now())
	data := fiber.Map{
		"Title":        "Dashboard",
		"Username":     c.Locals("username"),
		"Avatar":       c.Locals("avatar"),
		"Role":         c.Locals("role"),
		"CurrentRoute": "dashboard",
		"Dashboard":    snapshot,
	}

	partialRequest := c.Get("HX-Request") == "true" && c.Get("HX-Target") == "dashboard-content"
	if err != nil {
		log.Printf("[Dashboard] Failed to build operations snapshot: %v", err)
		if partialRequest {
			c.Set("HX-Trigger", `{"showNotification":{"message":"Dashboard refresh failed. The previous data was kept.","type":"error"}}`)
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		data["LoadError"] = "Operational data could not be loaded. Refresh the page or review the application logs."
		return c.Status(fiber.StatusInternalServerError).Render("dashboard", data, "base")
	}

	if partialRequest {
		return c.Render("partials/dashboard_stats", data)
	}
	return c.Render("dashboard", data, "base")
}
