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
