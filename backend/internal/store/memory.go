package store

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"proxmaster/backend/internal/models"
)

type MemoryStore struct {
	mu             sync.RWMutex
	jobs           map[string]models.Job
	byIdemKey      map[string]string
	audits         []models.AuditEvent
	incidents      []models.Incident
	state          models.ClusterState
	backupPolicies map[string]models.BackupPolicy
	backupTargets  map[string]models.BackupTarget
	decisionLogs   []models.BackupDecisionLog
	rebuildPlans   map[string]models.StorageRebuildPlan
	restorePlans   map[string]models.RestorePlan
	agentTasks     map[string]models.AgentTask
	seq            uint64
	vmIDSeq        uint64
	rollbackIDSeq  uint64
}

func NewMemoryStore() *MemoryStore {
	now := time.Now().UTC()
	return &MemoryStore{
		jobs:           make(map[string]models.Job),
		byIdemKey:      make(map[string]string),
		audits:         make([]models.AuditEvent, 0, 64),
		incidents:      make([]models.Incident, 0, 32),
		backupPolicies: make(map[string]models.BackupPolicy),
		backupTargets: map[string]models.BackupTarget{
			"target-pbs-1": {ID: "target-pbs-1", Name: "pbs-main", Kind: "pbs", URI: "pbs://10.0.10.10/datastore1", Healthy: true},
			"target-s3-1":  {ID: "target-s3-1", Name: "s3-archive", Kind: "s3", URI: "s3://proxmaster-archive", Healthy: true},
			"target-nfs-1": {ID: "target-nfs-1", Name: "nfs-backups", Kind: "nfs", URI: "nfs://10.0.20.5/exports/backups", Healthy: true},
		},
		decisionLogs: make([]models.BackupDecisionLog, 0, 32),
		rebuildPlans: make(map[string]models.StorageRebuildPlan),
		restorePlans: make(map[string]models.RestorePlan),
		agentTasks:   make(map[string]models.AgentTask),
		state: models.ClusterState{
			Nodes: []models.Node{
				{ID: "node-1", Name: "pve-node-1", Status: "online", Maintenance: false, LastHeartbeat: now, RunnerHealthy: true},
				{ID: "node-2", Name: "pve-node-2", Status: "online", Maintenance: false, LastHeartbeat: now, RunnerHealthy: true},
				{ID: "node-3", Name: "pve-node-3", Status: "online", Maintenance: false, LastHeartbeat: now, RunnerHealthy: true},
				{ID: "node-4", Name: "pve-node-4", Status: "online", Maintenance: false, LastHeartbeat: now, RunnerHealthy: true},
			},
			VMs: []models.VM{
				{ID: "100", NodeID: "node-1", Name: "proxmaster-mgmt", Power: "running", Priority: 100, CPU: 4, MemoryMB: 8192, DiskGB: 80, Kind: "vm"},
				{ID: "101", NodeID: "node-1", Name: "db-prod-1", Power: "running", Priority: 90, CPU: 4, MemoryMB: 4096, DiskGB: 100, Kind: "vm"},
				{ID: "102", NodeID: "node-2", Name: "api-prod-1", Power: "running", Priority: 80, CPU: 4, MemoryMB: 4096, DiskGB: 50, Kind: "vm"},
				{ID: "201", NodeID: "node-3", Name: "lxc-cache-1", Power: "running", Priority: 60, CPU: 2, MemoryMB: 1024, DiskGB: 10, Kind: "lxc"},
			},
			Pools: []models.StoragePool{
				{Name: "zfs-fast", Type: "zfs", Backend: "zfs", Status: "healthy", CapacityGB: 2000, UsedGB: 1200, Tier: "hot"},
				{Name: "ceph-rbd", Type: "ceph", Backend: "ceph", Status: "healthy", CapacityGB: 6000, UsedGB: 3200, Tier: "balanced"},
				{Name: "nfs-archive", Type: "nfs", Backend: "nfs", Status: "healthy", CapacityGB: 8000, UsedGB: 2100, Tier: "cold"},
			},
			Datastores: []models.Datastore{
				{ID: "ds-1", Name: "local-zfs", Kind: "zfs", PoolName: "zfs-fast", Path: "/zpool/local", Status: "online"},
				{ID: "ds-2", Name: "ceph-main", Kind: "ceph", PoolName: "ceph-rbd", Path: "rbd://ceph-rbd", Status: "online"},
				{ID: "ds-3", Name: "nfs-backup", Kind: "nfs", PoolName: "nfs-archive", Path: "/mnt/nfs/backup", Status: "online"},
			},
			SnapshotTiers: []models.SnapshotTier{
				{Name: "tier-hourly", Frequency: "hourly", Retention: "48h", Immutable: false, TargetDatastore: "ds-1"},
				{Name: "tier-daily", Frequency: "daily", Retention: "30d", Immutable: true, TargetDatastore: "ds-3"},
			},
			ReplicationPolicies: []models.ReplicationPolicy{
				{ID: "rep-1", Name: "zfs-to-nfs", SourcePool: "zfs-fast", TargetPool: "nfs-archive", Schedule: "0 */6 * * *", Compression: "zstd", VerifyAfter: true, Status: "active"},
			},
			BackupTargets: []models.BackupTarget{
				{ID: "target-pbs-1", Name: "pbs-main", Kind: "pbs", URI: "pbs://10.0.10.10/datastore1", Healthy: true},
				{ID: "target-s3-1", Name: "s3-archive", Kind: "s3", URI: "s3://proxmaster-archive", Healthy: true},
				{ID: "target-nfs-1", Name: "nfs-backups", Kind: "nfs", URI: "nfs://10.0.20.5/exports/backups", Healthy: true},
			},
			Networks: []models.NetworkObject{
				{Name: "vmbr0", Kind: "bridge", CIDR: "10.0.10.0/24", Status: "active"},
			},
			HAEnabled: true,
			UpdatedAt: now,
		},
	}
}

func (s *MemoryStore) nextID(prefix string) string {
	n := atomic.AddUint64(&s.seq, 1)
	return fmt.Sprintf("%s-%06d", prefix, n)
}

func (s *MemoryStore) nextVMID() string {
	n := atomic.AddUint64(&s.vmIDSeq, 1)
	return fmt.Sprintf("%d", 200+n)
}

func (s *MemoryStore) nextRollbackPlanID() string {
	n := atomic.AddUint64(&s.rollbackIDSeq, 1)
	return fmt.Sprintf("rollback-%06d", n)
}

func (s *MemoryStore) CreateJob(job models.Job) models.Job {
	now := time.Now().UTC()
	job.ID = s.nextID("job")
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = models.JobPlanned
	}
	if len(job.StatusHistory) == 0 {
		job.StatusHistory = []models.JobStatus{job.Status}
	}
	if job.RollbackPlanID == "" {
		job.RollbackPlanID = s.nextRollbackPlanID()
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	if job.IdempotencyKey != "" {
		s.byIdemKey[job.IdempotencyKey] = job.ID
	}
	s.mu.Unlock()
	return job
}

func (s *MemoryStore) UpdateJob(job models.Job) {
	job.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	if prev, ok := s.jobs[job.ID]; ok {
		if len(job.StatusHistory) == 0 {
			job.StatusHistory = prev.StatusHistory
		}
		if len(job.StatusHistory) == 0 || job.StatusHistory[len(job.StatusHistory)-1] != job.Status {
			job.StatusHistory = append(job.StatusHistory, job.Status)
		}
	}
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

func (s *MemoryStore) GetJob(id string) (models.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	return j, ok
}

func (s *MemoryStore) GetJobByIdempotencyKey(key string) (models.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobID, ok := s.byIdemKey[key]
	if !ok {
		return models.Job{}, false
	}
	j, ok := s.jobs[jobID]
	return j, ok
}

func (s *MemoryStore) ListJobs() []models.Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) AddAudit(action, actor string, risk models.RiskLevel, approved bool, meta map[string]any) models.AuditEvent {
	e := models.AuditEvent{
		ID:        s.nextID("audit"),
		Action:    action,
		Actor:     actor,
		Risk:      risk,
		Approved:  approved,
		Metadata:  meta,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.audits = append(s.audits, e)
	s.mu.Unlock()
	return e
}

func (s *MemoryStore) ListAudit() []models.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.AuditEvent, len(s.audits))
	copy(out, s.audits)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) ClusterState() models.ClusterState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *MemoryStore) RecordIncident(kind, severity, message string) models.Incident {
	incident := models.Incident{
		ID:        s.nextID("incident"),
		Kind:      kind,
		Severity:  severity,
		Message:   message,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.incidents = append(s.incidents, incident)
	s.mu.Unlock()
	return incident
}

func (s *MemoryStore) ListIncidents() []models.Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Incident, len(s.incidents))
	copy(out, s.incidents)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) SetNodeMaintenance(nodeID string, maintenance bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Nodes {
		if s.state.Nodes[i].ID == nodeID {
			s.state.Nodes[i].Maintenance = maintenance
			s.state.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

func (s *MemoryStore) MarkNodeHeartbeat(nodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Nodes {
		if s.state.Nodes[i].ID == nodeID {
			s.state.Nodes[i].LastHeartbeat = time.Now().UTC()
			s.state.Nodes[i].RunnerHealthy = true
			s.state.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

func (s *MemoryStore) MigrateVM(vmID, targetNode string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.VMs {
		if s.state.VMs[i].ID == vmID {
			s.state.VMs[i].NodeID = targetNode
			s.state.UpdatedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

func (s *MemoryStore) CreateVM(vm models.VM) models.VM {
	s.mu.Lock()
	defer s.mu.Unlock()
	if vm.ID == "" {
		vm.ID = s.nextVMID()
	}
	if vm.Kind == "" {
		vm.Kind = "vm"
	}
	if vm.Power == "" {
		vm.Power = "running"
	}
	s.state.VMs = append(s.state.VMs, vm)
	s.state.UpdatedAt = time.Now().UTC()
	return vm
}

func (s *MemoryStore) CreateLXC(vm models.VM) models.VM {
	vm.Kind = "lxc"
	return s.CreateVM(vm)
}

func (s *MemoryStore) CloneVM(templateID, newID, targetNode, name string) (models.VM, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.state.VMs {
		if v.ID == templateID {
			clone := v
			if newID != "" {
				clone.ID = newID
			} else {
				clone.ID = s.nextVMID()
			}
			clone.NodeID = targetNode
			if name != "" {
				clone.Name = name
			}
			s.state.VMs = append(s.state.VMs, clone)
			s.state.UpdatedAt = time.Now().UTC()
			return clone, true
		}
	}
	return models.VM{}, false
}

func (s *MemoryStore) ApplyPool(name, poolType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Pools {
		if s.state.Pools[i].Name == name {
			s.state.Pools[i].Type = poolType
			s.state.Pools[i].Status = "healthy"
			s.state.UpdatedAt = time.Now().UTC()
			return
		}
	}
	s.state.Pools = append(s.state.Pools, models.StoragePool{
		Name: name, Type: poolType, Backend: poolType, Status: "healthy", CapacityGB: 1000, UsedGB: 0, Tier: "balanced",
	})
	s.state.UpdatedAt = time.Now().UTC()
}

func (s *MemoryStore) ApplyNetwork(name, kind, cidr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Networks {
		if s.state.Networks[i].Name == name {
			s.state.Networks[i].Kind = kind
			s.state.Networks[i].CIDR = cidr
			s.state.Networks[i].Status = "active"
			s.state.UpdatedAt = time.Now().UTC()
			return
		}
	}
	s.state.Networks = append(s.state.Networks, models.NetworkObject{Name: name, Kind: kind, CIDR: cidr, Status: "active"})
	s.state.UpdatedAt = time.Now().UTC()
}

func (s *MemoryStore) SyncStorageInventory() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.UpdatedAt = time.Now().UTC()
	return map[string]any{
		"pools":                s.state.Pools,
		"datastores":           s.state.Datastores,
		"snapshot_tiers":       s.state.SnapshotTiers,
		"replication_policies": s.state.ReplicationPolicies,
		"backup_targets":       s.state.BackupTargets,
		"updated_at":           s.state.UpdatedAt,
	}
}

func (s *MemoryStore) PlanRebuildAllPools() models.StorageRebuildPlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	pools := make([]string, 0, len(s.state.Pools))
	for _, p := range s.state.Pools {
		pools = append(pools, p.Name)
	}
	workloads := make([]string, 0, len(s.state.VMs))
	for _, vm := range s.state.VMs {
		workloads = append(workloads, fmt.Sprintf("%s:%s", vm.Kind, vm.ID))
	}
	plan := models.StorageRebuildPlan{
		ID:                s.nextID("rebuild"),
		PoolNames:         pools,
		AffectedWorkloads: workloads,
		EstimatedDowntime: "per pool 2-5 min, full window 20-45 min",
		CanaryPool:        pools[0],
		GuardrailSummary:  "quorum >= 3 nodes, free capacity >= 20%, runner healthy all nodes",
		RollbackSteps: []string{
			"abort current pool operation",
			"restore previous pool metadata",
			"rebind datastore mappings",
			"resume workload IO",
		},
		DryRunPassed: true,
		CreatedAt:    time.Now().UTC(),
	}
	s.rebuildPlans[plan.ID] = plan
	return plan
}

func (s *MemoryStore) ExecuteRebuildAllPools(planID string) (models.StorageRebuildPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.rebuildPlans[planID]
	if !ok {
		return models.StorageRebuildPlan{}, false
	}
	for i := range s.state.Pools {
		s.state.Pools[i].Status = "healthy"
	}
	s.state.UpdatedAt = time.Now().UTC()
	return plan, true
}

func (s *MemoryStore) ApplyReplicationPolicy(policy models.ReplicationPolicy) models.ReplicationPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if policy.ID == "" {
		policy.ID = s.nextID("rep")
	}
	if policy.Status == "" {
		policy.Status = "active"
	}
	for i := range s.state.ReplicationPolicies {
		if s.state.ReplicationPolicies[i].ID == policy.ID {
			s.state.ReplicationPolicies[i] = policy
			s.state.UpdatedAt = time.Now().UTC()
			return policy
		}
	}
	s.state.ReplicationPolicies = append(s.state.ReplicationPolicies, policy)
	s.state.UpdatedAt = time.Now().UTC()
	return policy
}

func (s *MemoryStore) UpsertBackupTarget(target models.BackupTarget) models.BackupTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	if target.ID == "" {
		target.ID = s.nextID("target")
	}
	s.backupTargets[target.ID] = target
	rebuild := make([]models.BackupTarget, 0, len(s.backupTargets))
	for _, t := range s.backupTargets {
		rebuild = append(rebuild, t)
	}
	s.state.BackupTargets = rebuild
	s.state.UpdatedAt = time.Now().UTC()
	return target
}

func (s *MemoryStore) ListBackupTargets() []models.BackupTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.BackupTarget, 0, len(s.backupTargets))
	for _, t := range s.backupTargets {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *MemoryStore) UpsertBackupPolicy(policy models.BackupPolicy) models.BackupPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if policy.ID == "" {
		policy.ID = s.nextID("policy")
	}
	if policy.TargetID == "" {
		policy.TargetID = "target-pbs-1"
	}
	if policy.Schedule == "" {
		policy.Schedule = "0 2 * * *"
	}
	if policy.RPO == "" {
		policy.RPO = "24h"
	}
	if policy.Retention == "" {
		policy.Retention = "14d"
	}
	policy.UpdatedAt = time.Now().UTC()
	s.backupPolicies[policy.ID] = policy
	return policy
}

func (s *MemoryStore) ListBackupPolicies() []models.BackupPolicy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.BackupPolicy, 0, len(s.backupPolicies))
	for _, p := range s.backupPolicies {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WorkloadID == out[j].WorkloadID {
			return out[i].Priority > out[j].Priority
		}
		return out[i].WorkloadID < out[j].WorkloadID
	})
	return out
}

func (s *MemoryStore) ExplainBackupPolicy(workloadID string) (models.BackupPolicy, models.BackupDecisionLog, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected models.BackupPolicy
	found := false
	for _, p := range s.backupPolicies {
		if p.WorkloadID == workloadID {
			if !found || p.Priority > selected.Priority || p.Override {
				selected = p
				found = true
			}
		}
	}
	if !found {
		return models.BackupPolicy{}, models.BackupDecisionLog{}, false
	}
	reason := fmt.Sprintf("selected policy %s by priority/override", selected.ID)
	logEntry := models.BackupDecisionLog{
		ID:         s.nextID("policylog"),
		WorkloadID: workloadID,
		PolicyID:   selected.ID,
		Reason:     reason,
		CreatedAt:  time.Now().UTC(),
	}
	s.decisionLogs = append(s.decisionLogs, logEntry)
	return selected, logEntry, true
}

func (s *MemoryStore) RunBackupNow(workloadID string) map[string]any {
	policy, decision, ok := s.ExplainBackupPolicy(workloadID)
	if !ok {
		return map[string]any{
			"changed":     false,
			"workload_id": workloadID,
			"status":      "no_policy",
		}
	}
	return map[string]any{
		"changed":         true,
		"workload_id":     workloadID,
		"policy_id":       policy.ID,
		"target_id":       policy.TargetID,
		"decision_log_id": decision.ID,
		"status":          "backup_started",
		"started_at_utc":  time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *MemoryStore) PlanRestore(workloadID, targetID string) models.RestorePlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	if targetID == "" {
		targetID = "target-pbs-1"
	}
	plan := models.RestorePlan{
		ID:             s.nextID("restore"),
		WorkloadID:     workloadID,
		BackupTargetID: targetID,
		SnapshotRef:    fmt.Sprintf("%s@latest", workloadID),
		DryRunPassed:   true,
		CreatedAt:      time.Now().UTC(),
	}
	s.restorePlans[plan.ID] = plan
	return plan
}

func (s *MemoryStore) ExecuteRestore(planID string) (models.RestorePlan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.restorePlans[planID]
	return plan, ok
}

func (s *MemoryStore) VerifyBackupSample() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]map[string]any, 0, 2)
	for _, vm := range s.state.VMs {
		if len(results) >= 2 {
			break
		}
		results = append(results, map[string]any{
			"workload_id": vm.ID,
			"kind":        vm.Kind,
			"result":      "restore_test_passed",
		})
	}
	return map[string]any{
		"changed":         false,
		"sample_count":    len(results),
		"results":         results,
		"verified_at_utc": time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *MemoryStore) CreateAgentTask(task models.AgentTask) models.AgentTask {
	now := time.Now().UTC()
	if task.ID == "" {
		task.ID = s.nextID("task")
	}
	if task.Status == "" {
		task.Status = models.AgentTaskQueued
	}
	if task.RequestedBy == "" {
		task.RequestedBy = "android-admin"
	}
	task.CreatedAt = now
	task.UpdatedAt = now
	s.mu.Lock()
	s.agentTasks[task.ID] = task
	s.mu.Unlock()
	return task
}

func (s *MemoryStore) ListAgentTasks() []models.AgentTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.AgentTask, 0, len(s.agentTasks))
	for _, t := range s.agentTasks {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) GetAgentTask(id string) (models.AgentTask, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.agentTasks[id]
	return t, ok
}

func (s *MemoryStore) ClaimNextAgentTask(_ string) (models.AgentTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var picked models.AgentTask
	found := false
	for _, t := range s.agentTasks {
		if t.Status != models.AgentTaskQueued {
			continue
		}
		if !found || t.CreatedAt.Before(picked.CreatedAt) {
			picked = t
			found = true
		}
	}
	if !found {
		return models.AgentTask{}, false
	}
	now := time.Now().UTC()
	picked.Status = models.AgentTaskRunning
	picked.Attempts++
	picked.UpdatedAt = now
	picked.StartedAt = &now
	s.agentTasks[picked.ID] = picked
	return picked, true
}

func (s *MemoryStore) CompleteAgentTask(id string, result map[string]any, errMsg string) (models.AgentTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.agentTasks[id]
	if !ok {
		return models.AgentTask{}, false
	}
	now := time.Now().UTC()
	task.UpdatedAt = now
	task.FinishedAt = &now
	task.Result = result
	task.Error = errMsg
	if errMsg != "" {
		task.Status = models.AgentTaskFailed
	} else {
		task.Status = models.AgentTaskCompleted
	}
	s.agentTasks[id] = task
	return task, true
}
