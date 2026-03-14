package health

import (
	"fmt"
	"time"

	"proxmaster/backend/internal/models"
)

type Snapshot struct {
	QuorumHealthy bool
	RunnerHealthy bool
	OnlineNodes   int
	TotalNodes    int
	LastChecked   time.Time
}

type GateEvaluator struct {
	failClosed          bool
	runnerHeartbeatMax  time.Duration
}

func NewGateEvaluator(failClosed bool, runnerHeartbeatMaxSec int) *GateEvaluator {
	return &GateEvaluator{
		failClosed:         failClosed,
		runnerHeartbeatMax: time.Duration(runnerHeartbeatMaxSec) * time.Second,
	}
}

func (g *GateEvaluator) SnapshotFromState(state models.ClusterState) Snapshot {
	now := time.Now().UTC()
	s := Snapshot{
		QuorumHealthy: true,
		RunnerHealthy: true,
		TotalNodes:    len(state.Nodes),
		LastChecked:   now,
	}
	for _, n := range state.Nodes {
		if n.Status == "online" {
			s.OnlineNodes++
		}
		if !n.RunnerHealthy || now.Sub(n.LastHeartbeat) > g.runnerHeartbeatMax {
			s.RunnerHealthy = false
		}
	}
	if s.TotalNodes == 0 || s.OnlineNodes < (s.TotalNodes/2+1) {
		s.QuorumHealthy = false
	}
	return s
}

func (g *GateEvaluator) ValidateForWrite(snapshot Snapshot) (bool, string) {
	if !snapshot.QuorumHealthy {
		return false, "quorum is not healthy"
	}
	if !snapshot.RunnerHealthy {
		if g.failClosed {
			return false, "runner heartbeat unhealthy (fail-closed)"
		}
	}
	return true, ""
}

func (g *GateEvaluator) Explain(snapshot Snapshot) map[string]any {
	ok, reason := g.ValidateForWrite(snapshot)
	return map[string]any{
		"allow_writes":     ok,
		"reason":           reason,
		"quorum_healthy":   snapshot.QuorumHealthy,
		"runner_healthy":   snapshot.RunnerHealthy,
		"online_nodes":     snapshot.OnlineNodes,
		"total_nodes":      snapshot.TotalNodes,
		"last_checked_utc": snapshot.LastChecked.Format(time.RFC3339),
		"summary":          fmt.Sprintf("nodes %d/%d online", snapshot.OnlineNodes, snapshot.TotalNodes),
	}
}
