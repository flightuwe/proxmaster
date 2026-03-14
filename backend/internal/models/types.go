package models

import "time"

type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

type JobStatus string

const (
	JobPlanned         JobStatus = "PLANNED"
	JobApproved        JobStatus = "APPROVED"
	JobRunning         JobStatus = "RUNNING"
	JobVerified        JobStatus = "VERIFIED"
	JobCompleted       JobStatus = "COMPLETED"
	JobAborted         JobStatus = "ABORTED"
	JobRolledBack      JobStatus = "ROLLED_BACK"
	JobFailed          JobStatus = "FAILED"
	JobBlocked         JobStatus = "BLOCKED"
	JobPendingApproval JobStatus = "PENDING_APPROVAL"
)

type DecisionType string

const (
	DecisionAutoRun          DecisionType = "AUTO_RUN"
	DecisionRequiresApproval DecisionType = "REQUIRES_APPROVAL"
	DecisionBlocked          DecisionType = "BLOCKED"
)

type Job struct {
	ID                string                 `json:"id"`
	IdempotencyKey    string                 `json:"idempotency_key,omitempty"`
	Tool              string                 `json:"tool"`
	Input             map[string]any         `json:"input"`
	Risk              RiskLevel              `json:"risk"`
	Decision          DecisionType           `json:"decision"`
	RequiredApprovals int                    `json:"required_approvals"`
	RollbackPlanID    string                 `json:"rollback_plan_id,omitempty"`
	Status            JobStatus              `json:"status"`
	StatusHistory     []JobStatus            `json:"status_history,omitempty"`
	Result            map[string]any         `json:"result,omitempty"`
	Error             string                 `json:"error,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	Actor     string         `json:"actor"`
	Risk      RiskLevel      `json:"risk"`
	Approved  bool           `json:"approved"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type ClusterState struct {
	Nodes     []Node           `json:"nodes"`
	VMs       []VM             `json:"vms"`
	Pools     []StoragePool    `json:"pools"`
	Networks  []NetworkObject  `json:"networks"`
	HAEnabled bool             `json:"ha_enabled"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type Node struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Maintenance   bool      `json:"maintenance"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	RunnerHealthy bool      `json:"runner_healthy"`
}

type VM struct {
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	Power    string `json:"power"`
	Priority int    `json:"priority"`
	CPU      int    `json:"cpu"`
	MemoryMB int    `json:"memory_mb"`
	DiskGB   int    `json:"disk_gb"`
	Kind     string `json:"kind"`
}

type StoragePool struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type NetworkObject struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	CIDR   string `json:"cidr"`
	Status string `json:"status"`
}

type MCPCallRequest struct {
	Tool            string                 `json:"tool"`
	Params          map[string]any         `json:"params"`
	Actor           string                 `json:"actor"`
	ApproveNow      bool                   `json:"approve_now"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	ReauthToken     string                 `json:"reauth_token"`
	SecondApprover  string                 `json:"second_approver"`
	HardwareMFA     bool                   `json:"hardware_mfa"`
	Metadata        map[string]any         `json:"metadata"`
}

type MCPCallResponse struct {
	Job               Job            `json:"job"`
	JobID             string         `json:"job_id"`
	RiskLevel         RiskLevel      `json:"risk_level"`
	Decision          DecisionType   `json:"decision"`
	RequiredApprovals int            `json:"required_approvals"`
	RollbackPlanID    string         `json:"rollback_plan_id,omitempty"`
	HardBlocked       bool           `json:"hard_blocked"`
	NeedsApprove      bool           `json:"needs_approval"`
	AuditEvent        AuditEvent     `json:"audit_event"`
	Output            map[string]any `json:"output,omitempty"`
}

type PolicySimulationRequest struct {
	Tool        string         `json:"tool"`
	Params      map[string]any `json:"params"`
	ApproveNow  bool           `json:"approve_now"`
	HealthState string         `json:"health_state"`
}

type PolicySimulationResponse struct {
	RiskLevel         RiskLevel    `json:"risk_level"`
	Decision          DecisionType `json:"decision"`
	Reason            string       `json:"reason"`
	RequiredApprovals int          `json:"required_approvals"`
	HardBlocked       bool         `json:"hard_blocked"`
}

type Incident struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
