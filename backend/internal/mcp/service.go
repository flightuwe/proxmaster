package mcp

import (
	"context"

	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/store"
)

type Service struct {
	store        *store.MemoryStore
	riskEngine   *risk.Engine
	policyGate   *policy.Gate
	orchestrator *orchestrator.Service
}

func NewService(s *store.MemoryStore, r *risk.Engine, g *policy.Gate, o *orchestrator.Service) *Service {
	return &Service{store: s, riskEngine: r, policyGate: g, orchestrator: o}
}

func (s *Service) HandleCall(ctx context.Context, req models.MCPCallRequest) (models.MCPCallResponse, error) {
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}

	riskLevel := s.riskEngine.Classify(req.Tool, req.Params)
	hardBlocked, reason := s.riskEngine.HardBlockReason(req.Tool, req.Params, riskLevel)
	decision := s.policyGate.Evaluate(riskLevel, hardBlocked, reason, req.ApproveNow)

	status := models.JobRunning
	if !decision.Allow && decision.NeedsReview {
		status = models.JobPendingApproval
	}
	job := s.store.CreateJob(req.Tool, req.Params, riskLevel, status)
	audit := s.store.AddAudit(req.Tool, req.Actor, riskLevel, req.ApproveNow, map[string]any{
		"decision": decision.Reason,
		"metadata": req.Metadata,
	})

	resp := models.MCPCallResponse{
		Job:          job,
		HardBlocked:  hardBlocked,
		NeedsApprove: decision.NeedsReview,
		AuditEvent:   audit,
	}

	if !decision.Allow {
		job.Status = models.JobBlocked
		job.Error = decision.Reason
		s.store.UpdateJob(job)
		resp.Job = job
		return resp, nil
	}

	result, err := s.orchestrator.Execute(ctx, req.Tool, req.Params)
	if err != nil {
		job.Status = models.JobFailed
		job.Error = err.Error()
		s.store.UpdateJob(job)
		resp.Job = job
		return resp, nil
	}

	job.Status = models.JobSucceeded
	job.Result = result
	s.store.UpdateJob(job)
	resp.Job = job
	resp.Output = result
	return resp, nil
}