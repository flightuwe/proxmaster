package policy

import (
	"testing"

	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/models"
)

func TestGateEvaluate(t *testing.T) {
	g := NewGate()
	mode := models.PolicyModeState{Mode: models.PolicyModeGuardedSRE}
	allow := g.Evaluate(models.RiskMedium, false, "", false, true, "", false, "", mode)
	if !allow.Allow {
		t.Fatal("expected medium risk to be allowed")
	}
	blocked := g.Evaluate(models.RiskHigh, true, "need approval", false, true, "", false, "", mode)
	if blocked.Allow || !blocked.NeedsReview {
		t.Fatal("expected hard block to require review")
	}
}

func TestSimulateFailClosed(t *testing.T) {
	g := NewGate()
	d := g.Simulate(models.RiskMedium, false, "", false, health.Snapshot{
		QuorumHealthy: false,
		RunnerHealthy: false,
		OnlineNodes:   1,
		TotalNodes:    4,
	}, models.PolicyModeState{Mode: models.PolicyModeGuardedSRE})
	if d.Allow {
		t.Fatal("expected fail-closed decision")
	}
}
