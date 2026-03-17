package proxmoxapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL     string
	TokenID     string
	TokenSecret string
	InsecureTLS bool
	Enabled     bool
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type ClusterResource struct {
	Type    string  `json:"type"`
	Node    string  `json:"node"`
	ID      string  `json:"id"`
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	CPUs    float64 `json:"cpus"`
	MaxMem  float64 `json:"maxmem"`
	MaxDisk float64 `json:"maxdisk"`
	MaxCPU  float64 `json:"maxcpu"`
	MaxMemN float64 `json:"maxmemn"`
}

type StorageEntry struct {
	Storage string  `json:"storage"`
	Type    string  `json:"type"`
	Shared  int     `json:"shared"`
	Enabled int     `json:"enabled"`
	Total   float64 `json:"total"`
	Used    float64 `json:"used"`
}

func New(cfg Config) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureTLS},
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: tr,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.cfg.Enabled && c.cfg.BaseURL != "" && c.cfg.TokenID != "" && c.cfg.TokenSecret != ""
}

func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return fmt.Errorf("proxmox api client not enabled")
	}
	_, err := c.do(ctx, http.MethodGet, "/version", nil)
	return err
}

func (c *Client) ClusterResources(ctx context.Context, typ string) ([]ClusterResource, error) {
	path := "/cluster/resources"
	if typ != "" {
		path += "?type=" + url.QueryEscape(typ)
	}
	raw, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var out []ClusterResource
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) StorageList(ctx context.Context) ([]StorageEntry, error) {
	raw, err := c.do(ctx, http.MethodGet, "/storage", nil)
	if err != nil {
		return nil, err
	}
	var out []StorageEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) MigrateQemuVM(ctx context.Context, sourceNode string, vmID int, targetNode string, online bool) error {
	values := url.Values{}
	values.Set("target", targetNode)
	if online {
		values.Set("online", "1")
	} else {
		values.Set("online", "0")
	}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/migrate", url.PathEscape(sourceNode), vmID)
	_, err := c.do(ctx, http.MethodPost, path, values)
	return err
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	endpoint := base + path

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.cfg.TokenID, c.cfg.TokenSecret))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("proxmox api %s %s failed: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var env apiEnvelope
	if err := json.Unmarshal(rawBody, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

func ParseVMID(id string) (int, bool) {
	if id == "" {
		return 0, false
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return 0, false
	}
	return n, true
}
