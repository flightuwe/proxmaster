package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"proxmaster/backend/internal/api"
	"proxmaster/backend/internal/breakglass"
	"proxmaster/backend/internal/config"
	"proxmaster/backend/internal/connectivity"
	"proxmaster/backend/internal/controlplane"
	"proxmaster/backend/internal/gitops"
	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/proxmoxapi"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/runner"
	"proxmaster/backend/internal/store"
)

func main() {
	cfg := config.Load()
	st := buildStore(cfg)
	cp := controlplane.NewManager(controlplane.Config{
		Mode:        controlplane.Mode(cfg.ControlPlaneMode),
		VIP:         cfg.ControlPlaneVIP,
		DNSName:     cfg.ControlPlaneDNSName,
		APIPort:     cfg.ControlPlaneAPIPort,
		InitialNode: cfg.ControlPlaneNodeID,
	})
	realAPI := proxmoxapi.New(proxmoxapi.Config{
		BaseURL:     cfg.ProxmoxAPIBaseURL,
		TokenID:     cfg.ProxmoxAPITokenID,
		TokenSecret: cfg.ProxmoxAPITokenSecret,
		InsecureTLS: cfg.ProxmoxInsecureTLS,
		Enabled:     cfg.ProxmoxUseRealAPI,
	})
	if realAPI.Enabled() {
		if err := realAPI.Ping(context.Background()); err != nil {
			log.Printf("warn: proxmox real API enabled but ping failed: %v", err)
		} else {
			log.Printf("proxmox real API connected: %s", cfg.ProxmoxAPIBaseURL)
		}
	}
	px := proxmox.NewClient(st, cp, realAPI)
	connSvc := connectivity.NewService(cfg.WireGuardInterface)
	gitopsSvc := gitops.NewService(gitops.Config{
		RepoDir:        cfg.GitOpsRepoDir,
		Branch:         cfg.GitOpsBranch,
		ComposeFile:    cfg.GitOpsComposeFile,
		EnvFile:        cfg.GitOpsEnvFile,
		HealthURL:      cfg.GitOpsHealthURL,
		RollbackOnFail: cfg.GitOpsRollbackOnFail,
	})
	breakglassSvc := breakglass.NewService(cfg.BreakglassEnableCmd, cfg.BreakglassDisableCmd, cfg.BreakglassDefaultMin)
	runnerCtrl := runner.NewController()
	orch := orchestrator.New(px, runnerCtrl, connSvc, gitopsSvc, breakglassSvc)
	riskEngine := risk.NewEngine()
	policyGate := policy.NewGate()
	gateEval := health.NewGateEvaluator(cfg.FailClosed, cfg.RunnerHeartbeatMaxSec)
	mcpSvc := mcp.NewService(st, riskEngine, policyGate, gateEval, orch)

	srv := api.NewServer(cfg, st, mcpSvc, gateEval, cp, connSvc, gitopsSvc, breakglassSvc)
	log.Printf("proxmaster api listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func buildStore(cfg config.Config) store.Store {
	if cfg.StoreBackend == "postgres" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pg, err := store.NewPostgresStore(ctx, cfg.PostgresDSN)
		if err != nil {
			log.Printf("warn: postgres store unavailable, falling back to memory: %v", err)
			return store.NewMemoryStore()
		}
		log.Printf("using postgres-backed store")
		return pg
	}
	log.Printf("using memory store")
	return store.NewMemoryStore()
}
