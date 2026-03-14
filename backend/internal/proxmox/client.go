package proxmox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/store"
)

type Client struct {
	store       store.Store
	controlPlane *controlplane.Manager
}

func NewClient(s store.Store, cp *controlplane.Manager) *Client {
	return &Client{store: s, controlPlane: cp}
}

func (c *Client) GetState(_ context.Context) models.ClusterState {
	return c.store.ClusterState()
}

func (c *Client) SetNodeMaintenance(_ context.Context, nodeID string, maintenance bool) (map[string]any, error) {
	ok := c.store.SetNodeMaintenance(nodeID, maintenance)
	if !ok {
		return nil, errors.New("node not found")
	}
	return map[string]any{"changed": true, "node_id": nodeID, "maintenance": maintenance}, nil
}

func (c *Client) MigrateVM(_ context.Context, vmID, targetNode string) (map[string]any, error) {
	ok := c.store.MigrateVM(vmID, targetNode)
	if !ok {
		return nil, errors.New("vm not found")
	}
	return map[string]any{"changed": true, "vm_id": vmID, "target_node": targetNode}, nil
}

func (c *Client) SelfMigrateProxmaster(_ context.Context, vmID, targetNode string, restartAfter bool) (map[string]any, error) {
	if vmID == "" {
		vmID = "100"
	}
	if targetNode == "" {
		return nil, errors.New("missing target_node")
	}

	state := c.store.ClusterState()
	var vm *models.VM
	for i := range state.VMs {
		if state.VMs[i].ID == vmID {
			vm = &state.VMs[i]
			break
		}
	}
	if vm == nil {
		return nil, errors.New("proxmaster management vm not found")
	}
	if vm.NodeID == targetNode {
		return nil, errors.New("proxmaster already running on target node")
	}

	targetOnline := false
	for _, n := range state.Nodes {
		if n.ID == targetNode && n.Status == "online" {
			targetOnline = true
			break
		}
	}
	if !targetOnline {
		return nil, errors.New("target node is not online")
	}

	ok := c.store.MigrateVM(vmID, targetNode)
	if !ok {
		return nil, errors.New("vm migration failed")
	}
	switchResult := c.controlPlane.SwitchTo(targetNode)

	return map[string]any{
		"changed":                true,
		"action":                 "proxmaster_self_migrate",
		"management_vm_id":       vmID,
		"from_node":              vm.NodeID,
		"to_node":                targetNode,
		"live_migration":         true,
		"restart_after_migrate":  restartAfter,
		"switch_mode":            "seamless_handover",
		"client_reconnect_hint":  "reconnect API client to active control-plane endpoint",
		"handover":               switchResult,
		"completed_at_utc":       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) CreateVM(_ context.Context, name, nodeID string, cpu, memoryMB, diskGB int) (map[string]any, error) {
	if name == "" || nodeID == "" {
		return nil, errors.New("missing name or node_id")
	}
	vm := c.store.CreateVM(models.VM{
		Name:     name,
		NodeID:   nodeID,
		CPU:      cpu,
		MemoryMB: memoryMB,
		DiskGB:   diskGB,
		Priority: 80,
		Kind:     "vm",
	})
	return map[string]any{"changed": true, "vm": vm}, nil
}

func (c *Client) CloneVM(_ context.Context, templateID, newID, targetNode, name string) (map[string]any, error) {
	vm, ok := c.store.CloneVM(templateID, newID, targetNode, name)
	if !ok {
		return nil, errors.New("template vm not found")
	}
	return map[string]any{"changed": true, "vm": vm}, nil
}

func (c *Client) CreateLXC(_ context.Context, name, nodeID string, cpu, memoryMB, diskGB int) (map[string]any, error) {
	if name == "" || nodeID == "" {
		return nil, errors.New("missing name or node_id")
	}
	ct := c.store.CreateLXC(models.VM{
		Name:     name,
		NodeID:   nodeID,
		CPU:      cpu,
		MemoryMB: memoryMB,
		DiskGB:   diskGB,
		Priority: 70,
		Kind:     "lxc",
	})
	return map[string]any{"changed": true, "lxc": ct}, nil
}

func (c *Client) ApplyStoragePool(_ context.Context, name, poolType string) (map[string]any, error) {
	c.store.ApplyPool(name, poolType)
	return map[string]any{"changed": true, "pool_name": name, "pool_type": poolType}, nil
}

func (c *Client) ApplyNetwork(_ context.Context, name, kind, cidr string) (map[string]any, error) {
	c.store.ApplyNetwork(name, kind, cidr)
	return map[string]any{"changed": true, "name": name, "kind": kind, "cidr": cidr}, nil
}

func (c *Client) UpdatesPlan(_ context.Context, strategy string) (map[string]any, error) {
	if strategy == "" {
		strategy = "canary_then_rolling"
	}
	return map[string]any{
		"changed":            false,
		"strategy":           strategy,
		"canary_node":        "node-1",
		"estimated_duration": "45m",
	}, nil
}

func (c *Client) CanaryStart(_ context.Context, nodeID string) (map[string]any, error) {
	if nodeID == "" {
		nodeID = "node-1"
	}
	return map[string]any{
		"changed":       true,
		"status":        "canary_started",
		"node_id":       nodeID,
		"started_at_utc": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) RolloutContinue(_ context.Context) (map[string]any, error) {
	return map[string]any{"changed": true, "status": "rolling_update_continued"}, nil
}

func (c *Client) RolloutAbort(_ context.Context, reason string) (map[string]any, error) {
	if reason == "" {
		reason = "manual abort"
	}
	return map[string]any{"changed": true, "status": "rolling_update_aborted", "reason": reason}, nil
}

func (c *Client) SimulatePolicy(_ context.Context, tool string, risk string) (map[string]any, error) {
	return map[string]any{
		"tool":               tool,
		"risk_level":         risk,
		"recommended_action": "require gate check and approval for high risk",
	}, nil
}

func (c *Client) ExplainPolicy(_ context.Context, tool string) (map[string]any, error) {
	return map[string]any{
		"tool":         tool,
		"policy_scope": "SRE-mode guarded autonomy",
		"details":      fmt.Sprintf("tool %s is evaluated by risk + hard blocks + fail-closed health gates", tool),
	}, nil
}
