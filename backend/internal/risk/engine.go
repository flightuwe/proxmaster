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
	case "node.set_maintenance", "vm.migrate", "updates.rollout_pause":
		return models.RiskMedium
	case "storage.pool.apply", "network.apply", "updates.rollout_start", "updates.rollout_abort":
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
	if lowerTool == "network.apply" {
		return true, "network changes may impact quorum and are hard-blocked"
	}
	if lowerTool == "storage.pool.apply" {
		if scope, ok := params["scope"].(string); ok && strings.EqualFold(scope, "cluster") {
			return true, "cluster-wide storage change is hard-blocked"
		}
		return true, "storage pool mutations require explicit approval"
	}
	if lowerTool == "updates.rollout_start" {
		if count, ok := params["node_count"].(float64); ok && count >= 2 {
			return true, fmt.Sprintf("rolling update affecting %.0f nodes is hard-blocked", count)
		}
		return true, "rolling updates require explicit approval"
	}
	if lowerTool == "updates.rollout_abort" {
		return true, "rollout abort can cause inconsistent state and is hard-blocked"
	}

	return true, "high-risk action requires second approval"
}