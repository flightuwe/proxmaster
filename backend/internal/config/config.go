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
	WireGuardInterface    string
	GitOpsRepoDir         string
	GitOpsBranch          string
	GitOpsComposeFile     string
	GitOpsEnvFile         string
	GitOpsHealthURL       string
	GitOpsRollbackOnFail  bool
	BreakglassEnableCmd   string
	BreakglassDisableCmd  string
	BreakglassDefaultMin  int
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
		WireGuardInterface:    envOr("PROXMASTER_WIREGUARD_INTERFACE", "wg0"),
		GitOpsRepoDir:         envOr("PROXMASTER_GITOPS_REPO_DIR", "/opt/proxmaster"),
		GitOpsBranch:          envOr("PROXMASTER_GITOPS_BRANCH", "main"),
		GitOpsComposeFile:     envOr("PROXMASTER_GITOPS_COMPOSE_FILE", "/opt/proxmaster/infra/docker-compose.yml"),
		GitOpsEnvFile:         envOr("PROXMASTER_GITOPS_ENV_FILE", "/opt/proxmaster/infra/.env"),
		GitOpsHealthURL:       envOr("PROXMASTER_GITOPS_HEALTH_URL", "http://127.0.0.1:8080/healthz"),
		GitOpsRollbackOnFail:  envOr("PROXMASTER_GITOPS_ROLLBACK_ON_FAIL", "true") == "true",
		BreakglassEnableCmd:   envOr("PROXMASTER_BREAKGLASS_ENABLE_CMD", ""),
		BreakglassDisableCmd:  envOr("PROXMASTER_BREAKGLASS_DISABLE_CMD", ""),
		BreakglassDefaultMin:  envOrInt("PROXMASTER_BREAKGLASS_DEFAULT_MIN", 60),
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
