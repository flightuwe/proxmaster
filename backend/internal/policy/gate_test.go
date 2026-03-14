package policy

import (
	"testing"

	"proxmaster/backend/internal/models"
)

func TestGateEvaluate(t *testing.T) {
	g := NewGate()
	allow := g.Evaluate(models.RiskMedium, false, "", false)
	if !allow.Allow {
		t.Fatal("expected medium risk to be allowed")
	}
	blocked := g.Evaluate(models.RiskHigh, true, "need approval", false)
	if blocked.Allow || !blocked.NeedsReview {
		t.Fatal("expected hard block to require review")
	}
}