package mcp

import (
	"context"
	"strings"

	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/store"
)

type Service struct {
	store        store.Store
	riskEngine   *risk.Engine
	policyGate   *policy.Gate
	gateEval     *health.GateEvaluator
	orchestrator *orchestrator.Service
}

func NewService(st store.Store, r *risk.Engine, g *policy.Gate, gateEval *health.GateEvaluator, o *orchestrator.Service) *Service {
	return &Service{
		store:        st,
		riskEngine:   r,
		policyGate:   g,
		gateEval:     gateEval,
		orchestrator: o,
	}
}

func (s *Service) HandleCall(ctx context.Context, req models.MCPCallRequest) (models.MCPCallResponse, error) {
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if req.IdempotencyKey != "" {
		if existing, ok := s.store.GetJobByIdempotencyKey(req.IdempotencyKey); ok {
			resp := models.MCPCallResponse{
				Job:               existing,
				JobID:             existing.ID,
				RiskLevel:         existing.Risk,
				Decision:          existing.Decision,
				RequiredApprovals: existing.RequiredApprovals,
				RollbackPlanID:    existing.RollbackPlanID,
				NeedsApprove:      existing.Status == models.JobPendingApproval || existing.Status == models.JobBlocked,
			}
			return resp, nil
		}
	}

	riskLevel := s.riskEngine.Classify(req.Tool, req.Params)
	hardBlocked, hardReason := s.riskEngine.HardBlockReason(req.Tool, req.Params, riskLevel)
	snapshot := s.gateEval.SnapshotFromState(s.store.ClusterState())
	gateOK, gateReason := true, ""
	// Fail-closed health gates must block mutating operations, but not low-risk read-only calls.
	if riskLevel != models.RiskLow {
		gateOK, gateReason = s.gateEval.ValidateForWrite(snapshot)
	}

	decision := s.policyGate.Evaluate(
		riskLevel,
		hardBlocked,
		hardReason,
		req.ApproveNow,
		gateOK,
		gateReason,
		req.HardwareMFA,
		strings.TrimSpace(req.SecondApprover),
		s.store.GetPolicyMode(),
	)

	job := s.store.CreateJob(models.Job{
		IdempotencyKey:    req.IdempotencyKey,
		Tool:              req.Tool,
		Input:             req.Params,
		Risk:              riskLevel,
		Decision:          decision.Type,
		RequiredApprovals: decision.RequiredApprovals,
		Status:            models.JobPlanned,
	})
	audit := s.store.AddAudit(req.Tool, req.Actor, riskLevel, req.ApproveNow, map[string]any{
		"decision_reason":     decision.Reason,
		"metadata":            req.Metadata,
		"health_gate":         s.gateEval.Explain(snapshot),
		"second_approver":     req.SecondApprover,
		"hardware_mfa":        req.HardwareMFA,
		"idempotency_key_set": req.IdempotencyKey != "",
	})

	resp := models.MCPCallResponse{
		Job:               job,
		JobID:             job.ID,
		RiskLevel:         riskLevel,
		Decision:          decision.Type,
		RequiredApprovals: decision.RequiredApprovals,
		RollbackPlanID:    job.RollbackPlanID,
		HardBlocked:       hardBlocked,
		NeedsApprove:      decision.NeedsReview,
		AuditEvent:        audit,
	}

	if !decision.Allow {
		if decision.Type == models.DecisionBlocked {
			s.store.RecordIncident("policy_block", "warning", decision.Reason)
		}
		if decision.Type == models.DecisionRequiresApproval {
			job.Status = models.JobPendingApproval
		} else {
			job.Status = models.JobBlocked
		}
		job.Error = decision.Reason
		s.store.UpdateJob(job)
		resp.Job = job
		return resp, nil
	}

	job.Status = models.JobApproved
	s.store.UpdateJob(job)
	job.Status = models.JobRunning
	s.store.UpdateJob(job)

	result, err := s.orchestrator.Execute(ctx, req.Tool, req.Params)
	if err != nil {
		s.store.RecordIncident("job_failed", "critical", err.Error())
		job.Status = models.JobFailed
		job.Error = err.Error()
		s.store.UpdateJob(job)
		resp.Job = job
		return resp, nil
	}

	job.Status = models.JobVerified
	s.store.UpdateJob(job)
	job.Status = models.JobCompleted
	job.Result = result
	s.store.UpdateJob(job)

	resp.Job = job
	resp.Output = result
	if dd, ok := result["drift_delta"].(map[string]any); ok {
		resp.DriftDelta = dd
	}
	return resp, nil
}

func (s *Service) SimulatePolicy(req models.PolicySimulationRequest) models.PolicySimulationResponse {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	riskLevel := s.riskEngine.Classify(req.Tool, req.Params)
	hardBlocked, hardReason := s.riskEngine.HardBlockReason(req.Tool, req.Params, riskLevel)
	snapshot := s.gateEval.SnapshotFromState(s.store.ClusterState())
	decision := s.policyGate.Simulate(riskLevel, hardBlocked, hardReason, req.ApproveNow, snapshot, s.store.GetPolicyMode())
	return models.PolicySimulationResponse{
		RiskLevel:         riskLevel,
		Decision:          decision.Type,
		Reason:            decision.Reason,
		RequiredApprovals: decision.RequiredApprovals,
		HardBlocked:       hardBlocked,
	}
}
