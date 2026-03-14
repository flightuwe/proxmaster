package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"proxmaster/backend/internal/models"
)

// PostgresStore keeps the in-memory operational view for fast local orchestration
// and durably persists jobs/audit/incidents for recovery and replay.
type PostgresStore struct {
	mem *MemoryStore
	db  *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	s := &PostgresStore{mem: NewMemoryStore(), db: db}
	if err := s.initSchema(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) initSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			idempotency_key TEXT UNIQUE,
			tool TEXT NOT NULL,
			input_json JSONB NOT NULL,
			risk TEXT NOT NULL,
			decision TEXT NOT NULL,
			required_approvals INT NOT NULL,
			rollback_plan_id TEXT,
			status TEXT NOT NULL,
			status_history JSONB NOT NULL,
			result_json JSONB,
			error TEXT,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			action TEXT NOT NULL,
			actor TEXT NOT NULL,
			risk TEXT NOT NULL,
			approved BOOL NOT NULL,
			metadata_json JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS incidents (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		)`,
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) persistJob(ctx context.Context, job models.Job) {
	inputJSON, _ := json.Marshal(job.Input)
	historyJSON, _ := json.Marshal(job.StatusHistory)
	resultJSON, _ := json.Marshal(job.Result)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs (
			id, idempotency_key, tool, input_json, risk, decision, required_approvals,
			rollback_plan_id, status, status_history, result_json, error, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			idempotency_key=EXCLUDED.idempotency_key,
			tool=EXCLUDED.tool,
			input_json=EXCLUDED.input_json,
			risk=EXCLUDED.risk,
			decision=EXCLUDED.decision,
			required_approvals=EXCLUDED.required_approvals,
			rollback_plan_id=EXCLUDED.rollback_plan_id,
			status=EXCLUDED.status,
			status_history=EXCLUDED.status_history,
			result_json=EXCLUDED.result_json,
			error=EXCLUDED.error,
			updated_at=EXCLUDED.updated_at
	`, job.ID, nullable(job.IdempotencyKey), job.Tool, inputJSON, string(job.Risk), string(job.Decision),
		job.RequiredApprovals, nullable(job.RollbackPlanID), string(job.Status), historyJSON, nullJSON(resultJSON), nullable(job.Error), job.CreatedAt, job.UpdatedAt)
	if err != nil {
		log.Printf("warn: failed to persist job %s: %v", job.ID, err)
	}
}

func (s *PostgresStore) persistAudit(ctx context.Context, e models.AuditEvent) {
	metaJSON, _ := json.Marshal(e.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (id, action, actor, risk, approved, metadata_json, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO NOTHING
	`, e.ID, e.Action, e.Actor, string(e.Risk), e.Approved, metaJSON, e.CreatedAt)
	if err != nil {
		log.Printf("warn: failed to persist audit %s: %v", e.ID, err)
	}
}

func (s *PostgresStore) persistIncident(ctx context.Context, i models.Incident) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO incidents (id, kind, severity, message, created_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO NOTHING
	`, i.ID, i.Kind, i.Severity, i.Message, i.CreatedAt)
	if err != nil {
		log.Printf("warn: failed to persist incident %s: %v", i.ID, err)
	}
}

func (s *PostgresStore) CreateJob(job models.Job) models.Job {
	created := s.mem.CreateJob(job)
	s.persistJob(context.Background(), created)
	return created
}

func (s *PostgresStore) UpdateJob(job models.Job) {
	s.mem.UpdateJob(job)
	updated, ok := s.mem.GetJob(job.ID)
	if ok {
		s.persistJob(context.Background(), updated)
	}
}

func (s *PostgresStore) GetJob(id string) (models.Job, bool) { return s.mem.GetJob(id) }
func (s *PostgresStore) GetJobByIdempotencyKey(key string) (models.Job, bool) {
	return s.mem.GetJobByIdempotencyKey(key)
}
func (s *PostgresStore) ListJobs() []models.Job                        { return s.mem.ListJobs() }
func (s *PostgresStore) ClusterState() models.ClusterState             { return s.mem.ClusterState() }
func (s *PostgresStore) SetNodeMaintenance(nodeID string, maintenance bool) bool {
	return s.mem.SetNodeMaintenance(nodeID, maintenance)
}
func (s *PostgresStore) MarkNodeHeartbeat(nodeID string) bool          { return s.mem.MarkNodeHeartbeat(nodeID) }
func (s *PostgresStore) MigrateVM(vmID, targetNode string) bool        { return s.mem.MigrateVM(vmID, targetNode) }
func (s *PostgresStore) CreateVM(vm models.VM) models.VM               { return s.mem.CreateVM(vm) }
func (s *PostgresStore) CreateLXC(vm models.VM) models.VM              { return s.mem.CreateLXC(vm) }
func (s *PostgresStore) CloneVM(templateID, newID, targetNode, name string) (models.VM, bool) {
	return s.mem.CloneVM(templateID, newID, targetNode, name)
}
func (s *PostgresStore) ApplyPool(name, poolType string)               { s.mem.ApplyPool(name, poolType) }
func (s *PostgresStore) ApplyNetwork(name, kind, cidr string)          { s.mem.ApplyNetwork(name, kind, cidr) }

func (s *PostgresStore) AddAudit(action, actor string, risk models.RiskLevel, approved bool, meta map[string]any) models.AuditEvent {
	e := s.mem.AddAudit(action, actor, risk, approved, meta)
	s.persistAudit(context.Background(), e)
	return e
}

func (s *PostgresStore) ListAudit() []models.AuditEvent {
	return s.mem.ListAudit()
}

func (s *PostgresStore) RecordIncident(kind, severity, message string) models.Incident {
	incident := s.mem.RecordIncident(kind, severity, message)
	s.persistIncident(context.Background(), incident)
	return incident
}

func (s *PostgresStore) ListIncidents() []models.Incident {
	return s.mem.ListIncidents()
}

func (s *PostgresStore) SyncStorageInventory() map[string]any {
	return s.mem.SyncStorageInventory()
}

func (s *PostgresStore) PlanRebuildAllPools() models.StorageRebuildPlan {
	return s.mem.PlanRebuildAllPools()
}

func (s *PostgresStore) ExecuteRebuildAllPools(planID string) (models.StorageRebuildPlan, bool) {
	return s.mem.ExecuteRebuildAllPools(planID)
}

func (s *PostgresStore) ApplyReplicationPolicy(policy models.ReplicationPolicy) models.ReplicationPolicy {
	return s.mem.ApplyReplicationPolicy(policy)
}

func (s *PostgresStore) UpsertBackupTarget(target models.BackupTarget) models.BackupTarget {
	return s.mem.UpsertBackupTarget(target)
}

func (s *PostgresStore) ListBackupTargets() []models.BackupTarget {
	return s.mem.ListBackupTargets()
}

func (s *PostgresStore) UpsertBackupPolicy(policy models.BackupPolicy) models.BackupPolicy {
	return s.mem.UpsertBackupPolicy(policy)
}

func (s *PostgresStore) ListBackupPolicies() []models.BackupPolicy {
	return s.mem.ListBackupPolicies()
}

func (s *PostgresStore) ExplainBackupPolicy(workloadID string) (models.BackupPolicy, models.BackupDecisionLog, bool) {
	return s.mem.ExplainBackupPolicy(workloadID)
}

func (s *PostgresStore) RunBackupNow(workloadID string) map[string]any {
	return s.mem.RunBackupNow(workloadID)
}

func (s *PostgresStore) PlanRestore(workloadID, targetID string) models.RestorePlan {
	return s.mem.PlanRestore(workloadID, targetID)
}

func (s *PostgresStore) ExecuteRestore(planID string) (models.RestorePlan, bool) {
	return s.mem.ExecuteRestore(planID)
}

func (s *PostgresStore) VerifyBackupSample() map[string]any {
	return s.mem.VerifyBackupSample()
}

func nullable(sv string) any {
	if sv == "" {
		return nil
	}
	return sv
}

func nullJSON(b []byte) any {
	if string(b) == "null" {
		return nil
	}
	return b
}
