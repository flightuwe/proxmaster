package policy

import (
	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/models"
)

type Decision struct {
	Allow             bool
	NeedsReview       bool
	Reason            string
	Type              models.DecisionType
	RequiredApprovals int
}

type Gate struct{}

func NewGate() *Gate {
	return &Gate{}
}

func (g *Gate) Evaluate(risk models.RiskLevel, hardBlocked bool, hardBlockReason string, approveNow bool, gateOK bool, gateReason string, hardwareMFA bool, secondApprover string) Decision {
	if !gateOK {
		return Decision{
			Allow:       false,
			NeedsReview: true,
			Reason:      gateReason,
			Type:        models.DecisionBlocked,
		}
	}

	if hardBlocked && !approveNow {
		return Decision{
			Allow:             false,
			NeedsReview:       true,
			Reason:            hardBlockReason,
			Type:              models.DecisionRequiresApproval,
			RequiredApprovals: 2,
		}
	}

	switch risk {
	case models.RiskLow, models.RiskMedium:
		return Decision{Allow: true, Type: models.DecisionAutoRun}
	case models.RiskHigh:
		if approveNow && hardwareMFA && secondApprover != "" {
			return Decision{Allow: true, Type: models.DecisionAutoRun, RequiredApprovals: 2}
		}
		return Decision{
			Allow:             false,
			NeedsReview:       true,
			Reason:            "high-risk action requires reauth + hardware MFA + second approver",
			Type:              models.DecisionRequiresApproval,
			RequiredApprovals: 2,
		}
	default:
		return Decision{Allow: false, NeedsReview: true, Reason: "unknown risk level", Type: models.DecisionBlocked}
	}
}

func (g *Gate) Simulate(risk models.RiskLevel, hardBlocked bool, hardBlockReason string, approveNow bool, snapshot health.Snapshot) Decision {
	gate := health.NewGateEvaluator(true, 120)
	gateOK, gateReason := gate.ValidateForWrite(snapshot)
	return g.Evaluate(risk, hardBlocked, hardBlockReason, approveNow, gateOK, gateReason, false, "")
}
