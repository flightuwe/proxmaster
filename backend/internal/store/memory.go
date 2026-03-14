package store

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"proxmaster/backend/internal/models"
)

type MemoryStore struct {
	mu     sync.RWMutex
	jobs   map[string]models.Job
	audits []models.AuditEvent
	state  models.ClusterState
	seq    uint64
}

func NewMemoryStore() *MemoryStore {
	now := time.Now().UTC()
	return &MemoryStore{
		jobs: make(map[string]models.Job),
		audits: make([]models.AuditEvent, 0, 32),
		state: models.ClusterState{
			Nodes: []models.Node{
				{ID: "node-1", Name: "pve-node-1", Status: "online", Maintenance: false},
				{ID: "node-2", Name: "pve-node-2", Status: "online", Maintenance: false},
				{ID: "node-3", Name: "pve-node-3", Status: "online", Maintenance: false},
				{ID: "node-4", Name: "pve-node-4", Status: "online", Maintenance: false},
			},
			VMs: []models.VM{
				{ID: "100", NodeID: "node-1", Name: "proxmaster-mgmt", Power: "running", Priority: 100},
				{ID: "101", NodeID: "node-1", Name: "db-prod-1", Power: "running", Priority: 90},
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

func (s *MemoryStore) CreateJob(tool string, input map[string]any, risk models.RiskLevel, status models.JobStatus) models.Job {
	now := time.Now().UTC()
	job := models.Job{
		ID:        s.nextID("job"),
		Tool:      tool,
		Input:     input,
		Risk:      risk,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return job
}

func (s *MemoryStore) UpdateJob(job models.Job) {
	job.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

func (s *MemoryStore) GetJob(id string) (models.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
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