package mcp

import (
	"context"
	"testing"

	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/runner"
	"proxmaster/backend/internal/store"
)

func TestHandleCallHardBlock(t *testing.T) {
	st := store.NewMemoryStore()
	svc := NewService(
		st,
		risk.NewEngine(),
		policy.NewGate(),
		health.NewGateEvaluator(true, 120),
		orchestrator.New(proxmox.NewClient(st, controlplane.NewManager(controlplane.Config{Mode: controlplane.ModeVIP, InitialNode: "node-1"}), nil), runner.NewController()),
	)

	resp, err := svc.HandleCall(context.Background(), models.MCPCallRequest{
		Tool:   "network.apply",
		Params: map[string]any{"name": "vmbr1", "kind": "bridge", "cidr": "10.20.0.0/24"},
		Actor:  "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HardBlocked {
		t.Fatal("expected hard blocked response")
	}
	if resp.Job.Status != models.JobPendingApproval {
		t.Fatalf("expected pending approval job, got %s", resp.Job.Status)
	}
}

func TestHandleCallApproved(t *testing.T) {
	st := store.NewMemoryStore()
	svc := NewService(
		st,
		risk.NewEngine(),
		policy.NewGate(),
		health.NewGateEvaluator(true, 120),
		orchestrator.New(proxmox.NewClient(st, controlplane.NewManager(controlplane.Config{Mode: controlplane.ModeVIP, InitialNode: "node-1"}), nil), runner.NewController()),
	)

	resp, err := svc.HandleCall(context.Background(), models.MCPCallRequest{
		Tool:       "network.apply",
		Params:     map[string]any{"name": "vmbr1", "kind": "bridge", "cidr": "10.20.0.0/24"},
		Actor:      "tester",
		ApproveNow: true,
		HardwareMFA: true,
		SecondApprover: "second-admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Job.Status != models.JobCompleted {
		t.Fatalf("expected completed job, got %s", resp.Job.Status)
	}
}

func TestHandleCallIdempotencyKey(t *testing.T) {
	st := store.NewMemoryStore()
	svc := NewService(
		st,
		risk.NewEngine(),
		policy.NewGate(),
		health.NewGateEvaluator(true, 120),
		orchestrator.New(proxmox.NewClient(st, controlplane.NewManager(controlplane.Config{Mode: controlplane.ModeVIP, InitialNode: "node-1"}), nil), runner.NewController()),
	)

	req := models.MCPCallRequest{
		Tool:            "vm.create",
		Params:          map[string]any{"name": "x", "node_id": "node-2"},
		Actor:           "tester",
		IdempotencyKey:  "idem-1",
	}
	first, err := svc.HandleCall(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.HandleCall(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.JobID != second.JobID {
		t.Fatalf("expected same job id for idempotent call, got %s vs %s", first.JobID, second.JobID)
	}
}
