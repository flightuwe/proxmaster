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
	case "cluster.get_state":
		return models.RiskLow
	case "node.set_maintenance", "vm.migrate", "updates.rollout_pause", "updates.plan", "policy.simulate", "policy.explain", "vm.create", "vm.clone_from_template", "lxc.create":
		return models.RiskMedium
	case "storage.pool.apply", "storage.plan_apply", "network.apply", "network.plan_apply", "updates.rollout_start", "updates.canary_start", "updates.rollout_continue", "updates.rollout_abort", "node.runner.exec":
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
	if lowerTool == "updates.rollout_start" || lowerTool == "updates.canary_start" || lowerTool == "updates.rollout_continue" {
		if count, ok := params["node_count"].(float64); ok && count >= 2 {
			return true, fmt.Sprintf("rolling update affecting %.0f nodes is hard-blocked", count)
		}
		return true, "rolling updates require explicit approval"
	}
	if lowerTool == "updates.rollout_abort" {
		return true, "rollout abort can cause inconsistent state and is hard-blocked"
	}

	if lowerTool == "node.runner.exec" {
		return true, "host-level runner execution requires explicit approval"
	}

	return true, "high-risk action requires second approval"
}
