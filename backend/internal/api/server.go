package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"proxmaster/backend/internal/config"
	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.MemoryStore
	mcpSvc  *mcp.Service
	handler http.Handler
}

func NewServer(cfg config.Config, st *store.MemoryStore, mcpSvc *mcp.Service) *Server {
	s := &Server{cfg: cfg, store: st, mcpSvc: mcpSvc}
	r := http.NewServeMux()
	r.HandleFunc("/healthz", s.handleHealth)
	r.HandleFunc("/auth/login", s.handleLogin)
	r.HandleFunc("/auth/mfa/verify", s.handleMFAVerify)
	r.HandleFunc("/auth/reauth", s.handleReauth)
	r.HandleFunc("/cluster/overview", s.withAuth(s.handleClusterOverview))
	r.HandleFunc("/nodes", s.withAuth(s.handleNodes))
	r.HandleFunc("/vms", s.withAuth(s.handleVMs))
	r.HandleFunc("/jobs", s.withAuth(s.handleJobs))
	r.HandleFunc("/jobs/", s.withAuth(s.handleJobByID))
	r.HandleFunc("/audit", s.withAuth(s.handleAudit))
	r.HandleFunc("/mcp/call", s.withAuth(s.handleMCPCall))
	r.HandleFunc("/mcp/approve", s.withAuth(s.handleMCPApprove))
	s.handler = loggingMiddleware(r)
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mfa_required": true,
		"challenge_id":  "dev-challenge",
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
	writeJSON(w, http.StatusOK, map[string]any{"cluster": s.store.ClusterState()})
}

func (s *Server) handleNodes(w http.ResponseWriter, _ *http.Request) {
	state := s.store.ClusterState()
	writeJSON(w, http.StatusOK, map[string]any{"nodes": state.Nodes})
}

func (s *Server) handleVMs(w http.ResponseWriter, _ *http.Request) {
	state := s.store.ClusterState()
	writeJSON(w, http.StatusOK, map[string]any{"vms": state.VMs})
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
	if req.Params == nil {
		req.Params = map[string]any{}
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
		Tool        string                 `json:"tool"`
		Params      map[string]any         `json:"params"`
		Actor       string                 `json:"actor"`
		ReauthToken string                 `json:"reauth_token"`
		Metadata    map[string]any         `json:"metadata"`
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
		Tool:       req.Tool,
		Params:     req.Params,
		Actor:      req.Actor,
		ApproveNow: true,
		Metadata:   req.Metadata,
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