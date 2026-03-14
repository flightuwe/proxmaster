package main

import (
	"log"
	"net/http"

	"proxmaster/backend/internal/api"
	"proxmaster/backend/internal/config"
	"proxmaster/backend/internal/mcp"
	"proxmaster/backend/internal/orchestrator"
	"proxmaster/backend/internal/policy"
	"proxmaster/backend/internal/proxmox"
	"proxmaster/backend/internal/risk"
	"proxmaster/backend/internal/store"
)

func main() {
	cfg := config.Load()
	memStore := store.NewMemoryStore()
	px := proxmox.NewClient(memStore)
	orch := orchestrator.New(px)
	riskEngine := risk.NewEngine()
	policyGate := policy.NewGate()
	mcpSvc := mcp.NewService(memStore, riskEngine, policyGate, orch)

	srv := api.NewServer(cfg, memStore, mcpSvc)
	log.Printf("proxmaster api listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}