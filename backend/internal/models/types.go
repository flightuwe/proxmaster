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
	JobPendingApproval JobStatus = "PENDING_APPROVAL"
	JobRunning         JobStatus = "RUNNING"
	JobSucceeded       JobStatus = "SUCCEEDED"
	JobFailed          JobStatus = "FAILED"
	JobBlocked         JobStatus = "BLOCKED"
)

type Job struct {
	ID        string                 `json:"id"`
	Tool      string                 `json:"tool"`
	Input     map[string]any         `json:"input"`
	Risk      RiskLevel              `json:"risk"`
	Status    JobStatus              `json:"status"`
	Result    map[string]any         `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Maintenance bool   `json:"maintenance"`
}

type VM struct {
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	Power    string `json:"power"`
	Priority int    `json:"priority"`
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
	Tool       string                 `json:"tool"`
	Params     map[string]any         `json:"params"`
	Actor      string                 `json:"actor"`
	ApproveNow bool                   `json:"approve_now"`
	Metadata   map[string]any         `json:"metadata"`
}

type MCPCallResponse struct {
	Job          Job            `json:"job"`
	HardBlocked  bool           `json:"hard_blocked"`
	NeedsApprove bool           `json:"needs_approval"`
	AuditEvent   AuditEvent     `json:"audit_event"`
	Output       map[string]any `json:"output,omitempty"`
}