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
	if got := e.Classify("proxmox.connection.test", nil); got != models.RiskLow {
		t.Fatalf("expected LOW for proxmox.connection.test, got %s", got)
	}
	if got := e.Classify("gitops.status", nil); got != models.RiskLow {
		t.Fatalf("expected LOW for gitops.status, got %s", got)
	}
	if got := e.Classify("vpn.wireguard.status", nil); got != models.RiskLow {
		t.Fatalf("expected LOW for vpn.wireguard.status, got %s", got)
	}
	if got := e.Classify("vm.migrate", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM, got %s", got)
	}
	if got := e.Classify("gitops.sync.now", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM for gitops.sync.now, got %s", got)
	}
	if got := e.Classify("gitops.rollback", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM for gitops.rollback, got %s", got)
	}
	if got := e.Classify("vpn.wireguard.plan", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM for vpn.wireguard.plan, got %s", got)
	}
	if got := e.Classify("network.apply", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH, got %s", got)
	}
	if got := e.Classify("ssh.breakglass.enable", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH for ssh.breakglass.enable, got %s", got)
	}
	if got := e.Classify("vpn.wireguard.apply", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH for vpn.wireguard.apply, got %s", got)
	}
	if got := e.Classify("vm.create", nil); got != models.RiskMedium {
		t.Fatalf("expected MEDIUM for vm.create, got %s", got)
	}
	if got := e.Classify("proxmaster.self_migrate", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH for proxmaster.self_migrate, got %s", got)
	}
	if got := e.Classify("storage.pool.rebuild_all.execute", nil); got != models.RiskHigh {
		t.Fatalf("expected HIGH for rebuild all execute, got %s", got)
	}
}
