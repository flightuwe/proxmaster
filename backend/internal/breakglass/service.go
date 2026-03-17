package breakglass

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Service struct {
	mu                sync.Mutex
	enabled           bool
	enabledUntil      time.Time
	lastChangedBy     string
	lastChangedAt     time.Time
	lastError         string
	enableCmd         string
	disableCmd        string
	defaultDuration   time.Duration
	lastAutoDisableAt time.Time
}

func NewService(enableCmd, disableCmd string, defaultDurationMin int) *Service {
	if defaultDurationMin <= 0 {
		defaultDurationMin = 60
	}
	return &Service{
		enableCmd:       strings.TrimSpace(enableCmd),
		disableCmd:      strings.TrimSpace(disableCmd),
		defaultDuration: time.Duration(defaultDurationMin) * time.Minute,
	}
}

func (s *Service) Enable(ctx context.Context, duration time.Duration, actor string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(actor) == "" {
		actor = "admin"
	}
	if duration <= 0 {
		duration = s.defaultDuration
	}
	if err := s.runHook(ctx, s.enableCmd); err != nil {
		s.lastError = err.Error()
		return s.statusLocked(), err
	}

	now := time.Now().UTC()
	s.enabled = true
	s.enabledUntil = now.Add(duration)
	s.lastChangedBy = actor
	s.lastChangedAt = now
	s.lastError = ""
	return s.statusLocked(), nil
}

func (s *Service) Disable(ctx context.Context, actor string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(actor) == "" {
		actor = "admin"
	}
	if err := s.runHook(ctx, s.disableCmd); err != nil {
		s.lastError = err.Error()
		return s.statusLocked(), err
	}
	now := time.Now().UTC()
	s.enabled = false
	s.enabledUntil = time.Time{}
	s.lastChangedBy = actor
	s.lastChangedAt = now
	s.lastError = ""
	return s.statusLocked(), nil
}

func (s *Service) Status(ctx context.Context) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoDisableIfExpiredLocked(ctx)
	return s.statusLocked()
}

func (s *Service) runHook(ctx context.Context, cmdText string) error {
	if strings.TrimSpace(cmdText) == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdText)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func (s *Service) autoDisableIfExpiredLocked(ctx context.Context) {
	if !s.enabled {
		return
	}
	now := time.Now().UTC()
	if now.Before(s.enabledUntil) {
		return
	}
	_ = s.runHook(ctx, s.disableCmd)
	s.enabled = false
	s.lastAutoDisableAt = now
}

func (s *Service) statusLocked() map[string]any {
	remaining := int64(0)
	if s.enabled {
		remaining = int64(time.Until(s.enabledUntil).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}
	return map[string]any{
		"enabled":                  s.enabled,
		"enabled_until_utc":        timeOrEmpty(s.enabledUntil),
		"remaining_seconds":        remaining,
		"last_changed_by":          s.lastChangedBy,
		"last_changed_at_utc":      timeOrEmpty(s.lastChangedAt),
		"last_auto_disable_at_utc": timeOrEmpty(s.lastAutoDisableAt),
		"last_error":               s.lastError,
	}
}

func timeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
