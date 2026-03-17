package gitops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	RepoDir        string
	Branch         string
	ComposeFile    string
	EnvFile        string
	HealthURL      string
	RollbackOnFail bool
}

type Service struct {
	cfg              Config
	mu               sync.Mutex
	running          bool
	lastRunAt        time.Time
	lastResult       string
	lastError        string
	lastCommit       string
	lastStableCommit string
	lastHealth       string
}

func NewService(cfg Config) *Service {
	if strings.TrimSpace(cfg.Branch) == "" {
		cfg.Branch = "main"
	}
	if strings.TrimSpace(cfg.HealthURL) == "" {
		cfg.HealthURL = "http://127.0.0.1:8080/healthz"
	}
	return &Service{cfg: cfg}
}

func (s *Service) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) SyncNow(ctx context.Context, actor string) (map[string]any, error) {
	_ = actor
	s.mu.Lock()
	if s.running {
		out := s.statusLocked()
		s.mu.Unlock()
		return out, errors.New("gitops sync already running")
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.lastRunAt = time.Now().UTC()
		s.mu.Unlock()
	}()

	beforeCommit := strings.TrimSpace(s.gitOutput(ctx, "rev-parse", "HEAD"))
	if beforeCommit != "" {
		s.mu.Lock()
		s.lastCommit = beforeCommit
		s.mu.Unlock()
	}

	if err := s.gitRun(ctx, "fetch", "origin", s.cfg.Branch); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}
	if err := s.gitRun(ctx, "checkout", s.cfg.Branch); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}
	if err := s.gitRun(ctx, "pull", "--ff-only", "origin", s.cfg.Branch); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}

	afterCommit := strings.TrimSpace(s.gitOutput(ctx, "rev-parse", "HEAD"))
	if afterCommit == "" {
		afterCommit = beforeCommit
	}
	s.mu.Lock()
	s.lastCommit = afterCommit
	s.mu.Unlock()

	if err := s.composeRun(ctx, "up", "-d"); err != nil {
		return s.rollbackOnFailure(ctx, beforeCommit, fmt.Errorf("compose up failed: %w", err))
	}

	if err := s.healthCheck(ctx); err != nil {
		return s.rollbackOnFailure(ctx, beforeCommit, err)
	}

	s.mu.Lock()
	s.lastResult = "success"
	s.lastError = ""
	s.lastStableCommit = afterCommit
	s.lastHealth = "healthy"
	s.mu.Unlock()
	return s.Status(), nil
}

func (s *Service) RollbackLastStable(ctx context.Context) (map[string]any, error) {
	s.mu.Lock()
	if s.running {
		out := s.statusLocked()
		s.mu.Unlock()
		return out, errors.New("gitops sync already running")
	}
	if strings.TrimSpace(s.lastStableCommit) == "" {
		out := s.statusLocked()
		s.mu.Unlock()
		return out, errors.New("no stable commit recorded")
	}
	commit := s.lastStableCommit
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.lastRunAt = time.Now().UTC()
		s.mu.Unlock()
	}()

	if err := s.gitRun(ctx, "checkout", commit); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}
	if err := s.composeRun(ctx, "up", "-d"); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}
	if err := s.healthCheck(ctx); err != nil {
		s.markFailed(err)
		return s.Status(), err
	}

	s.mu.Lock()
	s.lastCommit = commit
	s.lastResult = "rolled_back"
	s.lastError = ""
	s.lastHealth = "healthy_after_rollback"
	s.mu.Unlock()
	return s.Status(), nil
}

func (s *Service) rollbackOnFailure(ctx context.Context, rollbackCommit string, rootErr error) (map[string]any, error) {
	s.markFailed(rootErr)
	if !s.cfg.RollbackOnFail || strings.TrimSpace(rollbackCommit) == "" {
		return s.Status(), rootErr
	}
	_ = s.gitRun(ctx, "checkout", rollbackCommit)
	_ = s.composeRun(ctx, "up", "-d")
	if err := s.healthCheck(ctx); err == nil {
		s.mu.Lock()
		s.lastResult = "rolled_back"
		s.lastError = rootErr.Error()
		s.lastCommit = rollbackCommit
		s.lastHealth = "healthy_after_rollback"
		s.mu.Unlock()
	}
	return s.Status(), rootErr
}

func (s *Service) gitRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.cfg.RepoDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) gitOutput(ctx context.Context, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.cfg.RepoDir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *Service) composeRun(ctx context.Context, args ...string) error {
	composeArgs := []string{"compose", "-f", s.cfg.ComposeFile}
	if strings.TrimSpace(s.cfg.EnvFile) != "" {
		composeArgs = append(composeArgs, "--env-file", s.cfg.EnvFile)
	}
	composeArgs = append(composeArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", composeArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) healthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.HealthURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) markFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastResult = "failed"
	s.lastError = err.Error()
	s.lastHealth = "unhealthy"
}

func (s *Service) statusLocked() map[string]any {
	return map[string]any{
		"running":            s.running,
		"last_run_at_utc":    timeOrEmpty(s.lastRunAt),
		"last_result":        s.lastResult,
		"last_error":         s.lastError,
		"last_commit":        s.lastCommit,
		"last_stable_commit": s.lastStableCommit,
		"last_health":        s.lastHealth,
		"repo_dir":           s.cfg.RepoDir,
		"branch":             s.cfg.Branch,
		"compose_file":       s.cfg.ComposeFile,
		"health_url":         s.cfg.HealthURL,
		"rollback_on_fail":   s.cfg.RollbackOnFail,
	}
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
