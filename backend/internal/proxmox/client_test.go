package proxmox

import (
	"context"
	"testing"

	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/store"
)

func TestSelfMigrateProxmasterSwitchesEndpoint(t *testing.T) {
	st := store.NewMemoryStore()
	cp := controlplane.NewManager(controlplane.Config{
		Mode:        controlplane.ModeVIP,
		VIP:         "100.100.100.10",
		APIPort:     8080,
		InitialNode: "node-1",
	})
	c := NewClient(st, cp, nil)

	out, err := c.SelfMigrateProxmaster(context.Background(), "100", "node-2", true)
	if err != nil {
		t.Fatal(err)
	}
	handover, ok := out["handover"].(controlplane.SwitchResult)
	if !ok {
		t.Fatalf("expected handover result, got %T", out["handover"])
	}
	if handover.ToNode != "node-2" {
		t.Fatalf("expected switch to node-2, got %s", handover.ToNode)
	}
	if cp.CurrentNode() != "node-2" {
		t.Fatalf("expected control-plane node node-2, got %s", cp.CurrentNode())
	}
}
