package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type execRequest struct {
	NodeID   string         `json:"node_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args"`
	IssuedAt string         `json:"issued_at"`
	Nonce    string         `json:"nonce"`
}

func main() {
	addr := envOr("RUNNER_LISTEN_ADDR", ":9091")
	sharedSecret := envOr("RUNNER_SHARED_SECRET", "dev-runner-secret")
	nodeID := envOr("RUNNER_NODE_ID", "node-1")
	apiBase := strings.TrimRight(envOr("RUNNER_API_BASE_URL", "http://proxmaster-api:8080"), "/")
	adminToken := envOr("RUNNER_ADMIN_TOKEN", "")
	heartbeatNodes := splitCSV(envOr("RUNNER_HEARTBEAT_NODES", "node-1,node-2,node-3,node-4"))
	heartbeatSec := intEnv("RUNNER_HEARTBEAT_INTERVAL_SEC", 30)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "node_id": nodeID, "time": time.Now().UTC()})
	})
	mux.HandleFunc("/heartbeat", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"node_id": nodeID, "runner_healthy": true, "ts": time.Now().UTC()})
	})
	mux.HandleFunc("/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req execRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.NodeID != nodeID {
			http.Error(w, "node mismatch", http.StatusForbidden)
			return
		}
		if !verifySignature(r.Header.Get("X-Signature"), sharedSecret, req) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
		resp := map[string]any{
			"accepted": true,
			"node_id":  nodeID,
			"command":  req.Command,
			"args":     req.Args,
			"ts":       time.Now().UTC(),
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	log.Printf("runner-agent listening on %s for %s", addr, nodeID)
	if adminToken != "" {
		go heartbeatLoop(apiBase, adminToken, heartbeatNodes, time.Duration(heartbeatSec)*time.Second)
	} else {
		log.Printf("runner-agent heartbeat bridge disabled: RUNNER_ADMIN_TOKEN is empty")
	}
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func heartbeatLoop(apiBase, token string, nodeIDs []string, every time.Duration) {
	if every < 5*time.Second {
		every = 5 * time.Second
	}
	client := &http.Client{Timeout: 6 * time.Second}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	log.Printf("runner-agent heartbeat bridge enabled: every=%s nodes=%v", every, nodeIDs)
	pushHeartbeats(client, apiBase, token, nodeIDs)
	for range ticker.C {
		pushHeartbeats(client, apiBase, token, nodeIDs)
	}
}

func pushHeartbeats(client *http.Client, apiBase, token string, nodeIDs []string) {
	for _, nodeID := range nodeIDs {
		body := map[string]string{"node_id": nodeID}
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, apiBase+"/nodes/heartbeat", bytes.NewReader(raw))
		if err != nil {
			log.Printf("heartbeat request build failed for %s: %v", nodeID, err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("heartbeat post failed for %s: %v", nodeID, err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("heartbeat post returned %d for %s", resp.StatusCode, nodeID)
		}
	}
}

func verifySignature(sigHeader, secret string, req execRequest) bool {
	if secret == "" || sigHeader == "" {
		return false
	}
	payload := req.NodeID + "|" + req.Command + "|" + req.IssuedAt + "|" + req.Nonce
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	got := strings.TrimPrefix(sigHeader, "sha256=")
	return hmac.Equal([]byte(expected), []byte(got))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"node-1"}
	}
	return out
}
