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
		`CREATE TABLE IF NOT EXISTS agent_tasks (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			payload_json JSONB NOT NULL,
			status TEXT NOT NULL,
			requested_by TEXT NOT NULL,
			priority INT NOT NULL DEFAULT 50,
			max_attempts INT NOT NULL DEFAULT 3,
			dead_letter BOOL NOT NULL DEFAULT FALSE,
			result_json JSONB,
			error TEXT,
			attempts INT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS specs (
			scope TEXT NOT NULL,
			spec_key TEXT NOT NULL,
			desired_json JSONB NOT NULL,
			observed_json JSONB NOT NULL,
			drift_status TEXT NOT NULL,
			last_reconcile_job_id TEXT,
			spec_version INT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (scope, spec_key)
		)`,
		`CREATE TABLE IF NOT EXISTS policy_mode (
			id SMALLINT PRIMARY KEY,
			mode TEXT NOT NULL,
			aggressive_until TIMESTAMPTZ,
			last_changed_by TEXT,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS blueprint_specs (
			name TEXT PRIMARY KEY,
			version TEXT NOT NULL,
			spec_json JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`ALTER TABLE agent_tasks ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 50`,
		`ALTER TABLE agent_tasks ADD COLUMN IF NOT EXISTS max_attempts INT NOT NULL DEFAULT 3`,
		`ALTER TABLE agent_tasks ADD COLUMN IF NOT EXISTS dead_letter BOOL NOT NULL DEFAULT FALSE`,
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

func (s *PostgresStore) persistAgentTask(ctx context.Context, t models.AgentTask) {
	payloadJSON, _ := json.Marshal(t.Payload)
	resultJSON, _ := json.Marshal(t.Result)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tasks (
			id, type, payload_json, status, requested_by, priority, max_attempts, dead_letter, result_json, error, attempts,
			created_at, updated_at, started_at, finished_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO UPDATE SET
			type=EXCLUDED.type,
			payload_json=EXCLUDED.payload_json,
			status=EXCLUDED.status,
			requested_by=EXCLUDED.requested_by,
			priority=EXCLUDED.priority,
			max_attempts=EXCLUDED.max_attempts,
			dead_letter=EXCLUDED.dead_letter,
			result_json=EXCLUDED.result_json,
			error=EXCLUDED.error,
			attempts=EXCLUDED.attempts,
			updated_at=EXCLUDED.updated_at,
			started_at=EXCLUDED.started_at,
			finished_at=EXCLUDED.finished_at
	`, t.ID, t.Type, payloadJSON, string(t.Status), t.RequestedBy, t.Priority, t.MaxAttempts, t.DeadLetter, nullJSON(resultJSON), nullable(t.Error), t.Attempts,
		t.CreatedAt, t.UpdatedAt, t.StartedAt, t.FinishedAt)
	if err != nil {
		log.Printf("warn: failed to persist task %s: %v", t.ID, err)
	}
}

func (s *PostgresStore) persistSpec(ctx context.Context, scope, key string, spec models.ResourceSpec) {
	desiredJSON, _ := json.Marshal(spec.Desired)
	observedJSON, _ := json.Marshal(spec.Observed)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO specs (scope, spec_key, desired_json, observed_json, drift_status, last_reconcile_job_id, spec_version, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (scope, spec_key) DO UPDATE SET
			desired_json=EXCLUDED.desired_json,
			observed_json=EXCLUDED.observed_json,
			drift_status=EXCLUDED.drift_status,
			last_reconcile_job_id=EXCLUDED.last_reconcile_job_id,
			spec_version=EXCLUDED.spec_version,
			updated_at=EXCLUDED.updated_at
	`, scope, key, desiredJSON, observedJSON, string(spec.DriftStatus), nullable(spec.LastReconcileJobID), spec.SpecVersion, spec.UpdatedAt)
	if err != nil {
		log.Printf("warn: failed to persist spec %s/%s: %v", scope, key, err)
	}
}

func (s *PostgresStore) persistPolicyMode(ctx context.Context, st models.PolicyModeState) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policy_mode (id, mode, aggressive_until, last_changed_by, updated_at)
		VALUES (1,$1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET
			mode=EXCLUDED.mode,
			aggressive_until=EXCLUDED.aggressive_until,
			last_changed_by=EXCLUDED.last_changed_by,
			updated_at=EXCLUDED.updated_at
	`, string(st.Mode), nullableTime(st.AggressiveUntil), nullable(st.LastChangedBy), st.UpdatedAt)
	if err != nil {
		log.Printf("warn: failed to persist policy mode: %v", err)
	}
}

func (s *PostgresStore) persistBlueprintSpec(ctx context.Context, spec models.ServiceBlueprintSpec) {
	raw, _ := json.Marshal(spec)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO blueprint_specs (name, version, spec_json, updated_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (name) DO UPDATE SET
			version=EXCLUDED.version,
			spec_json=EXCLUDED.spec_json,
			updated_at=EXCLUDED.updated_at
	`, spec.Name, spec.Version, raw, time.Now().UTC())
	if err != nil {
		log.Printf("warn: failed to persist blueprint spec %s: %v", spec.Name, err)
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
func (s *PostgresStore) ListJobs() []models.Job            { return s.mem.ListJobs() }
func (s *PostgresStore) ClusterState() models.ClusterState { return s.mem.ClusterState() }
func (s *PostgresStore) SetNodeMaintenance(nodeID string, maintenance bool) bool {
	return s.mem.SetNodeMaintenance(nodeID, maintenance)
}
func (s *PostgresStore) MarkNodeHeartbeat(nodeID string) bool { return s.mem.MarkNodeHeartbeat(nodeID) }
func (s *PostgresStore) MigrateVM(vmID, targetNode string) bool {
	return s.mem.MigrateVM(vmID, targetNode)
}
func (s *PostgresStore) CreateVM(vm models.VM) models.VM  { return s.mem.CreateVM(vm) }
func (s *PostgresStore) CreateLXC(vm models.VM) models.VM { return s.mem.CreateLXC(vm) }
func (s *PostgresStore) CloneVM(templateID, newID, targetNode, name string) (models.VM, bool) {
	return s.mem.CloneVM(templateID, newID, targetNode, name)
}
func (s *PostgresStore) ApplyPool(name, poolType string)      { s.mem.ApplyPool(name, poolType) }
func (s *PostgresStore) ApplyNetwork(name, kind, cidr string) { s.mem.ApplyNetwork(name, kind, cidr) }

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

func (s *PostgresStore) CreateAgentTask(task models.AgentTask) models.AgentTask {
	created := s.mem.CreateAgentTask(task)
	s.persistAgentTask(context.Background(), created)
	return created
}

func (s *PostgresStore) ListAgentTasks() []models.AgentTask {
	return s.mem.ListAgentTasks()
}

func (s *PostgresStore) GetAgentTask(id string) (models.AgentTask, bool) {
	return s.mem.GetAgentTask(id)
}

func (s *PostgresStore) ClaimNextAgentTask(worker string) (models.AgentTask, bool) {
	task, ok := s.mem.ClaimNextAgentTask(worker)
	if ok {
		s.persistAgentTask(context.Background(), task)
	}
	return task, ok
}

func (s *PostgresStore) CompleteAgentTask(id string, result map[string]any, errMsg string) (models.AgentTask, bool) {
	task, ok := s.mem.CompleteAgentTask(id, result, errMsg)
	if ok {
		s.persistAgentTask(context.Background(), task)
	}
	return task, ok
}

func (s *PostgresStore) UpsertSpec(scope string, key string, desired map[string]any) models.ResourceSpec {
	spec := s.mem.UpsertSpec(scope, key, desired)
	s.persistSpec(context.Background(), scope, key, spec)
	return spec
}

func (s *PostgresStore) GetSpec(scope string, key string) (models.ResourceSpec, bool) {
	return s.mem.GetSpec(scope, key)
}

func (s *PostgresStore) ListSpecs(scope string) map[string]models.ResourceSpec {
	return s.mem.ListSpecs(scope)
}

func (s *PostgresStore) SetSpecObserved(scope string, key string, observed map[string]any, drift models.DriftStatus, reconcileJobID string) (models.ResourceSpec, bool) {
	spec, ok := s.mem.SetSpecObserved(scope, key, observed, drift, reconcileJobID)
	if ok {
		s.persistSpec(context.Background(), scope, key, spec)
	}
	return spec, ok
}

func (s *PostgresStore) DesiredStateBundle() models.DesiredStateBundle {
	return s.mem.DesiredStateBundle()
}

func (s *PostgresStore) ListBlueprints() []models.BlueprintDefinition {
	return s.mem.ListBlueprints()
}

func (s *PostgresStore) GetBlueprint(name string) (models.BlueprintDefinition, bool) {
	return s.mem.GetBlueprint(name)
}

func (s *PostgresStore) UpsertBlueprint(spec models.ServiceBlueprintSpec) models.ServiceBlueprintSpec {
	out := s.mem.UpsertBlueprint(spec)
	s.persistBlueprintSpec(context.Background(), out)
	return out
}

func (s *PostgresStore) ListBlueprintSpecs() []models.ServiceBlueprintSpec {
	return s.mem.ListBlueprintSpecs()
}

func (s *PostgresStore) GetPolicyMode() models.PolicyModeState {
	return s.mem.GetPolicyMode()
}

func (s *PostgresStore) SetPolicyMode(mode models.PolicyMode, actor string, duration time.Duration) models.PolicyModeState {
	st := s.mem.SetPolicyMode(mode, actor, duration)
	s.persistPolicyMode(context.Background(), st)
	return st
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

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
