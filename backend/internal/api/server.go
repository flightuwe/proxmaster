package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proxmaster/backend/internal/breakglass"
	"proxmaster/backend/internal/config"
	"proxmaster/backend/internal/connectivity"
	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/gitops"
	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/store"
)

type Server struct {
	cfg      config.Config
	store    store.Store
	mcpSvc   *mcp.Service
	gateEval *health.GateEvaluator
	cp       *controlplane.Manager
	connSvc  *connectivity.Service
	gitops   *gitops.Service
	bgs      *breakglass.Service
	handler  http.Handler
}

func NewServer(cfg config.Config, st store.Store, mcpSvc *mcp.Service, gateEval *health.GateEvaluator, cp *controlplane.Manager, connSvc *connectivity.Service, gitopsSvc *gitops.Service, bgs *breakglass.Service) *Server {
	s := &Server{cfg: cfg, store: st, mcpSvc: mcpSvc, gateEval: gateEval, cp: cp, connSvc: connSvc, gitops: gitopsSvc, bgs: bgs}
	r := http.NewServeMux()
	r.HandleFunc("/", s.handleIndex)
	r.HandleFunc("/webui", s.handleWebUI)
	r.HandleFunc("/healthz", s.handleHealth)
	r.HandleFunc("/auth/login", s.handleLogin)
	r.HandleFunc("/auth/mfa/verify", s.handleMFAVerify)
	r.HandleFunc("/auth/reauth", s.handleReauth)
	r.HandleFunc("/cluster/overview", s.withAuth(s.handleClusterOverview))
	r.HandleFunc("/nodes", s.withAuth(s.handleNodes))
	r.HandleFunc("/nodes/heartbeat", s.withAuth(s.handleNodeHeartbeat))
	r.HandleFunc("/vms", s.withAuth(s.handleVMs))
	r.HandleFunc("/spec/cluster", s.withAuth(s.handleSpecCluster))
	r.HandleFunc("/spec/storage", s.withAuth(s.handleSpecStorage))
	r.HandleFunc("/spec/network", s.withAuth(s.handleSpecNetwork))
	r.HandleFunc("/spec/backup", s.withAuth(s.handleSpecBackup))
	r.HandleFunc("/spec/workloads/", s.withAuth(s.handleSpecWorkloadByID))
	r.HandleFunc("/state/cluster", s.withAuth(s.handleStateCluster))
	r.HandleFunc("/state/storage", s.withAuth(s.handleStateStorage))
	r.HandleFunc("/state/network", s.withAuth(s.handleStateNetwork))
	r.HandleFunc("/state/backup", s.withAuth(s.handleStateBackup))
	r.HandleFunc("/state/workloads", s.withAuth(s.handleStateWorkloads))
	r.HandleFunc("/state/all", s.withAuth(s.handleStateAll))
	r.HandleFunc("/blueprints", s.withAuth(s.handleBlueprints))
	r.HandleFunc("/blueprints/plan", s.withAuth(s.handleBlueprintPlan))
	r.HandleFunc("/blueprints/deploy", s.withAuth(s.handleBlueprintDeploy))
	r.HandleFunc("/blueprints/verify", s.withAuth(s.handleBlueprintVerify))
	r.HandleFunc("/blueprints/update", s.withAuth(s.handleBlueprintUpdate))
	r.HandleFunc("/blueprints/rollback", s.withAuth(s.handleBlueprintRollback))
	r.HandleFunc("/storage/inventory", s.withAuth(s.handleStorageInventory))
	r.HandleFunc("/storage/rebuild/plan", s.withAuth(s.handleStorageRebuildPlan))
	r.HandleFunc("/backup/policies", s.withAuth(s.handleBackupPolicies))
	r.HandleFunc("/backup/targets", s.withAuth(s.handleBackupTargets))
	r.HandleFunc("/jobs", s.withAuth(s.handleJobs))
	r.HandleFunc("/jobs/", s.withAuth(s.handleJobByID))
	r.HandleFunc("/jobs/timeline", s.withAuth(s.handleJobsTimeline))
	r.HandleFunc("/audit", s.withAuth(s.handleAudit))
	r.HandleFunc("/incidents", s.withAuth(s.handleIncidents))
	r.HandleFunc("/autonomy/tasks", s.withAuth(s.handleAutonomyTasks))
	r.HandleFunc("/autonomy/tasks/", s.withAuth(s.handleAutonomyTaskByID))
	r.HandleFunc("/policy/simulate", s.withAuth(s.handlePolicySimulate))
	r.HandleFunc("/policy/explain", s.withAuth(s.handlePolicyExplain))
	r.HandleFunc("/policy/mode", s.withAuth(s.handlePolicyMode))
	r.HandleFunc("/controlplane/endpoint", s.withAuth(s.handleControlPlaneEndpoint))
	r.HandleFunc("/connectivity/status", s.withAuth(s.handleConnectivityStatus))
	r.HandleFunc("/gitops/status", s.withAuth(s.handleGitOpsStatus))
	r.HandleFunc("/gitops/sync", s.withAuth(s.handleGitOpsSync))
	r.HandleFunc("/gitops/rollback", s.withAuth(s.handleGitOpsRollback))
	r.HandleFunc("/access/breakglass", s.withAuth(s.handleBreakglassStatus))
	r.HandleFunc("/access/breakglass/enable", s.withAuth(s.handleBreakglassEnable))
	r.HandleFunc("/access/breakglass/disable", s.withAuth(s.handleBreakglassDisable))
	r.HandleFunc("/vpn/wireguard/status", s.withAuth(s.handleWireGuardStatus))
	r.HandleFunc("/vpn/wireguard/plan", s.withAuth(s.handleWireGuardPlan))
	r.HandleFunc("/vpn/wireguard/apply", s.withAuth(s.handleWireGuardApply))
	r.HandleFunc("/mcp/call", s.withAuth(s.handleMCPCall))
	r.HandleFunc("/mcp/approve", s.withAuth(s.handleMCPApprove))
	r.HandleFunc("/terminal/exec", s.withAuth(s.handleTerminalExec))
	s.handler = loggingMiddleware(r)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "proxmaster-api",
		"status":  "ok",
		"hint":    "API reachable. Use /healthz for liveness and authenticated endpoints for control.",
		"endpoints": map[string]string{
			"health":        "/healthz",
			"overview":      "/cluster/overview",
			"webui":         "/webui",
			"connectivity":  "/connectivity/status",
			"wireguard":     "/vpn/wireguard/status",
			"gitops_status": "/gitops/status",
		},
	})
}

func (s *Server) handleWebUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Proxmaster WebUI</title>
  <style>
    body { font-family: -apple-system, Segoe UI, Roboto, sans-serif; margin: 0; background: #0b1220; color: #e5e7eb; }
    .wrap { max-width: 980px; margin: 0 auto; padding: 24px; }
    .row { display: grid; grid-template-columns: repeat(auto-fit,minmax(220px,1fr)); gap: 10px; margin: 16px 0; }
    button { background: #1f2937; color: #e5e7eb; border: 1px solid #374151; padding: 10px 12px; border-radius: 8px; cursor: pointer; }
    input { width: 100%; background: #111827; color: #e5e7eb; border: 1px solid #374151; padding: 8px; border-radius: 8px; }
    pre { background: #020617; border: 1px solid #1f2937; border-radius: 8px; padding: 12px; white-space: pre-wrap; }
  </style>
</head>
<body>
  <div class="wrap">
    <h2>Proxmaster WebUI</h2>
    <input id="token" placeholder="Bearer token" />
    <div class="row">
      <button onclick="call('GET','/state/all')">State All</button>
      <button onclick="call('GET','/blueprints')">Blueprints</button>
      <button onclick="call('GET','/jobs/timeline')">Timeline</button>
      <button onclick="call('GET','/policy/mode')">Policy Mode</button>
    </div>
    <div class="row">
      <button onclick="call('POST','/blueprints/plan',{name:'pfsense-gateway',node_id:'node-1',workload_name:'pfsense-gw'})">Plan pfSense</button>
      <button onclick="call('POST','/blueprints/deploy',{name:'pfsense-gateway',node_id:'node-1',workload_name:'pfsense-gw'})">Deploy pfSense</button>
      <button onclick="call('PUT','/spec/workloads/pfsense-gw',{id:'pfsense-gw',name:'pfsense-gw',kind:'vm',node_id:'node-1',cpu:2,memory_mb:4096,disk_gb:20,desired_power:'running'})">Spec pfSense</button>
      <button onclick="call('POST','/policy/mode',{mode:'AGGRESSIVE_AUTO',duration_minutes:30,reauth_token:'reauth-ok',hardware_mfa:true,second_approver:'web-admin'})">Aggressive 30m</button>
    </div>
    <pre id="out">ready</pre>
  </div>
  <script>
    const out = document.getElementById('out');
    const token = document.getElementById('token');
    function headers() {
      const h = { 'Content-Type': 'application/json' };
      if (token.value.trim()) h['Authorization'] = 'Bearer ' + token.value.trim();
      return h;
    }
    async function call(method, path, body) {
      try {
        const res = await fetch(path, { method, headers: headers(), body: body ? JSON.stringify(body) : undefined });
        const txt = await res.text();
        out.textContent = txt;
      } catch (e) {
        out.textContent = 'error: ' + e;
      }
    }
  </script>
</body>
</html>`))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	snap := s.gateEval.SnapshotFromState(s.store.ClusterState())
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"time":        time.Now().UTC(),
		"health_gate": s.gateEval.Explain(snap),
		"controlplane": map[string]any{
			"mode":         s.cp.Mode(),
			"endpoint":     s.cp.Endpoint(),
			"current_node": s.cp.CurrentNode(),
		},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mfa_required": true,
		"challenge_id": "dev-challenge",
	})
}

func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": s.cfg.AdminToken,
		"expires_in":   900,
	})
}

func (s *Server) handleReauth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reauth_token": "reauth-ok",
		"expires_in":   120,
	})
}

func (s *Server) handleClusterOverview(w http.ResponseWriter, _ *http.Request) {
	snap := s.gateEval.SnapshotFromState(s.store.ClusterState())
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster":     s.store.ClusterState(),
		"health_gate": s.gateEval.Explain(snap),
		"controlplane": map[string]any{
			"mode":         s.cp.Mode(),
			"endpoint":     s.cp.Endpoint(),
			"current_node": s.cp.CurrentNode(),
		},
	})
}

func (s *Server) handleNodes(w http.ResponseWriter, _ *http.Request) {
	state := s.store.ClusterState()
	writeJSON(w, http.StatusOK, map[string]any{"nodes": state.Nodes})
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.NodeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing node_id"})
		return
	}
	if ok := s.store.MarkNodeHeartbeat(req.NodeID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "node not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": true, "node_id": req.NodeID})
}

func (s *Server) handleVMs(w http.ResponseWriter, _ *http.Request) {
	state := s.store.ClusterState()
	writeJSON(w, http.StatusOK, map[string]any{"vms": state.VMs})
}

func (s *Server) handleSpecCluster(w http.ResponseWriter, r *http.Request) {
	s.handleSpecScope(w, r, "cluster", "default")
}

func (s *Server) handleSpecStorage(w http.ResponseWriter, r *http.Request) {
	s.handleSpecScope(w, r, "storage", "default")
}

func (s *Server) handleSpecNetwork(w http.ResponseWriter, r *http.Request) {
	s.handleSpecScope(w, r, "network", "default")
}

func (s *Server) handleSpecBackup(w http.ResponseWriter, r *http.Request) {
	s.handleSpecScope(w, r, "backup", "default")
}

func (s *Server) handleSpecWorkloadByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/spec/workloads/")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing workload id"})
		return
	}
	s.handleSpecScope(w, r, "workloads", id)
}

func (s *Server) handleSpecScope(w http.ResponseWriter, r *http.Request, scope, key string) {
	switch r.Method {
	case http.MethodGet:
		spec, ok := s.store.GetSpec(scope, key)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "spec not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scope": scope, "key": key, "spec": spec})
	case http.MethodPut:
		var desired map[string]any
		if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
			Tool: "workload.spec.apply",
			Params: map[string]any{
				"scope": scope,
				"key":   key,
				"spec":  desired,
			},
			Actor: "android-admin",
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleStateCluster(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"observed": s.store.ClusterState(),
		"spec":     s.store.ListSpecs("cluster"),
	})
}

func (s *Server) handleStateStorage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"observed": s.store.SyncStorageInventory(),
		"spec":     s.store.ListSpecs("storage"),
	})
}

func (s *Server) handleStateNetwork(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"observed": s.store.ClusterState().Networks,
		"spec":     s.store.ListSpecs("network"),
	})
}

func (s *Server) handleStateBackup(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"observed": map[string]any{
			"policies": s.store.ListBackupPolicies(),
			"targets":  s.store.ListBackupTargets(),
		},
		"spec": s.store.ListSpecs("backup"),
	})
}

func (s *Server) handleStateWorkloads(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"observed": s.store.ClusterState().VMs,
		"spec":     s.store.ListSpecs("workloads"),
	})
}

func (s *Server) handleStateAll(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster":   s.store.ClusterState(),
		"desired":   s.store.DesiredStateBundle(),
		"workloads": s.store.ListSpecs("workloads"),
		"policy":    s.store.GetPolicyMode(),
	})
}

func (s *Server) handleBlueprints(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{
			"catalog":        s.store.ListBlueprints(),
			"deployed_specs": s.store.ListBlueprintSpecs(),
		})
		return
	}
	writeMethodNotAllowed(w)
}

func (s *Server) handleBlueprintPlan(w http.ResponseWriter, r *http.Request) {
	s.handleBlueprintMCP(w, r, "blueprint.plan")
}

func (s *Server) handleBlueprintDeploy(w http.ResponseWriter, r *http.Request) {
	s.handleBlueprintMCP(w, r, "blueprint.deploy")
}

func (s *Server) handleBlueprintVerify(w http.ResponseWriter, r *http.Request) {
	s.handleBlueprintMCP(w, r, "blueprint.verify")
}

func (s *Server) handleBlueprintUpdate(w http.ResponseWriter, r *http.Request) {
	s.handleBlueprintMCP(w, r, "blueprint.update")
}

func (s *Server) handleBlueprintRollback(w http.ResponseWriter, r *http.Request) {
	s.handleBlueprintMCP(w, r, "blueprint.rollback")
}

func (s *Server) handleBlueprintMCP(w http.ResponseWriter, r *http.Request, tool string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var params map[string]any
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:   tool,
		Params: params,
		Actor:  "android-admin",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleStorageInventory(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.SyncStorageInventory())
}

func (s *Server) handleStorageRebuildPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	plan := s.store.PlanRebuildAllPools()
	writeJSON(w, http.StatusOK, map[string]any{"plan": plan, "requires_approval": true})
}

func (s *Server) handleBackupPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"policies": s.store.ListBackupPolicies()})
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req models.BackupPolicy
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.WorkloadID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "workload_id is required"})
		return
	}
	p := s.store.UpsertBackupPolicy(req)
	writeJSON(w, http.StatusOK, map[string]any{"policy": p})
}

func (s *Server) handleBackupTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"targets": s.store.ListBackupTargets()})
}

func (s *Server) handleJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.store.ListJobs()})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing job id"})
		return
	}
	job, ok := s.store.GetJob(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleAudit(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": s.store.ListAudit()})
}

func (s *Server) handleIncidents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"incidents": s.store.ListIncidents()})
}

func (s *Server) handleJobsTimeline(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":             s.store.ListJobs(),
		"tasks":            s.store.ListAgentTasks(),
		"audit":            s.store.ListAudit(),
		"incidents":        s.store.ListIncidents(),
		"generated_at_utc": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleAutonomyTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": s.store.ListAgentTasks()})
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Type        string         `json:"type"`
		Payload     map[string]any `json:"payload"`
		RequestedBy string         `json:"requested_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing type"})
		return
	}
	task := s.store.CreateAgentTask(models.AgentTask{
		Type:        strings.TrimSpace(req.Type),
		Payload:     req.Payload,
		RequestedBy: firstNonEmpty(req.RequestedBy, "android-admin"),
	})
	writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}

func (s *Server) handleAutonomyTaskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/autonomy/tasks/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing task id"})
		return
	}
	task, ok := s.store.GetAgentTask(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) handlePolicySimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req models.PolicySimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	resp := s.mcpSvc.SimulatePolicy(req)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePolicyExplain(w http.ResponseWriter, _ *http.Request) {
	snap := s.gateEval.SnapshotFromState(s.store.ClusterState())
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":        "SRE_MODE_FAIL_CLOSED",
		"policy_mode": s.store.GetPolicyMode(),
		"guarded":     []string{"storage.plan_apply", "network.plan_apply", "updates.canary_start", "node.runner.exec", "proxmaster.self_migrate", "ssh.breakglass.enable", "vpn.wireguard.apply"},
		"health_gate": s.gateEval.Explain(snap),
	})
}

func (s *Server) handlePolicyMode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.store.GetPolicyMode())
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Mode           string `json:"mode"`
		DurationMin    int    `json:"duration_minutes"`
		ReauthToken    string `json:"reauth_token"`
		SecondApprover string `json:"second_approver"`
		HardwareMFA    bool   `json:"hardware_mfa"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	approveNow := req.ReauthToken == "reauth-ok"
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool: "policy.mode.set",
		Params: map[string]any{
			"mode":             req.Mode,
			"duration_minutes": req.DurationMin,
			"actor":            "android-admin",
		},
		Actor:          "android-admin",
		ApproveNow:     approveNow,
		SecondApprover: req.SecondApprover,
		HardwareMFA:    req.HardwareMFA,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleControlPlaneEndpoint(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":         s.cp.Mode(),
		"endpoint":     s.cp.Endpoint(),
		"current_node": s.cp.CurrentNode(),
	})
}

func (s *Server) handleConnectivityStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	out := map[string]any{
		"checked_at_utc": time.Now().UTC().Format(time.RFC3339),
	}
	if s.connSvc != nil {
		out["wireguard"] = s.connSvc.Status(r.Context())
	}
	if s.gitops != nil {
		out["gitops"] = s.gitops.Status()
	}
	if s.bgs != nil {
		out["breakglass"] = s.bgs.Status(r.Context())
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGitOpsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	if s.gitops == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "gitops not configured"})
		return
	}
	writeJSON(w, http.StatusOK, s.gitops.Status())
}

func (s *Server) handleGitOpsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor          string `json:"actor"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:           "gitops.sync.now",
		Params:         map[string]any{"actor": req.Actor},
		Actor:          req.Actor,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGitOpsRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor          string `json:"actor"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:           "gitops.rollback",
		Params:         map[string]any{"actor": req.Actor},
		Actor:          req.Actor,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBreakglassStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	if s.bgs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "breakglass not configured"})
		return
	}
	writeJSON(w, http.StatusOK, s.bgs.Status(r.Context()))
}

func (s *Server) handleBreakglassEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor          string `json:"actor"`
		DurationMin    int    `json:"duration_minutes"`
		ReauthToken    string `json:"reauth_token"`
		SecondApprover string `json:"second_approver"`
		HardwareMFA    bool   `json:"hardware_mfa"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	approveNow := req.ReauthToken == "reauth-ok"
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:           "ssh.breakglass.enable",
		Params:         map[string]any{"duration_minutes": req.DurationMin, "actor": req.Actor},
		Actor:          req.Actor,
		ApproveNow:     approveNow,
		SecondApprover: req.SecondApprover,
		HardwareMFA:    req.HardwareMFA,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBreakglassDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor          string `json:"actor"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:           "ssh.breakglass.disable",
		Params:         map[string]any{"actor": req.Actor},
		Actor:          req.Actor,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWireGuardStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool:   "vpn.wireguard.status",
		Params: map[string]any{},
		Actor:  "android-admin",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWireGuardPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor          string `json:"actor"`
		ServerAddress  string `json:"server_address"`
		PeerAllowedIPs string `json:"peer_allowed_ips"`
		ListenPort     int    `json:"listen_port"`
		ServerEndpoint string `json:"server_endpoint"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool: "vpn.wireguard.plan",
		Params: map[string]any{
			"server_address":   req.ServerAddress,
			"peer_allowed_ips": req.PeerAllowedIPs,
			"listen_port":      req.ListenPort,
			"server_endpoint":  req.ServerEndpoint,
		},
		Actor:          req.Actor,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWireGuardApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Actor           string `json:"actor"`
		ClientPublicKey string `json:"client_public_key"`
		ServerAddress   string `json:"server_address"`
		PeerAllowedIPs  string `json:"peer_allowed_ips"`
		ListenPort      int    `json:"listen_port"`
		ServerEndpoint  string `json:"server_endpoint"`
		ReauthToken     string `json:"reauth_token"`
		SecondApprover  string `json:"second_approver"`
		HardwareMFA     bool   `json:"hardware_mfa"`
		IdempotencyKey  string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Actor == "" {
		req.Actor = "android-admin"
	}
	resp, err := s.mcpSvc.HandleCall(r.Context(), models.MCPCallRequest{
		Tool: "vpn.wireguard.apply",
		Params: map[string]any{
			"client_public_key": req.ClientPublicKey,
			"server_address":    req.ServerAddress,
			"peer_allowed_ips":  req.PeerAllowedIPs,
			"listen_port":       req.ListenPort,
			"server_endpoint":   req.ServerEndpoint,
		},
		Actor:          req.Actor,
		ApproveNow:     req.ReauthToken == "reauth-ok",
		SecondApprover: req.SecondApprover,
		HardwareMFA:    req.HardwareMFA,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMCPCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req models.MCPCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Tool == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "tool is required"})
		return
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	resp, err := s.mcpSvc.HandleCall(context.Background(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMCPApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		Tool           string         `json:"tool"`
		Params         map[string]any `json:"params"`
		Actor          string         `json:"actor"`
		ReauthToken    string         `json:"reauth_token"`
		SecondApprover string         `json:"second_approver"`
		HardwareMFA    bool           `json:"hardware_mfa"`
		IdempotencyKey string         `json:"idempotency_key"`
		Metadata       map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.ReauthToken != "reauth-ok" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "missing or invalid reauth token"})
		return
	}
	resp, err := s.mcpSvc.HandleCall(context.Background(), models.MCPCallRequest{
		Tool:           req.Tool,
		Params:         req.Params,
		Actor:          req.Actor,
		ApproveNow:     true,
		ReauthToken:    req.ReauthToken,
		SecondApprover: req.SecondApprover,
		HardwareMFA:    req.HardwareMFA,
		IdempotencyKey: req.IdempotencyKey,
		Metadata:       req.Metadata,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleTerminalExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req struct {
		NodeID         string `json:"node_id"`
		Script         string `json:"script"`
		Actor          string `json:"actor"`
		ReauthToken    string `json:"reauth_token"`
		SecondApprover string `json:"second_approver"`
		HardwareMFA    bool   `json:"hardware_mfa"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = "node-1"
	}
	if strings.TrimSpace(req.Script) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing script"})
		return
	}
	if strings.TrimSpace(req.Actor) == "" {
		req.Actor = "cli-admin"
	}
	resp, err := s.mcpSvc.HandleCall(context.Background(), models.MCPCallRequest{
		Tool: "node.runner.exec",
		Params: map[string]any{
			"node_id": req.NodeID,
			"command": "shell_script",
			"args": map[string]any{
				"script": req.Script,
			},
		},
		Actor:          req.Actor,
		ApproveNow:     req.ReauthToken == "reauth-ok",
		ReauthToken:    req.ReauthToken,
		SecondApprover: req.SecondApprover,
		HardwareMFA:    req.HardwareMFA,
		IdempotencyKey: firstNonEmpty(req.IdempotencyKey, r.Header.Get("Idempotency-Key")),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if tok == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing bearer token"})
			return
		}
		if !strings.EqualFold(tok, s.cfg.AdminToken) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "invalid token"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": errors.New("method not allowed").Error()})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}
