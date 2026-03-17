package vpn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type WireGuardConfig struct {
	Interface      string
	ConfigPath     string
	KeysDir        string
	DefaultPort    int
	DefaultSubnet  string
	DefaultSrvIP   string
	DefaultPeerIPs string
}

type WireGuardService struct {
	cfg WireGuardConfig
}

func NewWireGuardService(cfg WireGuardConfig) *WireGuardService {
	if strings.TrimSpace(cfg.Interface) == "" {
		cfg.Interface = "wg0"
	}
	if strings.TrimSpace(cfg.ConfigPath) == "" {
		cfg.ConfigPath = "/etc/wireguard/" + cfg.Interface + ".conf"
	}
	if strings.TrimSpace(cfg.KeysDir) == "" {
		cfg.KeysDir = "/etc/proxmaster/wireguard"
	}
	if cfg.DefaultPort <= 0 {
		cfg.DefaultPort = 51820
	}
	if strings.TrimSpace(cfg.DefaultSubnet) == "" {
		cfg.DefaultSubnet = "10.13.13.0/24"
	}
	if strings.TrimSpace(cfg.DefaultSrvIP) == "" {
		cfg.DefaultSrvIP = "10.13.13.2/24"
	}
	if strings.TrimSpace(cfg.DefaultPeerIPs) == "" {
		cfg.DefaultPeerIPs = "10.13.13.1/32"
	}
	return &WireGuardService{cfg: cfg}
}

func (s *WireGuardService) Plan(_ context.Context, params map[string]any) (map[string]any, error) {
	listenPort := intFrom(params["listen_port"], s.cfg.DefaultPort)
	serverAddress := stringFrom(params["server_address"], s.cfg.DefaultSrvIP)
	peerAllowedIPs := stringFrom(params["peer_allowed_ips"], s.cfg.DefaultPeerIPs)
	if peerAllowedIPs == "" {
		peerAllowedIPs = s.cfg.DefaultPeerIPs
	}
	endpoint := stringFrom(params["server_endpoint"], "<public-ip-or-dns>:51820")
	clientPrivate := "<CLIENT_PRIVATE_KEY>"
	serverPublic := "<SERVER_PUBLIC_KEY_AFTER_APPLY>"

	clientConf := fmt.Sprintf(`[Interface]
Address = %s
PrivateKey = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, peerAllowedIPs, clientPrivate, serverPublic, endpoint, serverAddress)

	return map[string]any{
		"changed": false,
		"plan": map[string]any{
			"interface":             s.cfg.Interface,
			"server_config_path":    s.cfg.ConfigPath,
			"keys_dir":              s.cfg.KeysDir,
			"listen_port":           listenPort,
			"server_address":        serverAddress,
			"peer_allowed_ips":      peerAllowedIPs,
			"required_client_input": []string{"client_public_key", "server_endpoint"},
			"commands": []string{
				"apt-get update && apt-get install -y wireguard wireguard-tools",
				fmt.Sprintf("systemctl enable wg-quick@%s", s.cfg.Interface),
				fmt.Sprintf("systemctl restart wg-quick@%s", s.cfg.Interface),
				"wg show",
			},
			"client_config_example": clientConf,
		},
	}, nil
}

func (s *WireGuardService) Apply(ctx context.Context, params map[string]any) (map[string]any, error) {
	clientPublicKey := strings.TrimSpace(stringFrom(params["client_public_key"], ""))
	if clientPublicKey == "" {
		return nil, errors.New("missing client_public_key")
	}

	listenPort := intFrom(params["listen_port"], s.cfg.DefaultPort)
	serverAddress := stringFrom(params["server_address"], s.cfg.DefaultSrvIP)
	peerAllowedIPs := stringFrom(params["peer_allowed_ips"], s.cfg.DefaultPeerIPs)
	serverEndpoint := stringFrom(params["server_endpoint"], "")

	if err := s.ensureWireGuardTools(ctx); err != nil {
		return nil, err
	}
	privateKey, publicKey, err := s.ensureKeys(ctx)
	if err != nil {
		return nil, err
	}
	conf := fmt.Sprintf(`[Interface]
Address = %s
ListenPort = %d
PrivateKey = %s

[Peer]
PublicKey = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, serverAddress, listenPort, privateKey, clientPublicKey, peerAllowedIPs)

	if err := os.MkdirAll(filepath.Dir(s.cfg.ConfigPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.cfg.ConfigPath, []byte(conf), 0o600); err != nil {
		return nil, err
	}
	if err := s.exec(ctx, "systemctl", "enable", "wg-quick@"+s.cfg.Interface); err != nil {
		return nil, err
	}
	if err := s.exec(ctx, "systemctl", "restart", "wg-quick@"+s.cfg.Interface); err != nil {
		return nil, err
	}

	clientConfig := ""
	if strings.TrimSpace(serverEndpoint) != "" {
		clientConfig = fmt.Sprintf(`[Interface]
Address = %s
PrivateKey = <CLIENT_PRIVATE_KEY>

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, peerAllowedIPs, publicKey, serverEndpoint, serverAddress)
	}

	return map[string]any{
		"changed":            true,
		"interface":          s.cfg.Interface,
		"config_path":        s.cfg.ConfigPath,
		"server_public_key":  publicKey,
		"server_address":     serverAddress,
		"peer_allowed_ips":   peerAllowedIPs,
		"applied_at_utc":     time.Now().UTC().Format(time.RFC3339),
		"client_config_hint": clientConfig,
	}, nil
}

func (s *WireGuardService) Status(ctx context.Context) (map[string]any, error) {
	out := map[string]any{
		"interface":       s.cfg.Interface,
		"config_path":     s.cfg.ConfigPath,
		"config_exists":   fileExists(s.cfg.ConfigPath),
		"checked_at_utc":  time.Now().UTC().Format(time.RFC3339),
		"wireguard_tools": false,
	}
	if _, err := exec.LookPath("wg"); err != nil {
		out["reason"] = "wg binary not found"
		return out, nil
	}
	out["wireguard_tools"] = true
	showOut, err := exec.CommandContext(ctx, "wg", "show", s.cfg.Interface, "dump").Output()
	if err != nil {
		out["tunnel_up"] = false
		out["reason"] = "interface not active"
		return out, nil
	}
	lines := strings.Split(strings.TrimSpace(string(showOut)), "\n")
	peerCount := 0
	if len(lines) > 1 {
		peerCount = len(lines) - 1
	}
	out["tunnel_up"] = true
	out["peer_count"] = peerCount
	return out, nil
}

func (s *WireGuardService) ensureWireGuardTools(ctx context.Context) error {
	if _, err := exec.LookPath("wg"); err == nil {
		return nil
	}
	if err := s.exec(ctx, "apt-get", "update"); err != nil {
		return err
	}
	return s.exec(ctx, "apt-get", "install", "-y", "wireguard", "wireguard-tools")
}

func (s *WireGuardService) ensureKeys(ctx context.Context) (privateKey, publicKey string, err error) {
	if err := os.MkdirAll(s.cfg.KeysDir, 0o700); err != nil {
		return "", "", err
	}
	privPath := filepath.Join(s.cfg.KeysDir, "privatekey")
	pubPath := filepath.Join(s.cfg.KeysDir, "publickey")

	if !fileExists(privPath) || !fileExists(pubPath) {
		gen := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("umask 077; wg genkey | tee %s | wg pubkey > %s", shellEscape(privPath), shellEscape(pubPath)))
		if out, gerr := gen.CombinedOutput(); gerr != nil {
			return "", "", fmt.Errorf("failed to generate keys: %s", strings.TrimSpace(string(out)))
		}
	}
	privBytes, err := os.ReadFile(privPath)
	if err != nil {
		return "", "", err
	}
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(privBytes)), strings.TrimSpace(string(pubBytes)), nil
}

func (s *WireGuardService) exec(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func shellEscape(in string) string {
	return "'" + strings.ReplaceAll(in, "'", "'\\''") + "'"
}

func intFrom(v any, fallback int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return fallback
	}
}

func stringFrom(v any, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
