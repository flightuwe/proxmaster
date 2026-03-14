package mcp

import (
	"context"
	"testing"

	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/store"
)

func TestHandleCallHardBlock(t *testing.T) {
	st := store.NewMemoryStore()
	svc := NewService(st, risk.NewEngine(), policy.NewGate(), orchestrator.New(proxmox.NewClient(st)))

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
	if resp.Job.Status != models.JobBlocked {
		t.Fatalf("expected blocked job, got %s", resp.Job.Status)
	}
}

func TestHandleCallApproved(t *testing.T) {
	st := store.NewMemoryStore()
	svc := NewService(st, risk.NewEngine(), policy.NewGate(), orchestrator.New(proxmox.NewClient(st)))

	resp, err := svc.HandleCall(context.Background(), models.MCPCallRequest{
		Tool:       "network.apply",
		Params:     map[string]any{"name": "vmbr1", "kind": "bridge", "cidr": "10.20.0.0/24"},
		Actor:      "tester",
		ApproveNow: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Job.Status != models.JobSucceeded {
		t.Fatalf("expected succeeded job, got %s", resp.Job.Status)
	}
}