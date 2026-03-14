package policy

import "proxmaster/backend/internal/models"

type Decision struct {
	Allow       bool
	NeedsReview bool
	Reason      string
}

type Gate struct{}

func NewGate() *Gate {
	return &Gate{}
}

func (g *Gate) Evaluate(risk models.RiskLevel, hardBlocked bool, hardBlockReason string, approveNow bool) Decision {
	if hardBlocked && !approveNow {
		return Decision{
			Allow:       false,
			NeedsReview: true,
			Reason:      hardBlockReason,
		}
	}

	switch risk {
	case models.RiskLow, models.RiskMedium:
		return Decision{Allow: true}
	case models.RiskHigh:
		if approveNow {
			return Decision{Allow: true}
		}
		return Decision{Allow: false, NeedsReview: true, Reason: "high-risk action requires explicit approval"}
	default:
		return Decision{Allow: false, NeedsReview: true, Reason: "unknown risk level"}
	}
}