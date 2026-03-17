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
	r.HandleFunc("/healthz", s.handleHealth)
	r.HandleFunc("/auth/login", s.handleLogin)
	r.HandleFunc("/auth/mfa/verify", s.handleMFAVerify)
	r.HandleFunc("/auth/reauth", s.handleReauth)
	r.HandleFunc("/cluster/overview", s.withAuth(s.handleClusterOverview))
	r.HandleFunc("/nodes", s.withAuth(s.handleNodes))
	r.HandleFunc("/nodes/heartbeat", s.withAuth(s.handleNodeHeartbeat))
	r.HandleFunc("/vms", s.withAuth(s.handleVMs))
	r.HandleFunc("/storage/inventory", s.withAuth(s.handleStorageInventory))
	r.HandleFunc("/storage/rebuild/plan", s.withAuth(s.handleStorageRebuildPlan))
	r.HandleFunc("/backup/policies", s.withAuth(s.handleBackupPolicies))
	r.HandleFunc("/backup/targets", s.withAuth(s.handleBackupTargets))
	r.HandleFunc("/jobs", s.withAuth(s.handleJobs))
	r.HandleFunc("/jobs/", s.withAuth(s.handleJobByID))
	r.HandleFunc("/audit", s.withAuth(s.handleAudit))
	r.HandleFunc("/incidents", s.withAuth(s.handleIncidents))
	r.HandleFunc("/policy/simulate", s.withAuth(s.handlePolicySimulate))
	r.HandleFunc("/policy/explain", s.withAuth(s.handlePolicyExplain))
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
	s.handler = loggingMiddleware(r)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

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
		"guarded":     []string{"storage.plan_apply", "network.plan_apply", "updates.canary_start", "node.runner.exec", "proxmaster.self_migrate", "ssh.breakglass.enable", "vpn.wireguard.apply"},
		"health_gate": s.gateEval.Explain(snap),
	})
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
