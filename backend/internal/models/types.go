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
	ID                string         `json:"id"`
	IdempotencyKey    string         `json:"idempotency_key,omitempty"`
	Tool              string         `json:"tool"`
	Input             map[string]any `json:"input"`
	Risk              RiskLevel      `json:"risk"`
	Decision          DecisionType   `json:"decision"`
	RequiredApprovals int            `json:"required_approvals"`
	RollbackPlanID    string         `json:"rollback_plan_id,omitempty"`
	Status            JobStatus      `json:"status"`
	StatusHistory     []JobStatus    `json:"status_history,omitempty"`
	Result            map[string]any `json:"result,omitempty"`
	Error             string         `json:"error,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
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
	Nodes               []Node              `json:"nodes"`
	VMs                 []VM                `json:"vms"`
	Pools               []StoragePool       `json:"pools"`
	Datastores          []Datastore         `json:"datastores"`
	SnapshotTiers       []SnapshotTier      `json:"snapshot_tiers"`
	ReplicationPolicies []ReplicationPolicy `json:"replication_policies"`
	BackupTargets       []BackupTarget      `json:"backup_targets"`
	Networks            []NetworkObject     `json:"networks"`
	HAEnabled           bool                `json:"ha_enabled"`
	UpdatedAt           time.Time           `json:"updated_at"`
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
	Name       string `json:"name"`
	Type       string `json:"type"`
	Backend    string `json:"backend"`
	Status     string `json:"status"`
	CapacityGB int    `json:"capacity_gb"`
	UsedGB     int    `json:"used_gb"`
	Tier       string `json:"tier"`
}

type Datastore struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	PoolName string `json:"pool_name"`
	Path     string `json:"path"`
	Status   string `json:"status"`
}

type SnapshotTier struct {
	Name            string `json:"name"`
	Frequency       string `json:"frequency"`
	Retention       string `json:"retention"`
	Immutable       bool   `json:"immutable"`
	TargetDatastore string `json:"target_datastore"`
}

type ReplicationPolicy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SourcePool  string `json:"source_pool"`
	TargetPool  string `json:"target_pool"`
	Schedule    string `json:"schedule"`
	Compression string `json:"compression"`
	VerifyAfter bool   `json:"verify_after"`
	Status      string `json:"status"`
}

type BackupTarget struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	URI     string `json:"uri"`
	Healthy bool   `json:"healthy"`
}

type BackupPolicy struct {
	ID            string    `json:"id"`
	WorkloadID    string    `json:"workload_id"`
	WorkloadKind  string    `json:"workload_kind"`
	Priority      int       `json:"priority"`
	Override      bool      `json:"override"`
	Schedule      string    `json:"schedule"`
	TargetID      string    `json:"target_id"`
	RPO           string    `json:"rpo"`
	Retention     string    `json:"retention"`
	Encryption    bool      `json:"encryption"`
	Immutability  bool      `json:"immutability"`
	VerifyRestore bool      `json:"verify_restore"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type BackupDecisionLog struct {
	ID         string    `json:"id"`
	WorkloadID string    `json:"workload_id"`
	PolicyID   string    `json:"policy_id"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

type StorageRebuildPlan struct {
	ID                string    `json:"id"`
	PoolNames         []string  `json:"pool_names"`
	AffectedWorkloads []string  `json:"affected_workloads"`
	EstimatedDowntime string    `json:"estimated_downtime"`
	CanaryPool        string    `json:"canary_pool"`
	GuardrailSummary  string    `json:"guardrail_summary"`
	RollbackSteps     []string  `json:"rollback_steps"`
	DryRunPassed      bool      `json:"dry_run_passed"`
	CreatedAt         time.Time `json:"created_at"`
}

type RestorePlan struct {
	ID             string    `json:"id"`
	WorkloadID     string    `json:"workload_id"`
	BackupTargetID string    `json:"backup_target_id"`
	SnapshotRef    string    `json:"snapshot_ref"`
	DryRunPassed   bool      `json:"dry_run_passed"`
	CreatedAt      time.Time `json:"created_at"`
}

type NetworkObject struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	CIDR   string `json:"cidr"`
	Status string `json:"status"`
}

type MCPCallRequest struct {
	Tool           string         `json:"tool"`
	Params         map[string]any `json:"params"`
	Actor          string         `json:"actor"`
	ApproveNow     bool           `json:"approve_now"`
	IdempotencyKey string         `json:"idempotency_key"`
	ReauthToken    string         `json:"reauth_token"`
	SecondApprover string         `json:"second_approver"`
	HardwareMFA    bool           `json:"hardware_mfa"`
	Metadata       map[string]any `json:"metadata"`
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
	DriftDelta        map[string]any `json:"drift_delta,omitempty"`
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

type AgentTaskStatus string

const (
	AgentTaskQueued    AgentTaskStatus = "QUEUED"
	AgentTaskRunning   AgentTaskStatus = "RUNNING"
	AgentTaskCompleted AgentTaskStatus = "COMPLETED"
	AgentTaskFailed    AgentTaskStatus = "FAILED"
)

type AgentTask struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     map[string]any  `json:"payload,omitempty"`
	Status      AgentTaskStatus `json:"status"`
	RequestedBy string          `json:"requested_by"`
	Priority    int             `json:"priority"`
	MaxAttempts int             `json:"max_attempts"`
	DeadLetter  bool            `json:"dead_letter"`
	Result      map[string]any  `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	Attempts    int             `json:"attempts"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
}

type DriftStatus string

const (
	DriftInSync  DriftStatus = "IN_SYNC"
	DriftPending DriftStatus = "PENDING"
	DriftDrifted DriftStatus = "DRIFTED"
	DriftError   DriftStatus = "ERROR"
)

type ResourceSpec struct {
	Desired            map[string]any `json:"desired"`
	Observed           map[string]any `json:"observed"`
	DriftStatus        DriftStatus    `json:"drift_status"`
	LastReconcileJobID string         `json:"last_reconcile_job_id,omitempty"`
	SpecVersion        int            `json:"spec_version"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type ClusterSpec struct {
	HAEnabled bool `json:"ha_enabled"`
}

type NodeSpec struct {
	NodeID      string `json:"node_id"`
	Maintenance bool   `json:"maintenance"`
}

type WorkloadSpec struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	NodeID       string            `json:"node_id"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	DesiredPower string            `json:"desired_power"`
	TemplateID   string            `json:"template_id,omitempty"`
	CloudInit    map[string]string `json:"cloud_init,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

type StorageSpec struct {
	Pools []StoragePool `json:"pools"`
}

type NetworkSpec struct {
	Networks []NetworkObject `json:"networks"`
}

type BackupSpec struct {
	Policies []BackupPolicy `json:"policies"`
}

type ServiceBlueprintSpec struct {
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	Workload   WorkloadSpec   `json:"workload"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type DesiredStateBundle struct {
	Cluster    ResourceSpec `json:"cluster"`
	Storage    ResourceSpec `json:"storage"`
	Network    ResourceSpec `json:"network"`
	Backup     ResourceSpec `json:"backup"`
	Workloads  ResourceSpec `json:"workloads"`
	Blueprints ResourceSpec `json:"blueprints"`
}

type BlueprintDefinition struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Description   string         `json:"description"`
	ProvisionKind string         `json:"provision_kind"`
	DefaultCPU    int            `json:"default_cpu"`
	DefaultMemMB  int            `json:"default_memory_mb"`
	DefaultDiskGB int            `json:"default_disk_gb"`
	AnsibleRoles  []string       `json:"ansible_roles"`
	HealthChecks  []string       `json:"health_checks"`
	RollbackSteps []string       `json:"rollback_steps"`
	Parameters    map[string]any `json:"parameters"`
}

type PolicyMode string

const (
	PolicyModeGuardedSRE PolicyMode = "GUARDED_SRE"
	PolicyModeAggressive PolicyMode = "AGGRESSIVE_AUTO"
)

type PolicyModeState struct {
	Mode            PolicyMode `json:"mode"`
	AggressiveUntil time.Time  `json:"aggressive_until,omitempty"`
	LastChangedBy   string     `json:"last_changed_by,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
