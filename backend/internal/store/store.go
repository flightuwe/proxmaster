package store

import "proxmaster/backend/internal/models"

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
}
