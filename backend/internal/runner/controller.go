package runner

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Controller struct {
	allowlist map[string]bool
}

func NewController() *Controller {
	return &Controller{
		allowlist: map[string]bool{
			"apt_update":       true,
			"apt_upgrade":      true,
			"node_reboot":      true,
			"diagnostics_ping": true,
			"service_restart":  true,
		},
	}
}

func (c *Controller) Execute(_ context.Context, nodeID, command string, args map[string]any) (map[string]any, error) {
	cmd := strings.TrimSpace(strings.ToLower(command))
	if !c.allowlist[cmd] {
		return nil, errors.New("command is not allowlisted")
	}
	return map[string]any{
		"node_id":     nodeID,
		"command":     cmd,
		"args":        args,
		"status":      "accepted",
		"executed_at": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Controller) Allowlist() []string {
	out := make([]string, 0, len(c.allowlist))
	for k := range c.allowlist {
		out = append(out, k)
	}
	return out
}
