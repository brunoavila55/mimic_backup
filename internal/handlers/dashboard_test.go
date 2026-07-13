package handlers

import (
	"mimic/internal/models"
	"testing"
	"time"
)

func TestPercent(t *testing.T) {
	tests := []struct {
		name        string
		part, total int64
		want        int64
	}{
		{name: "zero total", part: 0, total: 0, want: 0},
		{name: "rounded", part: 2, total: 3, want: 67},
		{name: "complete", part: 8, total: 8, want: 100},
		{name: "clamped", part: 12, total: 8, want: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percent(tt.part, tt.total); got != tt.want {
				t.Fatalf("percent(%d, %d) = %d, want %d", tt.part, tt.total, got, tt.want)
			}
		})
	}
}

func TestDashboardAttentionItem(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	lastBackup := now.Add(-72 * time.Hour)

	failure := dashboardAttentionItem(models.Node{
		Name: "edge-01", Vendor: "Cisco", Group: "Core", LastStatus: "error", LastError: "SSH timeout",
	}, now)
	if failure.Severity != "danger" || failure.Label != "Failed" || failure.Detail != "SSH timeout" {
		t.Fatalf("unexpected failure item: %#v", failure)
	}

	stale := dashboardAttentionItem(models.Node{
		Name: "edge-02", Vendor: "Juniper", LastStatus: "success", LastBackupAt: &lastBackup,
	}, now)
	if stale.Severity != "warning" || stale.Label != "Stale" || stale.Detail != "Last successful backup was 3d ago." {
		t.Fatalf("unexpected stale item: %#v", stale)
	}

	never := dashboardAttentionItem(models.Node{Name: "edge-03", Vendor: "MikroTik"}, now)
	if never.Label != "No backup" || never.Title != "First backup still pending" {
		t.Fatalf("unexpected never-backed-up item: %#v", never)
	}
}

func TestBuildDashboardTrend(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	trend := buildDashboardTrend(now, []dashboardTrendRow{
		{Day: "2026-07-12", Status: "success", Count: 8},
		{Day: "2026-07-12", Status: "error", Count: 2},
		{Day: "2026-07-13", Status: "success", Count: 5},
	})
	if len(trend) != 7 {
		t.Fatalf("trend has %d days, want 7", len(trend))
	}
	if trend[5].Total != 10 || trend[5].SuccessRate != 80 {
		t.Fatalf("unexpected previous-day aggregate: %#v", trend[5])
	}
	if trend[6].Total != 5 || trend[6].SuccessRate != 100 || trend[6].SuccessHeight != 50 {
		t.Fatalf("unexpected current-day aggregate: %#v", trend[6])
	}
}
