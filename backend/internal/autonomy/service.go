package autonomy

import (
	"context"
	"log"
	"sync"
	"time"

	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/store"
)

type Service struct {
	store     store.Store
	mcpSvc    *mcp.Service
	pollEvery time.Duration

	mu            sync.Mutex
	lastScheduled map[string]time.Time
}

func NewService(st store.Store, mcpSvc *mcp.Service, pollEverySec int) *Service {
	if pollEverySec < 5 {
		pollEverySec = 5
	}
	return &Service{
		store:         st,
		mcpSvc:        mcpSvc,
		pollEvery:     time.Duration(pollEverySec) * time.Second,
		lastScheduled: make(map[string]time.Time),
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.schedulerLoop(ctx)
	go s.workerLoop(ctx)
}

func (s *Service) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	s.ensureRecurringTasks()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ensureRecurringTasks()
		}
	}
}

func (s *Service) ensureRecurringTasks() {
	s.scheduleOnce("storage.inventory.sync", 2*time.Minute)
	s.scheduleOnce("backup.verify.sample", 15*time.Minute)
	s.scheduleOnce("spec.reconcile.all", 1*time.Minute)
}

func (s *Service) scheduleOnce(tool string, minInterval time.Duration) {
	now := time.Now().UTC()
	s.mu.Lock()
	last := s.lastScheduled[tool]
	if !last.IsZero() && now.Sub(last) < minInterval {
		s.mu.Unlock()
		return
	}
	s.lastScheduled[tool] = now
	s.mu.Unlock()
	s.store.CreateAgentTask(models.AgentTask{
		Type:        tool,
		Status:      models.AgentTaskQueued,
		Payload:     map[string]any{},
		RequestedBy: "autonomy-scheduler",
		Priority:    50,
		MaxAttempts: 3,
	})
}

func (s *Service) workerLoop(ctx context.Context) {
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Service) runOnce(ctx context.Context) {
	task, ok := s.store.ClaimNextAgentTask("autonomy-worker")
	if !ok {
		return
	}
	if task.Type == "spec.reconcile.all" {
		s.reconcileAllSpecs(task.ID)
		return
	}
	if task.Type == "spec.reconcile" {
		s.reconcileOneSpec(task.ID, task.Payload)
		return
	}
	resp, err := s.mcpSvc.HandleCall(ctx, models.MCPCallRequest{
		Tool:           task.Type,
		Params:         task.Payload,
		Actor:          "autonomy-worker",
		ApproveNow:     false,
		HardwareMFA:    false,
		SecondApprover: "",
		IdempotencyKey: task.ID,
		Metadata: map[string]any{
			"source": "autonomy-task",
		},
	})
	if err != nil {
		s.completeWithRetry(task, nil, err.Error())
		log.Printf("autonomy task failed: id=%s tool=%s err=%v", task.ID, task.Type, err)
		return
	}
	result := map[string]any{
		"job_id":   resp.JobID,
		"decision": resp.Decision,
		"status":   resp.Job.Status,
		"output":   resp.Output,
	}
	errMsg := ""
	if resp.Job.Status == models.JobBlocked || resp.Job.Status == models.JobPendingApproval || resp.Job.Status == models.JobFailed {
		errMsg = resp.Job.Error
		if errMsg == "" {
			errMsg = "task not completed automatically"
		}
	}
	if errMsg != "" {
		s.completeWithRetry(task, result, errMsg)
		return
	}
	s.store.CompleteAgentTask(task.ID, result, "")
}

func (s *Service) reconcileAllSpecs(taskID string) {
	scopes := []string{"cluster", "storage", "network", "backup", "workloads", "blueprint"}
	queued := 0
	for _, scope := range scopes {
		group := s.store.ListSpecs(scope)
		for key := range group {
			s.store.CreateAgentTask(models.AgentTask{
				Type:        "spec.reconcile",
				Payload:     map[string]any{"scope": scope, "key": key},
				RequestedBy: "autonomy-worker",
				Priority:    80,
				MaxAttempts: 3,
			})
			queued++
		}
	}
	s.store.CompleteAgentTask(taskID, map[string]any{"queued_reconcile_tasks": queued}, "")
}

func (s *Service) reconcileOneSpec(taskID string, payload map[string]any) {
	scope, _ := payload["scope"].(string)
	key, _ := payload["key"].(string)
	if scope == "" || key == "" {
		s.store.CompleteAgentTask(taskID, nil, "missing scope/key")
		return
	}
	spec, ok := s.store.GetSpec(scope, key)
	if !ok {
		s.store.CompleteAgentTask(taskID, nil, "spec not found")
		return
	}
	drift := models.DriftInSync
	if len(spec.Desired) == 0 {
		drift = models.DriftPending
	}
	observed := map[string]any{
		"reconciled_at_utc": time.Now().UTC().Format(time.RFC3339),
		"scope":             scope,
		"key":               key,
		"desired_hash_hint": len(spec.Desired),
	}
	updated, _ := s.store.SetSpecObserved(scope, key, observed, drift, taskID)
	s.store.CompleteAgentTask(taskID, map[string]any{"spec": updated, "phase": "reconcile"}, "")
}

func (s *Service) completeWithRetry(task models.AgentTask, result map[string]any, errMsg string) {
	updated, _ := s.store.CompleteAgentTask(task.ID, result, errMsg)
	maxAttempts := updated.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if updated.Attempts < maxAttempts {
		s.store.CreateAgentTask(models.AgentTask{
			Type:        updated.Type,
			Payload:     updated.Payload,
			RequestedBy: updated.RequestedBy,
			Priority:    updated.Priority,
			MaxAttempts: maxAttempts,
		})
		return
	}
	s.store.CreateAgentTask(models.AgentTask{
		Type:        "deadletter." + updated.Type,
		Payload:     map[string]any{"origin_task_id": updated.ID, "error": errMsg, "result": result},
		RequestedBy: "autonomy-worker",
		Priority:    10,
		MaxAttempts: 1,
		DeadLetter:  true,
	})
}
