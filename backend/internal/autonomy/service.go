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
		s.store.CompleteAgentTask(task.ID, nil, err.Error())
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
	s.store.CompleteAgentTask(task.ID, result, errMsg)
}
