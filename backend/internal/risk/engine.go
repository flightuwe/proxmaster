package risk

import (
	"fmt"
	"strings"

	"proxmaster/backend/internal/models"
)

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Classify(tool string, params map[string]any) models.RiskLevel {
	tool = strings.ToLower(tool)
	switch tool {
	case "cluster.get_state", "proxmox.connection.test", "connectivity.status", "gitops.status", "ssh.breakglass.status", "vpn.wireguard.status", "state.get.all", "workload.spec.explain", "backup.policy.simulate", "blueprint.list", "blueprint.plan", "blueprint.verify":
		return models.RiskLow
	case "node.set_maintenance", "vm.migrate", "vm.migration.plan", "updates.rollout_pause", "updates.plan", "policy.simulate", "policy.explain", "vm.create", "vm.clone_from_template", "lxc.create", "storage.inventory.sync", "storage.health.explain", "backup.policy.explain", "backup.policy.list", "backup.target.list", "gitops.sync.now", "gitops.rollback", "ssh.breakglass.disable", "vpn.wireguard.plan", "workload.spec.apply", "policy.mode.set":
		return models.RiskMedium
	case "storage.pool.apply", "storage.plan_apply", "storage.pool.rebuild_all.plan", "storage.pool.rebuild_all.execute", "storage.replication.plan_apply", "network.apply", "network.plan_apply", "updates.rollout_start", "updates.canary_start", "updates.rollout_continue", "updates.rollout_abort", "node.runner.exec", "proxmaster.self_migrate", "backup.policy.upsert", "backup.run.now", "backup.restore.plan", "backup.restore.execute", "backup.verify.sample", "ssh.breakglass.enable", "vpn.wireguard.apply", "blueprint.deploy", "blueprint.update", "blueprint.rollback":
		return models.RiskHigh
	}

	if strings.Contains(tool, "shutdown") || strings.Contains(tool, "quorum") {
		return models.RiskHigh
	}

	if val, ok := params["scope"].(string); ok {
		scope := strings.ToLower(val)
		if scope == "cluster" || scope == "all_nodes" {
			return models.RiskHigh
		}
	}

	if cmd, ok := params["command"].(string); ok {
		if strings.Contains(strings.ToLower(cmd), "reboot") || strings.Contains(strings.ToLower(cmd), "shutdown") {
			return models.RiskHigh
		}
	}

	return models.RiskMedium
}

func (e *Engine) HardBlockReason(tool string, params map[string]any, risk models.RiskLevel) (bool, string) {
	if risk != models.RiskHigh {
		return false, ""
	}

	lowerTool := strings.ToLower(tool)
	if strings.Contains(lowerTool, "shutdown") {
		return true, "cluster shutdown is always hard-blocked"
	}
	if lowerTool == "network.apply" || lowerTool == "network.plan_apply" {
		return true, "network changes may impact quorum and are hard-blocked"
	}
	if lowerTool == "storage.pool.apply" || lowerTool == "storage.plan_apply" {
		if scope, ok := params["scope"].(string); ok && strings.EqualFold(scope, "cluster") {
			return true, "cluster-wide storage change is hard-blocked"
		}
		return true, "storage pool mutations require explicit approval"
	}
	if lowerTool == "storage.pool.rebuild_all.plan" {
		return true, "rebuild-all planning is guarded and requires explicit approval"
	}
	if lowerTool == "storage.pool.rebuild_all.execute" {
		return true, "rebuild-all execution always requires explicit dual approval"
	}
	if lowerTool == "storage.replication.plan_apply" {
		return true, "replication policy changes require explicit approval"
	}
	if lowerTool == "updates.rollout_start" || lowerTool == "updates.canary_start" || lowerTool == "updates.rollout_continue" {
		if count, ok := params["node_count"].(float64); ok && count >= 2 {
			return true, fmt.Sprintf("rolling update affecting %.0f nodes is hard-blocked", count)
		}
		return true, "rolling updates require explicit approval"
	}
	if lowerTool == "updates.rollout_abort" {
		return true, "rollout abort can cause inconsistent state and is hard-blocked"
	}
	if lowerTool == "proxmaster.self_migrate" {
		return true, "control-plane migration requires explicit dual approval"
	}
	if lowerTool == "ssh.breakglass.enable" {
		return true, "break-glass ssh enable requires explicit dual approval"
	}
	if lowerTool == "vpn.wireguard.apply" {
		return true, "wireguard apply touches host network and requires explicit approval"
	}

	if lowerTool == "node.runner.exec" {
		return true, "host-level runner execution requires explicit approval"
	}
	if strings.HasPrefix(lowerTool, "backup.") {
		return true, "backup and restore mutations require explicit approval"
	}

	return true, "high-risk action requires second approval"
}
