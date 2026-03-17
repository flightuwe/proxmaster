package connectivity

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	wgInterface string
}

func NewService(wgInterface string) *Service {
	if strings.TrimSpace(wgInterface) == "" {
		wgInterface = "wg0"
	}
	return &Service{wgInterface: wgInterface}
}

func (s *Service) Status(ctx context.Context) map[string]any {
	out := map[string]any{
		"wireguard_interface": s.wgInterface,
		"checked_at_utc":      time.Now().UTC().Format(time.RFC3339),
	}

	if _, err := exec.LookPath("wg"); err != nil {
		out["wireguard_available"] = false
		out["tunnel_up"] = false
		out["reason"] = "wg binary not found"
		if ipState := s.interfaceState(ctx); ipState != "" {
			out["interface_state"] = ipState
			out["tunnel_up"] = strings.Contains(strings.ToUpper(ipState), "UP")
		}
		out["peers"] = []map[string]any{}
		out["peer_count"] = 0
		return out
	}

	cmd := exec.CommandContext(ctx, "wg", "show", s.wgInterface, "dump")
	raw, err := cmd.Output()
	if err != nil {
		out["wireguard_available"] = true
		out["tunnel_up"] = false
		out["reason"] = "wg interface unavailable or no permission"
		out["error"] = err.Error()
		out["peers"] = []map[string]any{}
		out["peer_count"] = 0
		return out
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	peers := make([]map[string]any, 0, len(lines))
	tunnelUp := false
	now := time.Now().Unix()
	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if idx == 0 {
			continue
		}
		if len(fields) < 8 {
			continue
		}
		latestHandshake, _ := strconv.ParseInt(fields[5], 10, 64)
		rxBytes, _ := strconv.ParseInt(fields[6], 10, 64)
		txBytes, _ := strconv.ParseInt(fields[7], 10, 64)
		age := int64(-1)
		if latestHandshake > 0 {
			age = now - latestHandshake
			if age < 180 {
				tunnelUp = true
			}
		}
		peers = append(peers, map[string]any{
			"public_key":               fields[0],
			"endpoint":                 fields[2],
			"allowed_ips":              fields[3],
			"latest_handshake_age_sec": age,
			"rx_bytes":                 rxBytes,
			"tx_bytes":                 txBytes,
		})
	}
	out["wireguard_available"] = true
	out["tunnel_up"] = tunnelUp
	out["interface_state"] = s.interfaceState(ctx)
	out["peers"] = peers
	out["peer_count"] = len(peers)
	return out
}

func (s *Service) interfaceState(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "ip", "-brief", "addr", "show", s.wgInterface)
	raw, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
