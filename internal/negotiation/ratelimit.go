package negotiation

import (
	"sync"
	"time"
)

type sessionRateState struct {
	previewTimestamps []time.Time
	lastCommitTime    time.Time
}

// RateLimiter manages in-memory request limits per session ID.
type RateLimiter struct {
	mu             sync.Mutex
	sessions       map[string]*sessionRateState
	previewLimit   int
	previewWindow  time.Duration
	commitCooldown time.Duration
}

func NewRateLimiter(commitCooldownSeconds int) *RateLimiter {
	if commitCooldownSeconds <= 0 {
		commitCooldownSeconds = 30
	}
	return &RateLimiter{
		sessions:       make(map[string]*sessionRateState),
		previewLimit:   60,
		previewWindow:  10 * time.Minute,
		commitCooldown: time.Duration(commitCooldownSeconds) * time.Second,
	}
}

func (r *RateLimiter) getSessionState(sessionID string) *sessionRateState {
	s, exists := r.sessions[sessionID]
	if !exists {
		s = &sessionRateState{
			previewTimestamps: make([]time.Time, 0, 10),
		}
		r.sessions[sessionID] = s
	}
	return s
}

// AllowPreview checks if a preview request is allowed under the 60/10m limit.
func (r *RateLimiter) AllowPreview(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	s := r.getSessionState(sessionID)

	// Purge timestamps older than previewWindow
	cutoff := now.Add(-r.previewWindow)
	valid := s.previewTimestamps[:0]
	for _, t := range s.previewTimestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	s.previewTimestamps = valid

	if len(s.previewTimestamps) >= r.previewLimit {
		return false
	}

	s.previewTimestamps = append(s.previewTimestamps, now)
	return true
}

// AllowCommit reports whether the cooldown since the last successful COMMIT has elapsed.
// It does not consume the cooldown; call MarkCommit once the commit actually succeeded.
func (r *RateLimiter) AllowCommit(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	s := r.getSessionState(sessionID)
	return s.lastCommitTime.IsZero() || time.Since(s.lastCommitTime) >= r.commitCooldown
}

// MarkCommit records a successful COMMIT, starting the cooldown window.
func (r *RateLimiter) MarkCommit(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getSessionState(sessionID).lastCommitTime = time.Now()
}
