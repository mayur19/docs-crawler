package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRate_PriorityHierarchy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        LimiterConfig
		headerRate float64
		expected   float64
	}{
		{
			name:       "explicit rate takes priority",
			cfg:        LimiterConfig{ExplicitRate: 10, DefaultRate: 3, RobotsCrawlDelay: 2 * time.Second},
			headerRate: 7,
			expected:   10,
		},
		{
			name:       "header rate when no explicit",
			cfg:        LimiterConfig{DefaultRate: 3, RobotsCrawlDelay: 2 * time.Second},
			headerRate: 7,
			expected:   7,
		},
		{
			name:     "robots crawl delay when no explicit or header",
			cfg:      LimiterConfig{DefaultRate: 3, RobotsCrawlDelay: 500 * time.Millisecond},
			expected: 2.0, // 1/0.5
		},
		{
			name:     "default rate when no explicit, header, or robots",
			cfg:      LimiterConfig{DefaultRate: 8},
			expected: 8,
		},
		{
			name:     "global default when nothing configured",
			cfg:      LimiterConfig{},
			expected: 5.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := resolveRate(tc.cfg, tc.headerRate)
			assert.InDelta(t, tc.expected, result, 0.001)
		})
	}
}

func TestNewAdaptiveLimiter(t *testing.T) {
	t.Parallel()

	t.Run("uses explicit rate", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 10})
		assert.InDelta(t, 10.0, lim.Rate(), 0.001)
	})

	t.Run("uses default rate", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{DefaultRate: 3})
		assert.InDelta(t, 3.0, lim.Rate(), 0.001)
	})

	t.Run("uses global default", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{})
		assert.InDelta(t, 5.0, lim.Rate(), 0.001)
	})

	t.Run("uses robots crawl delay", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{RobotsCrawlDelay: 2 * time.Second})
		assert.InDelta(t, 0.5, lim.Rate(), 0.001)
	})
}

func TestAdaptiveLimiter_Wait(t *testing.T) {
	t.Parallel()

	t.Run("allows request within rate", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 100})

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := lim.Wait(ctx)
		require.NoError(t, err)
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 0.01})

		// Consume the initial burst.
		ctx := context.Background()
		_ = lim.Wait(ctx)

		// Now cancel immediately.
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err := lim.Wait(cancelCtx)
		require.Error(t, err)

		var rlErr *RateLimitError
		assert.ErrorAs(t, err, &rlErr)
	})
}

func TestAdaptiveLimiter_UpdateFromHeaders(t *testing.T) {
	t.Parallel()

	t.Run("adjusts rate from headers", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{DefaultRate: 5})

		headers := http.Header{
			"X-Ratelimit-Limit":     {"100"},
			"X-Ratelimit-Remaining": {"80"},
		}

		lim.UpdateFromHeaders(headers)

		// Rate should be updated based on header info.
		rate := lim.Rate()
		assert.Greater(t, rate, 0.0, "rate should be positive after header update")
	})

	t.Run("explicit rate cannot be overridden by headers", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 10})

		headers := http.Header{
			"X-Ratelimit-Limit":     {"1"},
			"X-Ratelimit-Remaining": {"0"},
		}

		lim.UpdateFromHeaders(headers)
		assert.InDelta(t, 10.0, lim.Rate(), 0.001, "explicit rate should not change")
	})

	t.Run("proactive slowdown when remaining is low", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{DefaultRate: 50})

		// First set a "normal" header rate with plenty remaining.
		highHeaders := http.Header{
			"X-Ratelimit-Limit":     {"100"},
			"X-Ratelimit-Remaining": {"80"},
		}
		lim.UpdateFromHeaders(highHeaders)
		normalRate := lim.Rate()

		// Now simulate remaining dropping below 10% threshold.
		lowHeaders := http.Header{
			"X-Ratelimit-Limit":     {"100"},
			"X-Ratelimit-Remaining": {"5"}, // 5% remaining = below 10% threshold
		}
		lim.UpdateFromHeaders(lowHeaders)
		slowedRate := lim.Rate()

		assert.Less(t, slowedRate, normalRate, "rate should decrease when remaining is low")
	})

	t.Run("no update for missing headers", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{DefaultRate: 5})

		lim.UpdateFromHeaders(http.Header{})
		assert.InDelta(t, 5.0, lim.Rate(), 0.001, "rate should not change with empty headers")
	})
}

func TestAdaptiveLimiter_HandleRetryAfter(t *testing.T) {
	t.Parallel()

	t.Run("temporarily pauses and resumes", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 10})

		lim.HandleRetryAfter(200 * time.Millisecond)

		// Rate should be 0 during the retry-after period.
		assert.InDelta(t, 0.0, lim.Rate(), 0.001, "rate should be 0 during retry-after")

		// Wait for the retry-after to expire.
		time.Sleep(400 * time.Millisecond)

		assert.InDelta(t, 10.0, lim.Rate(), 0.001, "rate should be restored after retry-after")
	})

	t.Run("ignores zero duration", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 10})

		lim.HandleRetryAfter(0)
		assert.InDelta(t, 10.0, lim.Rate(), 0.001, "rate should not change for zero duration")
	})

	t.Run("ignores negative duration", func(t *testing.T) {
		t.Parallel()
		lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 10})

		lim.HandleRetryAfter(-5 * time.Second)
		assert.InDelta(t, 10.0, lim.Rate(), 0.001, "rate should not change for negative duration")
	})
}

func TestAdaptiveLimiter_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	lim := NewAdaptiveLimiter(LimiterConfig{ExplicitRate: 1000})

	const goroutines = 20
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			for j := range opsPerGoroutine {
				switch j % 3 {
				case 0:
					_ = lim.Wait(ctx)
				case 1:
					headers := http.Header{
						"X-Ratelimit-Limit":     {"1000"},
						"X-Ratelimit-Remaining": {"500"},
					}
					lim.UpdateFromHeaders(headers)
				case 2:
					_ = lim.Rate()
				}
			}
		}(i)
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes.
}

func TestRateLimitError(t *testing.T) {
	t.Parallel()

	cause := context.Canceled
	err := &RateLimitError{Cause: cause}

	assert.Contains(t, err.Error(), "rate limit wait failed")
	assert.ErrorIs(t, err, context.Canceled)
}
