package store

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"proxmaster/backend/internal/models"
)

type MemoryStore struct {
	mu            sync.RWMutex
	jobs          map[string]models.Job
	byIdemKey     map[string]string
	audits        []models.AuditEvent
	incidents     []models.Incident
	state         models.ClusterState
	seq           uint64
	vmIDSeq       uint64
	rollbackIDSeq uint64
}

func NewMemoryStore() *MemoryStore {
	now := time.Now().UTC()
	return &MemoryStore{
		jobs:      make(map[string]models.Job),
		byIdemKey: make(map[string]string),
		audits:    make([]models.AuditEvent, 0, 64),
		incidents: make([]models.Incident, 0, 32),
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
			},
			Pools: []models.StoragePool{
				{Name: "rbd-main", Type: "ceph", Status: "healthy"},
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
			clone.ID = newID
			if clone.ID == "" {
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
	s.state.Pools = append(s.state.Pools, models.StoragePool{Name: name, Type: poolType, Status: "healthy"})
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
