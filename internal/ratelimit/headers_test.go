package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRateLimitHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		headers        http.Header
		expectFound    bool
		expectLimit    float64
		expectRemain   int
		expectRetry    time.Duration
		checkResetAt   func(t *testing.T, resetAt time.Time)
	}{
		{
			name:        "nil headers",
			headers:     nil,
			expectFound: false,
		},
		{
			name:        "empty headers",
			headers:     http.Header{},
			expectFound: false,
		},
		{
			name: "X-RateLimit standard headers",
			headers: http.Header{
				"X-Ratelimit-Limit":     {"100"},
				"X-Ratelimit-Remaining": {"42"},
				"X-Ratelimit-Reset":     {"30"},
			},
			expectFound:  true,
			expectLimit:  100,
			expectRemain: 42,
			checkResetAt: func(t *testing.T, resetAt time.Time) {
				t.Helper()
				diff := time.Until(resetAt)
				assert.InDelta(t, 30, diff.Seconds(), 2, "reset should be ~30 seconds from now")
			},
		},
		{
			name: "IETF draft RateLimit headers",
			headers: http.Header{
				"Ratelimit-Limit":     {"200"},
				"Ratelimit-Remaining": {"150"},
				"Ratelimit-Reset":     {"60"},
			},
			expectFound:  true,
			expectLimit:  200,
			expectRemain: 150,
			checkResetAt: func(t *testing.T, resetAt time.Time) {
				t.Helper()
				diff := time.Until(resetAt)
				assert.InDelta(t, 60, diff.Seconds(), 2, "reset should be ~60 seconds from now")
			},
		},
		{
			name: "X-RateLimit takes precedence over IETF",
			headers: http.Header{
				"X-Ratelimit-Limit":   {"100"},
				"Ratelimit-Limit":     {"200"},
				"Ratelimit-Remaining": {"150"},
			},
			expectFound: true,
			expectLimit: 100,
			// IETF remaining is not used when X-RateLimit headers are found
			expectRemain: 0,
		},
		{
			name: "reset as unix timestamp",
			headers: http.Header{
				"X-Ratelimit-Limit":     {"100"},
				"X-Ratelimit-Remaining": {"50"},
				"X-Ratelimit-Reset":     {"1893456000"}, // 2030-01-01
			},
			expectFound:  true,
			expectLimit:  100,
			expectRemain: 50,
			checkResetAt: func(t *testing.T, resetAt time.Time) {
				t.Helper()
				expected := time.Unix(1893456000, 0)
				assert.Equal(t, expected, resetAt, "should parse as unix timestamp")
			},
		},
		{
			name: "Retry-After in seconds",
			headers: http.Header{
				"Retry-After": {"120"},
			},
			expectFound: true,
			expectRetry: 120 * time.Second,
		},
		{
			name: "Retry-After as HTTP-date",
			headers: http.Header{
				"Retry-After": {time.Now().Add(60 * time.Second).UTC().Format(http.TimeFormat)},
			},
			expectFound: true,
			expectRetry: 60 * time.Second,
		},
		{
			name: "combined rate limit and Retry-After",
			headers: http.Header{
				"X-Ratelimit-Limit":     {"100"},
				"X-Ratelimit-Remaining": {"0"},
				"Retry-After":           {"30"},
			},
			expectFound:  true,
			expectLimit:  100,
			expectRemain: 0,
			expectRetry:  30 * time.Second,
		},
		{
			name: "only Retry-After present",
			headers: http.Header{
				"Retry-After": {"45"},
			},
			expectFound: true,
			expectRetry: 45 * time.Second,
		},
		{
			name: "partial headers - only limit",
			headers: http.Header{
				"X-Ratelimit-Limit": {"50"},
			},
			expectFound: true,
			expectLimit: 50,
		},
		{
			name: "invalid header values",
			headers: http.Header{
				"X-Ratelimit-Limit":     {"not-a-number"},
				"X-Ratelimit-Remaining": {"also-bad"},
				"X-Ratelimit-Reset":     {"nope"},
			},
			expectFound:  true,
			expectLimit:  0,
			expectRemain: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			info := ParseRateLimitHeaders(tc.headers)

			assert.Equal(t, tc.expectFound, info.Found, "Found mismatch")
			assert.InDelta(t, tc.expectLimit, info.Limit, 0.001, "Limit mismatch")
			assert.Equal(t, tc.expectRemain, info.Remaining, "Remaining mismatch")

			if tc.expectRetry > 0 {
				require.NotZero(t, info.RetryAfter, "RetryAfter should be non-zero")
				assert.InDelta(t, tc.expectRetry.Seconds(), info.RetryAfter.Seconds(), 2, "RetryAfter mismatch")
			}

			if tc.checkResetAt != nil {
				tc.checkResetAt(t, info.ResetAt)
			}
		})
	}
}

func TestParseResetValue_Threshold(t *testing.T) {
	t.Parallel()

	// A value below the unix threshold should be treated as relative seconds.
	result := parseResetValue("300")
	diff := time.Until(result)
	assert.InDelta(t, 300, diff.Seconds(), 2, "should be ~300 seconds from now")

	// A value above the unix threshold should be treated as a unix timestamp.
	ts := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC).Unix()
	result = parseResetValue(time.Now().Format("") + string(rune(0)) + "invalid")
	assert.True(t, result.IsZero(), "invalid input should return zero time")

	result = parseResetValue(time.Unix(ts, 0).Format("not-a-number"))
	assert.True(t, result.IsZero(), "non-numeric input should return zero time")

	result = parseResetValue("1893456000")
	assert.Equal(t, time.Unix(1893456000, 0), result, "large value should be unix timestamp")
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	t.Parallel()

	// A past HTTP-date should return 0 duration.
	pastDate := time.Now().Add(-1 * time.Hour).UTC().Format(http.TimeFormat)
	headers := http.Header{"Retry-After": {pastDate}}
	d := parseRetryAfter(headers)
	assert.Zero(t, d, "past date should return zero duration")
}
