package store

import (
	"time"

	"proxmaster/backend/internal/models"
)

type Store interface {
	CreateJob(job models.Job) models.Job
	UpdateJob(job models.Job)
	GetJob(id string) (models.Job, bool)
	GetJobByIdempotencyKey(key string) (models.Job, bool)
	ListJobs() []models.Job
	AddAudit(action, actor string, risk models.RiskLevel, approved bool, meta map[string]any) models.AuditEvent
	ListAudit() []models.AuditEvent
	ClusterState() models.ClusterState
	RecordIncident(kind, severity, message string) models.Incident
	ListIncidents() []models.Incident
	SetNodeMaintenance(nodeID string, maintenance bool) bool
	MarkNodeHeartbeat(nodeID string) bool
	MigrateVM(vmID, targetNode string) bool
	CreateVM(vm models.VM) models.VM
	CreateLXC(vm models.VM) models.VM
	CloneVM(templateID, newID, targetNode, name string) (models.VM, bool)
	ApplyPool(name, poolType string)
	ApplyNetwork(name, kind, cidr string)
	SyncStorageInventory() map[string]any
	PlanRebuildAllPools() models.StorageRebuildPlan
	ExecuteRebuildAllPools(planID string) (models.StorageRebuildPlan, bool)
	ApplyReplicationPolicy(policy models.ReplicationPolicy) models.ReplicationPolicy
	UpsertBackupTarget(target models.BackupTarget) models.BackupTarget
	ListBackupTargets() []models.BackupTarget
	UpsertBackupPolicy(policy models.BackupPolicy) models.BackupPolicy
	ListBackupPolicies() []models.BackupPolicy
	ExplainBackupPolicy(workloadID string) (models.BackupPolicy, models.BackupDecisionLog, bool)
	RunBackupNow(workloadID string) map[string]any
	PlanRestore(workloadID, targetID string) models.RestorePlan
	ExecuteRestore(planID string) (models.RestorePlan, bool)
	VerifyBackupSample() map[string]any
	CreateAgentTask(task models.AgentTask) models.AgentTask
	ListAgentTasks() []models.AgentTask
	GetAgentTask(id string) (models.AgentTask, bool)
	ClaimNextAgentTask(worker string) (models.AgentTask, bool)
	CompleteAgentTask(id string, result map[string]any, errMsg string) (models.AgentTask, bool)
	UpsertSpec(scope string, key string, desired map[string]any) models.ResourceSpec
	GetSpec(scope string, key string) (models.ResourceSpec, bool)
	ListSpecs(scope string) map[string]models.ResourceSpec
	SetSpecObserved(scope string, key string, observed map[string]any, drift models.DriftStatus, reconcileJobID string) (models.ResourceSpec, bool)
	DesiredStateBundle() models.DesiredStateBundle
	ListBlueprints() []models.BlueprintDefinition
	GetBlueprint(name string) (models.BlueprintDefinition, bool)
	UpsertBlueprint(spec models.ServiceBlueprintSpec) models.ServiceBlueprintSpec
	ListBlueprintSpecs() []models.ServiceBlueprintSpec
	GetPolicyMode() models.PolicyModeState
	SetPolicyMode(mode models.PolicyMode, actor string, duration time.Duration) models.PolicyModeState
}
