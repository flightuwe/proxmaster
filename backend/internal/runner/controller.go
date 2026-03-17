package runner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type Controller struct {
	allowlist    map[string]bool
	nodeEndpoint map[string]string
	sharedSecret string
	httpClient   *http.Client
}

type execRequest struct {
	NodeID   string         `json:"node_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args"`
	IssuedAt string         `json:"issued_at"`
	Nonce    string         `json:"nonce"`
}

func NewController() *Controller {
	endpoints := map[string]string{
		"node-1": "http://proxmaster-runner-agent:9091",
	}
	if raw := strings.TrimSpace(os.Getenv("PROXMASTER_RUNNER_NODE_ENDPOINTS")); raw != "" {
		for _, pair := range strings.Split(raw, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) != 2 {
				continue
			}
			endpoints[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return &Controller{
		allowlist: map[string]bool{
			"apt_update":        true,
			"apt_upgrade":       true,
			"node_reboot":       true,
			"diagnostics_ping":  true,
			"service_restart":   true,
			"wireguard_install": true,
			"wireguard_status":  true,
			"shell_script":      true,
		},
		nodeEndpoint: endpoints,
		sharedSecret: envOr("RUNNER_SHARED_SECRET", "dev-runner-secret"),
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Controller) Execute(ctx context.Context, nodeID, command string, args map[string]any) (map[string]any, error) {
	cmd := strings.TrimSpace(strings.ToLower(command))
	if !c.allowlist[cmd] {
		return nil, errors.New("command is not allowlisted")
	}
	endpoint := c.nodeEndpoint[nodeID]
	if endpoint == "" {
		endpoint = fmt.Sprintf("http://%s:9091", nodeID)
	}
	reqBody := execRequest{
		NodeID:   nodeID,
		Command:  cmd,
		Args:     args,
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
		Nonce:    fmt.Sprintf("%d", time.Now().UTC().UnixNano()),
	}
	raw, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/exec", strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Signature", "sha256="+c.sign(reqBody))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runner exec failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	out["node_id"] = nodeID
	out["command"] = cmd
	out["endpoint"] = endpoint
	out["executed_at"] = time.Now().UTC().Format(time.RFC3339)
	return out, nil
}

func (c *Controller) sign(req execRequest) string {
	payload := req.NodeID + "|" + req.Command + "|" + req.IssuedAt + "|" + req.Nonce
	mac := hmac.New(sha256.New, []byte(c.sharedSecret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c *Controller) Allowlist() []string {
	out := make([]string, 0, len(c.allowlist))
	for k := range c.allowlist {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Controller) Endpoints() map[string]string {
	out := map[string]string{}
	for k, v := range c.nodeEndpoint {
		out[k] = v
	}
	return out
}
