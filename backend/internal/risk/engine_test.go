package risk

import (
	"testing"

	"proxmaster/backend/internal/models"
)

func TestClassify(t *testing.T) {
	e := NewEngine()
	if got := e.Classify("cluster.get_state", nil); got != models.RiskLow {
		t.Fatalf("expected LOW, got %s", got)
	}
	if got := e.Classify("vm.migrate", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM, got %s", got)
	}
	if got := e.Classify("network.apply", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH, got %s", got)
	}
	if got := e.Classify("vm.create", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM for vm.create, got %s", got)
	}
}
