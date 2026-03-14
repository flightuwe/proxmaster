package store

import (
	"testing"

	"proxmaster/backend/internal/models"
)

func TestPlanAndExecuteRebuildAllPools(t *testing.T) {
	s := NewMemoryStore()
	plan := s.PlanRebuildAllPools()
	if plan.ID == "" || len(plan.PoolNames) == 0 {
		t.Fatal("expected non-empty rebuild plan")
	}
	executed, ok := s.ExecuteRebuildAllPools(plan.ID)
	if !ok {
		t.Fatal("expected rebuild plan execution to succeed")
	}
	if executed.ID != plan.ID {
		t.Fatalf("expected same plan id, got %s != %s", executed.ID, plan.ID)
	}
}

func TestBackupPolicyExplainPriorityOverride(t *testing.T) {
	s := NewMemoryStore()
	_ = s.UpsertBackupPolicy(models.BackupPolicy{
		WorkloadID:   "101",
		WorkloadKind: "vm",
		Priority:     100,
		TargetID:     "target-pbs-1",
	})
	override := s.UpsertBackupPolicy(models.BackupPolicy{
		WorkloadID:   "101",
		WorkloadKind: "vm",
		Priority:     10,
		Override:     true,
		TargetID:     "target-s3-1",
	})
	p, _, ok := s.ExplainBackupPolicy("101")
	if !ok {
		t.Fatal("expected policy selection")
	}
	if p.ID != override.ID {
		t.Fatalf("expected override policy, got %s", p.ID)
	}
}
