package scheduler

import "testing"

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
