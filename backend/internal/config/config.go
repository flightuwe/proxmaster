package config

import (
	"fmt"
	"os"
)

type Config struct {
	ListenAddr            string
	AdminToken            string
	EnableDangerOps       bool
	StoreBackend          string
	PostgresDSN           string
	FailClosed            bool
	RunnerHeartbeatMaxSec int
	ControlPlaneMode      string
	ControlPlaneVIP       string
	ControlPlaneDNSName   string
	ControlPlaneAPIPort   int
	ControlPlaneNodeID    string
	ProxmoxUseRealAPI     bool
	ProxmoxAPIBaseURL     string
	ProxmoxAPITokenID     string
	ProxmoxAPITokenSecret string
	ProxmoxInsecureTLS    bool
}

func Load() Config {
	cfg := Config{
		ListenAddr:            envOr("PROXMASTER_LISTEN_ADDR", ":8080"),
		AdminToken:            envOr("PROXMASTER_ADMIN_TOKEN", "dev-admin-token"),
		EnableDangerOps:       envOr("PROXMASTER_ENABLE_DANGER_OPS", "false") == "true",
		StoreBackend:          envOr("PROXMASTER_STORE_BACKEND", "memory"),
		PostgresDSN:           envOr("PROXMASTER_POSTGRES_DSN", "postgres://proxmaster:proxmaster@localhost:5432/proxmaster?sslmode=disable"),
		FailClosed:            envOr("PROXMASTER_FAIL_CLOSED", "true") == "true",
		RunnerHeartbeatMaxSec: envOrInt("PROXMASTER_RUNNER_HEARTBEAT_MAX_SEC", 120),
		ControlPlaneMode:      envOr("PROXMASTER_CONTROLPLANE_MODE", "vip"),
		ControlPlaneVIP:       envOr("PROXMASTER_CONTROLPLANE_VIP", "100.100.100.10"),
		ControlPlaneDNSName:   envOr("PROXMASTER_CONTROLPLANE_DNS_NAME", "proxmaster.internal"),
		ControlPlaneAPIPort:   envOrInt("PROXMASTER_CONTROLPLANE_API_PORT", 8080),
		ControlPlaneNodeID:    envOr("PROXMASTER_NODE_ID", "node-1"),
		ProxmoxUseRealAPI:     envOr("PROXMASTER_PROXMOX_USE_REAL_API", "false") == "true",
		ProxmoxAPIBaseURL:     envOr("PROXMASTER_PROXMOX_API_BASE_URL", "https://proxmox-node1:8006/api2/json"),
		ProxmoxAPITokenID:     envOr("PROXMASTER_PROXMOX_API_TOKEN_ID", ""),
		ProxmoxAPITokenSecret: envOr("PROXMASTER_PROXMOX_API_TOKEN_SECRET", ""),
		ProxmoxInsecureTLS:    envOr("PROXMASTER_PROXMOX_INSECURE_TLS", "false") == "true",
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
