package ratelimit

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitInfo holds parsed rate limit information from HTTP response headers.
type RateLimitInfo struct {
	Limit      float64
	Remaining  int
	ResetAt    time.Time
	RetryAfter time.Duration
	Found      bool
}

// ParseRateLimitHeaders extracts rate limit information from HTTP response headers.
// It supports X-RateLimit-* headers, IETF draft RateLimit-* headers, and the
// Retry-After header (both delta-seconds and HTTP-date formats).
func ParseRateLimitHeaders(headers http.Header) RateLimitInfo {
	if headers == nil {
		return RateLimitInfo{}
	}

	info := parseStandardHeaders(headers)
	if !info.Found {
		info = parseIETFHeaders(headers)
	}

	retryAfter := parseRetryAfter(headers)
	if retryAfter > 0 {
		info.RetryAfter = retryAfter
		info.Found = true
	}

	return info
}

// parseStandardHeaders parses X-RateLimit-* style headers.
func parseStandardHeaders(headers http.Header) RateLimitInfo {
	limitStr := headers.Get("X-RateLimit-Limit")
	remainingStr := headers.Get("X-RateLimit-Remaining")
	resetStr := headers.Get("X-RateLimit-Reset")

	if limitStr == "" && remainingStr == "" && resetStr == "" {
		return RateLimitInfo{}
	}

	return buildRateLimitInfo(limitStr, remainingStr, resetStr)
}

// parseIETFHeaders parses IETF draft standard RateLimit-* headers.
func parseIETFHeaders(headers http.Header) RateLimitInfo {
	limitStr := headers.Get("RateLimit-Limit")
	remainingStr := headers.Get("RateLimit-Remaining")
	resetStr := headers.Get("RateLimit-Reset")

	if limitStr == "" && remainingStr == "" && resetStr == "" {
		return RateLimitInfo{}
	}

	return buildRateLimitInfo(limitStr, remainingStr, resetStr)
}

// buildRateLimitInfo constructs a RateLimitInfo from string values.
func buildRateLimitInfo(limitStr, remainingStr, resetStr string) RateLimitInfo {
	info := RateLimitInfo{Found: true}

	if limitStr != "" {
		if v, err := strconv.ParseFloat(limitStr, 64); err == nil {
			info.Limit = v
		}
	}

	if remainingStr != "" {
		if v, err := strconv.Atoi(remainingStr); err == nil {
			info.Remaining = v
		}
	}

	if resetStr != "" {
		info.ResetAt = parseResetValue(resetStr)
	}

	return info
}

// parseResetValue interprets a reset header value as either a unix timestamp
// or seconds-from-now. Values above a threshold are treated as unix timestamps;
// smaller values are treated as relative seconds.
func parseResetValue(s string) time.Time {
	s = strings.TrimSpace(s)

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}

	// Heuristic: unix timestamps are large numbers (after year 2000).
	// Seconds-from-now are typically small.
	const unixThreshold = 946_684_800 // 2000-01-01 00:00:00 UTC
	if v > unixThreshold {
		return time.Unix(v, 0)
	}

	return time.Now().Add(time.Duration(v) * time.Second)
}

// parseRetryAfter parses the Retry-After header.
// It supports both delta-seconds (integer) and HTTP-date formats.
func parseRetryAfter(headers http.Header) time.Duration {
	val := headers.Get("Retry-After")
	if val == "" {
		return 0
	}

	val = strings.TrimSpace(val)

	// Try delta-seconds first.
	if seconds, err := strconv.Atoi(val); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try HTTP-date format (RFC 1123).
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}

	return 0
}
