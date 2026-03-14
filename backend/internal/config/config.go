package config

import "os"

type Config struct {
	ListenAddr      string
	AdminToken      string
	EnableDangerOps bool
}

func Load() Config {
	cfg := Config{
		ListenAddr:      envOr("PROXMASTER_LISTEN_ADDR", ":8080"),
		AdminToken:      envOr("PROXMASTER_ADMIN_TOKEN", "dev-admin-token"),
		EnableDangerOps: envOr("PROXMASTER_ENABLE_DANGER_OPS", "false") == "true",
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}