package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	// defaultRate is the fallback rate when no other source provides one.
	defaultRate = 5.0

	// remainingSlowdownThreshold triggers proactive slowdown when the
	// fraction of remaining requests falls below this value.
	remainingSlowdownThreshold = 0.1

	// slowdownFactor reduces the rate when remaining requests are low.
	slowdownFactor = 0.5
)

// LimiterConfig holds the configuration for an AdaptiveLimiter.
type LimiterConfig struct {
	ExplicitRate    float64
	DefaultRate     float64
	RobotsCrawlDelay time.Duration
}

// AdaptiveLimiter is a thread-safe, adaptive rate limiter that adjusts its
// rate based on a priority hierarchy: Explicit > Headers > robots.txt > Default.
type AdaptiveLimiter struct {
	mu           sync.RWMutex
	limiter      *rate.Limiter
	cfg          LimiterConfig
	effectiveRate float64
	headerRate   float64
	logger       *slog.Logger
}

// NewAdaptiveLimiter creates a new AdaptiveLimiter from the given config.
// The initial rate is determined by the priority hierarchy.
func NewAdaptiveLimiter(cfg LimiterConfig) *AdaptiveLimiter {
	effective := resolveRate(cfg, 0)

	return &AdaptiveLimiter{
		limiter:       rate.NewLimiter(rate.Limit(effective), max(1, int(effective))),
		cfg:           cfg,
		effectiveRate: effective,
		logger:        slog.Default(),
	}
}

// Wait blocks until the rate limiter allows one event, or the context is
// cancelled. Returns a context error if the context expires.
func (a *AdaptiveLimiter) Wait(ctx context.Context) error {
	a.mu.RLock()
	lim := a.limiter
	a.mu.RUnlock()

	if err := lim.Wait(ctx); err != nil {
		return &RateLimitError{Cause: err}
	}

	return nil
}

// UpdateFromHeaders adjusts the rate based on parsed rate limit headers.
// When Remaining approaches 0 relative to Limit, the rate is proactively
// reduced to avoid hitting the limit.
func (a *AdaptiveLimiter) UpdateFromHeaders(headers map[string][]string) {
	info := ParseRateLimitHeaders(headers)
	if !info.Found {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// If an explicit rate is set, headers cannot override it.
	if a.cfg.ExplicitRate > 0 {
		return
	}

	newRate := a.computeHeaderRate(info)
	if newRate <= 0 {
		return
	}

	a.headerRate = newRate
	a.applyRate(resolveRate(a.cfg, a.headerRate))
}

// HandleRetryAfter temporarily slows the limiter for the given duration,
// then restores the previous rate. The slowdown is applied by setting the
// rate to zero (blocking new requests) for the specified duration.
func (a *AdaptiveLimiter) HandleRetryAfter(d time.Duration) {
	if d <= 0 {
		return
	}

	a.mu.Lock()
	previousRate := a.effectiveRate
	a.applyRate(0)
	a.mu.Unlock()

	a.logger.Info("rate limiter paused for retry-after",
		"duration", d,
		"previous_rate", previousRate,
	)

	go func() {
		time.Sleep(d)

		a.mu.Lock()
		defer a.mu.Unlock()

		a.applyRate(previousRate)
		a.logger.Info("rate limiter resumed", "rate", previousRate)
	}()
}

// Rate returns the current effective rate limit.
func (a *AdaptiveLimiter) Rate() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.effectiveRate
}

// computeHeaderRate derives a rate from header information.
// When remaining requests are low relative to the limit, the rate is reduced.
func (a *AdaptiveLimiter) computeHeaderRate(info RateLimitInfo) float64 {
	if info.Limit <= 0 {
		return 0
	}

	baseRate := info.Limit

	// If we know when the window resets, compute a per-second rate.
	if !info.ResetAt.IsZero() {
		window := time.Until(info.ResetAt)
		if window > 0 {
			baseRate = float64(info.Remaining) / window.Seconds()
		}
	}

	// Proactively slow down when remaining is low.
	fraction := float64(info.Remaining) / info.Limit
	if fraction < remainingSlowdownThreshold {
		baseRate *= slowdownFactor
	}

	if baseRate < 0.1 {
		baseRate = 0.1
	}

	return baseRate
}

// applyRate sets a new effective rate on the underlying limiter.
// Must be called while holding a.mu.
func (a *AdaptiveLimiter) applyRate(r float64) {
	a.effectiveRate = r
	a.limiter.SetLimit(rate.Limit(r))
	burst := max(1, int(r))
	a.limiter.SetBurst(burst)
}

// resolveRate determines the effective rate using the priority hierarchy:
// Explicit > headerRate > robots.txt CrawlDelay > Default config > global default.
func resolveRate(cfg LimiterConfig, headerRate float64) float64 {
	if cfg.ExplicitRate > 0 {
		return cfg.ExplicitRate
	}

	if headerRate > 0 {
		return headerRate
	}

	if cfg.RobotsCrawlDelay > 0 {
		return 1.0 / cfg.RobotsCrawlDelay.Seconds()
	}

	if cfg.DefaultRate > 0 {
		return cfg.DefaultRate
	}

	return defaultRate
}

// RateLimitError wraps an underlying error from the rate limiter.
type RateLimitError struct {
	Cause error
}

func (e *RateLimitError) Error() string {
	return "rate limit wait failed: " + e.Cause.Error()
}

func (e *RateLimitError) Unwrap() error {
	return e.Cause
}
