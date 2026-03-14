package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"proxmaster/backend/internal/api"
	"proxmaster/backend/internal/config"
	"proxmaster/backend/internal/health"
	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/runner"
	"proxmaster/backend/internal/store"
)

func main() {
	cfg := config.Load()
	st := buildStore(cfg)
	px := proxmox.NewClient(st)
	runnerCtrl := runner.NewController()
	orch := orchestrator.New(px, runnerCtrl)
	riskEngine := risk.NewEngine()
	policyGate := policy.NewGate()
	gateEval := health.NewGateEvaluator(cfg.FailClosed, cfg.RunnerHeartbeatMaxSec)
	mcpSvc := mcp.NewService(st, riskEngine, policyGate, gateEval, orch)

	srv := api.NewServer(cfg, st, mcpSvc, gateEval)
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
