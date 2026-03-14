package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
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
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
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
