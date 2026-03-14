package controlplane

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Mode string

const (
	ModeVIP Mode = "vip"
	ModeDNS Mode = "dns"
)

type Config struct {
	Mode        Mode
	VIP         string
	DNSName     string
	APIPort     int
	InitialNode string
}

type SwitchResult struct {
	Mode                Mode      `json:"mode"`
	Endpoint            string    `json:"endpoint"`
	FromNode            string    `json:"from_node"`
	ToNode              string    `json:"to_node"`
	Switched            bool      `json:"switched"`
	ReconnectSeconds    int       `json:"reconnect_seconds"`
	DNSPropagationHint  string    `json:"dns_propagation_hint,omitempty"`
	CompletedAt         time.Time `json:"completed_at"`
}

type Manager struct {
	mu          sync.RWMutex
	cfg         Config
	currentNode string
}

func NewManager(cfg Config) *Manager {
	if cfg.Mode == "" {
		cfg.Mode = ModeVIP
	}
	if cfg.APIPort == 0 {
		cfg.APIPort = 8080
	}
	if cfg.VIP == "" {
		cfg.VIP = "127.0.0.1"
	}
	if cfg.DNSName == "" {
		cfg.DNSName = "proxmaster.internal"
	}
	return &Manager{
		cfg:         cfg,
		currentNode: cfg.InitialNode,
	}
}

func (m *Manager) Endpoint() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.endpointLocked()
}

func (m *Manager) Mode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Mode
}

func (m *Manager) CurrentNode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentNode
}

func (m *Manager) SwitchTo(node string) SwitchResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	old := m.currentNode
	m.currentNode = node
	result := SwitchResult{
		Mode:             m.cfg.Mode,
		Endpoint:         m.endpointLocked(),
		FromNode:         old,
		ToNode:           node,
		Switched:         old != node,
		ReconnectSeconds: 20,
		CompletedAt:      time.Now().UTC(),
	}
	if m.cfg.Mode == ModeDNS {
		result.DNSPropagationHint = "ensure low TTL (<=30s) for near-seamless client reconnect"
	}
	return result
}

func (m *Manager) endpointLocked() string {
	host := m.cfg.VIP
	if m.cfg.Mode == ModeDNS {
		host = m.cfg.DNSName
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return fmt.Sprintf("http://%s:%d", host, m.cfg.APIPort)
}
