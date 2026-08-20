package scheduler

import (
	"testing"
	"time"
)

func TestNodeLockRejectsConcurrentBackup(t *testing.T) {
	s := &SchedulerService{}
	const nodeID = uint(42)
	if !s.tryLockNode(nodeID) {
		t.Fatal("first lock should succeed")
	}
	if s.tryLockNode(nodeID) {
		t.Fatal("concurrent lock for the same node should fail")
	}
	s.unlockNode(nodeID)
	if !s.tryLockNode(nodeID) {
		t.Fatal("lock should become available after unlock")
	}
	s.unlockNode(nodeID)
}

func TestNodeLocksAreIndependent(t *testing.T) {
	s := &SchedulerService{}
	if !s.tryLockNode(1) || !s.tryLockNode(2) {
		t.Fatal("different nodes should be able to run concurrently")
	}
	s.unlockNode(1)
	s.unlockNode(2)
}

func TestNextRunAfterNoPreferredTimeFallsBackToRollingInterval(t *testing.T) {
	after := time.Date(2026, 3, 10, 9, 15, 0, 0, time.UTC)
	got := nextRunAfter(after, 6, "", "")
	want := after.Add(6 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("expected rolling interval %v, got %v", want, got)
	}
}

func TestNextRunAfterDailyAnchorsToTimeOfDay(t *testing.T) {
	// Before the anchor time today: should run later today.
	before := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	got := nextRunAfter(before, 24, "02:00", "")
	want := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	// After the anchor time today: should roll over to tomorrow.
	after := time.Date(2026, 3, 10, 3, 0, 0, 0, time.UTC)
	got = nextRunAfter(after, 24, "02:00", "")
	want = time.Date(2026, 3, 11, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextRunAfterSubDailyAlignsToSlots(t *testing.T) {
	// hour=02:00, freq=6h -> slots at 02:00, 08:00, 14:00, 20:00.
	after := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	got := nextRunAfter(after, 6, "02:00", "")
	want := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestNextRunAfterWeeklyAnchorsToWeekday(t *testing.T) {
	// backup_day "2" = Wednesday (0=Monday..6=Sunday). 2026-03-10 is a Tuesday.
	after := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	got := nextRunAfter(after, 168, "03:00", "2")
	want := time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC) // next Wednesday
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got.Weekday() != time.Wednesday {
		t.Fatalf("expected Wednesday, got %v", got.Weekday())
	}
}
