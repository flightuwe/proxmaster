package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"proxmaster/backend/internal/proxmox"
)

type Service struct {
	px *proxmox.Client
}

func New(px *proxmox.Client) *Service {
	return &Service{px: px}
}

func (s *Service) Execute(ctx context.Context, tool string, params map[string]any) (map[string]any, error) {
	switch tool {
	case "cluster.get_state":
		state := s.px.GetState(ctx)
		return map[string]any{"state": state}, nil
	case "node.set_maintenance":
		nodeID, _ := params["node_id"].(string)
		maintenance, _ := params["maintenance"].(bool)
		if nodeID == "" {
			return nil, errors.New("missing node_id")
		}
		return s.px.SetNodeMaintenance(ctx, nodeID, maintenance)
	case "vm.migrate":
		vmID, _ := params["vm_id"].(string)
		targetNode, _ := params["target_node"].(string)
		if vmID == "" || targetNode == "" {
			return nil, errors.New("missing vm_id or target_node")
		}
		return s.px.MigrateVM(ctx, vmID, targetNode)
	case "storage.pool.apply":
		name, _ := params["name"].(string)
		poolType, _ := params["type"].(string)
		if name == "" || poolType == "" {
			return nil, errors.New("missing name or type")
		}
		return s.px.ApplyStoragePool(ctx, name, poolType)
	case "network.apply":
		name, _ := params["name"].(string)
		kind, _ := params["kind"].(string)
		cidr, _ := params["cidr"].(string)
		if name == "" || kind == "" || cidr == "" {
			return nil, errors.New("missing name, kind, or cidr")
		}
		return s.px.ApplyNetwork(ctx, name, kind, cidr)
	case "updates.rollout_start":
		return map[string]any{"changed": true, "status": "rolling_update_started"}, nil
	case "updates.rollout_pause":
		return map[string]any{"changed": true, "status": "rolling_update_paused"}, nil
	case "updates.rollout_abort":
		return map[string]any{"changed": true, "status": "rolling_update_aborted"}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", tool)
	}
}