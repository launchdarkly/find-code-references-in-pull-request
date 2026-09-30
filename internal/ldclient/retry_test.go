package ldapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryWaitUsesLatestServiceHintAndJitter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	hints := parseRateLimitHeaders(http.Header{
		"X-Ratelimit-Reset":            {strconv.FormatInt(now.Add(3*time.Second).UnixMilli(), 10)},
		"X-Ratelimit-Auth-Token-Reset": {strconv.FormatInt(now.Add(5*time.Second).UnixMilli(), 10)},
		"Retry-After":                  {"2"},
	})

	decision := retryWait(now, &deadline, 0, hints, func(_, maximum time.Duration) time.Duration {
		return maximum
	})

	assert.False(t, decision.unfulfillable)
	assert.Equal(t, "rate_limit_reset", decision.reason)
	assert.Equal(t, 6*time.Second, decision.delay)
}

func TestRetryWaitUsesApplicableQuotaResets(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	maximumJitter := func(_, maximum time.Duration) time.Duration { return maximum }
	tests := []struct {
		name           string
		global         string
		route          string
		token          string
		sharedReset    string
		tokenReset     string
		retryAfter     string
		deadlineOffset time.Duration
		wantDelay      time.Duration
		wantReason     string
		wantUnfulfill  bool
	}{
		{
			name: "route exhausted, token available", global: "1", route: "0", token: "5",
			sharedReset: "2", tokenReset: "8", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "global exhausted, token available", global: "0", route: "1", token: "5",
			sharedReset: "2", tokenReset: "8", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "token exhausted, shared quotas available", global: "1", route: "1", token: "0",
			sharedReset: "8", tokenReset: "2", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "only route remaining supplied", global: "?", route: "1", token: "0",
			sharedReset: "8", tokenReset: "2", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "only global remaining supplied", global: "1", route: "?", token: "0",
			sharedReset: "8", tokenReset: "2", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "both reset families exhausted", global: "1", route: "0", token: "0",
			sharedReset: "2", tokenReset: "4", deadlineOffset: 10 * time.Second,
			wantDelay: 5 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "unknown token count", global: "1", route: "0", token: "?",
			sharedReset: "2", tokenReset: "8", deadlineOffset: 10 * time.Second,
			wantDelay: 9 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "only reset headers supplied", global: "?", route: "?", token: "?",
			sharedReset: "2", tokenReset: "4", deadlineOffset: 10 * time.Second,
			wantDelay: 5 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "all known quotas have capacity", global: "1", route: "1", token: "5",
			sharedReset: "8", tokenReset: "8", deadlineOffset: 5 * time.Second,
			wantDelay: 2 * time.Second, wantReason: "rate_limit_backoff",
		},
		{
			name: "retry-after exceeds relevant reset", global: "1", route: "0", token: "5",
			sharedReset: "2", tokenReset: "8", retryAfter: "3", deadlineOffset: 5 * time.Second,
			wantDelay: 4 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "jitter reaches deadline", global: "1", route: "0", token: "5",
			sharedReset: "2", tokenReset: "8", retryAfter: "4", deadlineOffset: 5 * time.Second,
			wantReason: "rate_limit_reset", wantUnfulfill: true,
		},
		{
			name: "retry-after exceeds deadline", global: "1", route: "0", token: "5",
			sharedReset: "2", tokenReset: "8", retryAfter: "6", deadlineOffset: 5 * time.Second,
			wantReason: "rate_limit_reset", wantUnfulfill: true,
		},
		{
			name: "ignored token reset overflows", global: "1", route: "0", token: "5",
			sharedReset: "2", tokenReset: "9223372036854775808", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "ignored shared reset overflows", global: "1", route: "1", token: "0",
			sharedReset: "9223372036854775808", tokenReset: "2", deadlineOffset: 5 * time.Second,
			wantDelay: 3 * time.Second, wantReason: "rate_limit_reset",
		},
		{
			name: "exhausted token reset overflows", global: "1", route: "0", token: "0",
			sharedReset: "2", tokenReset: "9223372036854775808", deadlineOffset: 5 * time.Second,
			wantReason: "rate_limit_reset", wantUnfulfill: true,
		},
		{
			name: "unknown token reset overflows", global: "1", route: "0", token: "?",
			sharedReset: "2", tokenReset: "9223372036854775808", deadlineOffset: 5 * time.Second,
			wantReason: "rate_limit_reset", wantUnfulfill: true,
		},
		{
			name: "exhausted route reset overflows", global: "1", route: "0", token: "5",
			sharedReset: "9223372036854775808", tokenReset: "8", deadlineOffset: 5 * time.Second,
			wantReason: "rate_limit_reset", wantUnfulfill: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.global != "?" {
				headers.Set("X-Ratelimit-Global-Remaining", tt.global)
			}
			if tt.route != "?" {
				headers.Set("X-Ratelimit-Route-Remaining", tt.route)
			}
			if tt.token != "?" {
				headers.Set("X-Ratelimit-Auth-Token-Remaining", tt.token)
			}
			if tt.sharedReset != "" {
				if strings.HasPrefix(tt.sharedReset, "9223372036854775808") {
					headers.Set("X-Ratelimit-Reset", tt.sharedReset)
				} else {
					offset, err := time.ParseDuration(tt.sharedReset + "s")
					require.NoError(t, err)
					headers.Set("X-Ratelimit-Reset", strconv.FormatInt(now.Add(offset).UnixMilli(), 10))
				}
			}
			if tt.tokenReset != "" {
				if strings.HasPrefix(tt.tokenReset, "9223372036854775808") {
					headers.Set("X-Ratelimit-Auth-Token-Reset", tt.tokenReset)
				} else {
					offset, err := time.ParseDuration(tt.tokenReset + "s")
					require.NoError(t, err)
					headers.Set("X-Ratelimit-Auth-Token-Reset", strconv.FormatInt(now.Add(offset).UnixMilli(), 10))
				}
			}
			if tt.retryAfter != "" {
				headers.Set("Retry-After", tt.retryAfter)
			}

			decision := retryWait(
				now,
				timePtr(now.Add(tt.deadlineOffset)),
				0,
				parseRateLimitHeaders(headers),
				maximumJitter,
			)
			assert.Equal(t, tt.wantReason, decision.reason)
			assert.Equal(t, tt.wantUnfulfill, decision.unfulfillable)
			if tt.wantUnfulfill {
				assert.Zero(t, decision.delay)
			} else {
				assert.Equal(t, tt.wantDelay, decision.delay)
			}
		})
	}

	for _, remaining := range []string{"?", "malformed"} {
		t.Run("unknown token "+remaining, func(t *testing.T) {
			headers := http.Header{
				"X-Ratelimit-Global-Remaining": {"1"},
				"X-Ratelimit-Route-Remaining":  {"0"},
				"X-Ratelimit-Auth-Token-Reset": {
					strconv.FormatInt(now.Add(8*time.Second).UnixMilli(), 10),
				},
				"X-Ratelimit-Reset": {
					strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10),
				},
			}
			if remaining == "malformed" {
				headers.Set("X-Ratelimit-Auth-Token-Remaining", remaining)
			}
			decision := retryWait(
				now,
				timePtr(now.Add(10*time.Second)),
				0,
				parseRateLimitHeaders(headers),
				maximumJitter,
			)
			assert.Equal(t, 9*time.Second, decision.delay)
			assert.Equal(t, "rate_limit_reset", decision.reason)
			assert.False(t, decision.unfulfillable)
		})
	}

	for _, remaining := range []string{"?", "malformed"} {
		t.Run("route-only shared "+remaining, func(t *testing.T) {
			headers := http.Header{
				"X-Ratelimit-Route-Remaining":      {"1"},
				"X-Ratelimit-Auth-Token-Remaining": {"0"},
				"X-Ratelimit-Reset": {
					strconv.FormatInt(now.Add(8*time.Second).UnixMilli(), 10),
				},
				"X-Ratelimit-Auth-Token-Reset": {
					strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10),
				},
			}
			if remaining == "malformed" {
				headers.Set("X-Ratelimit-Global-Remaining", remaining)
			}
			decision := retryWait(
				now,
				timePtr(now.Add(5*time.Second)),
				0,
				parseRateLimitHeaders(headers),
				maximumJitter,
			)
			assert.Equal(t, 3*time.Second, decision.delay)
			assert.Equal(t, "rate_limit_reset", decision.reason)
			assert.False(t, decision.unfulfillable)
		})
	}
}

func TestRetryWaitUsesHTTPDateRetryAfter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(5 * time.Second)
	hints := parseRateLimitHeaders(http.Header{
		"X-Ratelimit-Global-Remaining": {"1"},
		"X-Ratelimit-Route-Remaining":  {"0"},
		"X-Ratelimit-Reset": {
			strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10),
		},
		"Retry-After": {now.Add(3 * time.Second).UTC().Format(http.TimeFormat)},
	})
	decision := retryWait(
		now,
		&deadline,
		0,
		hints,
		func(_, maximum time.Duration) time.Duration { return maximum },
	)
	assert.Equal(t, "rate_limit_reset", decision.reason)
	assert.False(t, decision.unfulfillable)
	assert.Equal(t, 4*time.Second, decision.delay)
}

func TestRetryWaitFallbackWindows(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(30 * time.Second)

	for retryNumber, want := range map[int]time.Duration{
		0: 2 * time.Second,
		1: 4 * time.Second,
		2: 8 * time.Second,
		3: 16 * time.Second,
	} {
		decision := retryWait(
			now,
			&deadline,
			retryNumber,
			parsedRateLimitHeaders{},
			func(_, maximum time.Duration) time.Duration { return maximum },
		)
		assert.False(t, decision.unfulfillable)
		assert.Equal(t, "rate_limit_backoff", decision.reason)
		assert.Equal(t, want, decision.delay)
	}
}

func TestRetryWaitRejectsUnrepresentableReset(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	for _, header := range []string{
		"X-Ratelimit-Reset",
		"X-Ratelimit-Auth-Token-Reset",
	} {
		t.Run(header, func(t *testing.T) {
			hints := parseRateLimitHeaders(http.Header{
				header: {"9223372036854775808"},
			})

			decision := retryWait(now, &deadline, 0, hints, uniformJitter)

			assert.True(t, decision.unfulfillable)
		})
	}
}

func TestRetryWaitParsesEachTimingHeader(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	date := now.Add(2 * time.Second).UTC().Format(http.TimeFormat)
	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "global reset", header: "X-Ratelimit-Reset", value: strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
		{name: "auth reset", header: "X-Ratelimit-Auth-Token-Reset", value: strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
		{name: "retry after seconds", header: "Retry-After", value: "2"},
		{name: "retry after date", header: "Retry-After", value: date},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := parseRateLimitHeaders(http.Header{tt.header: {tt.value}})
			decision := retryWait(
				now,
				&deadline,
				0,
				hints,
				func(_, maximum time.Duration) time.Duration { return maximum },
			)
			assert.False(t, decision.unfulfillable)
			assert.Equal(t, 3*time.Second, decision.delay)
		})
	}
}

func TestRetryWaitHandlesWhitespacePastZeroNegativeAndMalformedHints(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	for _, value := range []string{" 0 ", " ", "-1", "not-a-number", now.Add(-time.Second).Format(http.TimeFormat)} {
		hints := parseRateLimitHeaders(http.Header{"Retry-After": {value}})
		decision := retryWait(
			now,
			&deadline,
			0,
			hints,
			func(minimum, _ time.Duration) time.Duration { return minimum },
		)
		assert.False(t, decision.unfulfillable, value)
		assert.Equal(t, "rate_limit_backoff", decision.reason, value)
		assert.Equal(t, time.Second, decision.delay, value)
	}
}

func TestRetryBackoffReturnsRetainedWaitWithoutResampling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	jitterCalls := 0
	fetcher := newFlagFetcher(ctx, fetcherOptions{
		jitter: func(_, maximum time.Duration) time.Duration {
			jitterCalls++
			return maximum
		},
	})
	fetcher.requestLogHook(nil, nil, 0)

	shouldRetry, err := fetcher.checkRetry(
		ctx,
		&http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}},
		nil,
	)

	require.NoError(t, err)
	assert.True(t, shouldRetry)
	assert.Equal(t, 1, jitterCalls)
	assert.Equal(t, 2*time.Second, fetcher.retryBackoff(0, 0, 0, nil))
	assert.Equal(t, time.Duration(0), fetcher.pendingRetryWait)
	assert.Equal(t, time.Duration(0), fetcher.retryBackoff(0, 0, 0, nil))
	assert.Equal(t, 1, jitterCalls)
}

func TestRetryWaitHandlesMultipleHintsAndOverflow(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	header := http.Header{}
	header.Add("X-Ratelimit-Reset", strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10))
	header.Add("X-Ratelimit-Reset", strconv.FormatInt(now.Add(4*time.Second).UnixMilli(), 10))
	header.Add("Retry-After", " 1 ")
	header.Add("Retry-After", "3")
	hints := parseRateLimitHeaders(header)
	decision := retryWait(
		now,
		&deadline,
		0,
		hints,
		func(_, maximum time.Duration) time.Duration { return maximum },
	)
	assert.False(t, decision.unfulfillable)
	assert.Equal(t, 5*time.Second, decision.delay)

	for _, value := range []string{"9223372037", "18446744073709551616"} {
		overflowHints := parseRateLimitHeaders(http.Header{"Retry-After": {value}})
		decision = retryWait(now, &deadline, 0, overflowHints, uniformJitter)
		assert.True(t, decision.unfulfillable, value)
	}
}

func TestRetryWaitJitterBounds(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(30 * time.Second)
	for name, jitter := range map[string]func(time.Duration, time.Duration) time.Duration{
		"minimum": func(minimum, _ time.Duration) time.Duration { return minimum },
		"maximum": func(_, maximum time.Duration) time.Duration { return maximum },
	} {
		t.Run(name, func(t *testing.T) {
			decision := retryWait(now, &deadline, 0, parsedRateLimitHeaders{}, jitter)
			assert.False(t, decision.unfulfillable)
			assert.GreaterOrEqual(t, decision.delay, time.Second)
			assert.LessOrEqual(t, decision.delay, 2*time.Second)
		})
	}
}

func TestRetryWaitRejectsDelayAtDeadline(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(2 * time.Second)
	hints := parseRateLimitHeaders(http.Header{
		"Retry-After": {"2"},
	})

	decision := retryWait(
		now,
		&deadline,
		0,
		hints,
		func(minimum, _ time.Duration) time.Duration { return minimum },
	)

	assert.True(t, decision.unfulfillable)
}

func TestProactiveWaitPairsRemainingAndResetHeaders(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	hints := parseRateLimitHeaders(http.Header{
		"X-Ratelimit-Global-Remaining":     {"1"},
		"X-Ratelimit-Route-Remaining":      {"0"},
		"X-Ratelimit-Auth-Token-Remaining": {"5"},
		"X-Ratelimit-Reset":                {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
		"X-Ratelimit-Auth-Token-Reset":     {strconv.FormatInt(now.Add(8*time.Second).UnixMilli(), 10)},
	})

	decision := proactiveWait(
		now,
		&deadline,
		hints,
		func(_, maximum time.Duration) time.Duration { return maximum },
	)

	assert.False(t, decision.unfulfillable)
	assert.Equal(t, 3*time.Second, decision.delay)
}

func TestProactiveWaitRequiresAnExhaustedQuota(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	reset := strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)
	tests := []struct {
		name        string
		header      string
		resetHeader string
	}{
		{
			name:        "global",
			header:      "X-Ratelimit-Global-Remaining",
			resetHeader: "X-Ratelimit-Reset",
		},
		{
			name:        "route",
			header:      "X-Ratelimit-Route-Remaining",
			resetHeader: "X-Ratelimit-Reset",
		},
		{
			name:        "auth token",
			header:      "X-Ratelimit-Auth-Token-Remaining",
			resetHeader: "X-Ratelimit-Auth-Token-Reset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := parseRateLimitHeaders(http.Header{
				tt.header:      {"0"},
				tt.resetHeader: {reset},
			})
			decision := proactiveWait(
				now,
				&deadline,
				hints,
				func(_, maximum time.Duration) time.Duration { return maximum },
			)
			assert.False(t, decision.unfulfillable)
			assert.Equal(t, 3*time.Second, decision.delay)
		})
	}
}

func TestProactiveWaitIgnoresPositiveAndUnknownRemaining(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	tests := []struct {
		name            string
		remaining       string
		remainingHeader string
		resetHeader     string
	}{
		{
			name:            "positive global",
			remaining:       "1",
			remainingHeader: "X-Ratelimit-Global-Remaining",
			resetHeader:     "X-Ratelimit-Reset",
		},
		{
			name:            "unknown route",
			remaining:       "unknown",
			remainingHeader: "X-Ratelimit-Route-Remaining",
			resetHeader:     "X-Ratelimit-Reset",
		},
		{
			name:            "positive token",
			remaining:       "1",
			remainingHeader: "X-Ratelimit-Auth-Token-Remaining",
			resetHeader:     "X-Ratelimit-Auth-Token-Reset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, remaining := range []string{tt.remaining, "unknown"} {
				reset := strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)
				hints := parseRateLimitHeaders(http.Header{
					tt.remainingHeader: {remaining},
					tt.resetHeader:     {reset},
				})
				decision := proactiveWait(
					now,
					&deadline,
					hints,
					uniformJitter,
				)
				assert.False(t, decision.unfulfillable, remaining)
				assert.Zero(t, decision.delay, remaining)
			}
		})
	}
}

func TestProactiveWaitUsesFallbackForMissingOrMalformedReset(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	tests := []struct {
		name            string
		remaining       string
		remainingHeader string
		resetHeader     string
	}{
		{
			name:            "global missing",
			remaining:       "0",
			remainingHeader: "X-Ratelimit-Global-Remaining",
			resetHeader:     "X-Ratelimit-Reset",
		},
		{
			name:            "route malformed",
			remaining:       "0",
			remainingHeader: "X-Ratelimit-Route-Remaining",
			resetHeader:     "X-Ratelimit-Reset",
		},
		{
			name:            "token missing",
			remaining:       "0",
			remainingHeader: "X-Ratelimit-Auth-Token-Remaining",
			resetHeader:     "X-Ratelimit-Auth-Token-Reset",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{tt.remainingHeader: {tt.remaining}}
			if tt.name == "route malformed" {
				headers.Set(tt.resetHeader, "not-a-reset")
			}
			hints := parseRateLimitHeaders(headers)
			decision := proactiveWait(
				now,
				&deadline,
				hints,
				func(_, maximum time.Duration) time.Duration { return maximum },
			)
			assert.False(t, decision.unfulfillable)
			assert.Equal(t, 2*time.Second, decision.delay)
		})
	}
}

func TestProactiveWaitHonorsRetryAfterMinimum(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	tests := []struct {
		name          string
		retryAfter    []string
		wantDelay     time.Duration
		unfulfillable bool
	}{
		{name: "seconds", retryAfter: []string{"5"}, wantDelay: 6 * time.Second},
		{name: "multiple values", retryAfter: []string{"3", "5"}, wantDelay: 6 * time.Second},
		{
			name:       "HTTP date",
			retryAfter: []string{now.Add(5 * time.Second).UTC().Format(http.TimeFormat)},
			wantDelay:  6 * time.Second,
		},
		{name: "duration overflow", retryAfter: []string{"9223372037"}, unfulfillable: true},
		{
			name:          "integer overflow",
			retryAfter:    []string{"18446744073709551616"},
			unfulfillable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := parseRateLimitHeaders(http.Header{
				"X-Ratelimit-Route-Remaining": {"0"},
				"X-Ratelimit-Reset": {
					strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10),
				},
				"Retry-After": tt.retryAfter,
			})
			decision := proactiveWait(
				now,
				&deadline,
				hints,
				func(_, maximum time.Duration) time.Duration { return maximum },
			)
			assert.Equal(t, tt.unfulfillable, decision.unfulfillable)
			if tt.unfulfillable {
				assert.Zero(t, decision.delay)
			} else {
				assert.Equal(t, tt.wantDelay, decision.delay)
			}
		})
	}
}

func TestProactiveWaitDoesNotPairTokenResetWithAnotherQuota(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	deadline := now.Add(10 * time.Second)
	hints := parseRateLimitHeaders(http.Header{
		"X-Ratelimit-Route-Remaining":      {"0"},
		"X-Ratelimit-Global-Remaining":     {"1"},
		"X-Ratelimit-Auth-Token-Remaining": {"1"},
		"X-Ratelimit-Reset":                {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
		"X-Ratelimit-Auth-Token-Reset":     {strconv.FormatInt(now.Add(8*time.Second).UnixMilli(), 10)},
	})

	decision := proactiveWait(
		now,
		&deadline,
		hints,
		func(_, maximum time.Duration) time.Duration { return maximum },
	)

	assert.False(t, decision.unfulfillable)
	assert.Equal(t, 3*time.Second, decision.delay)
}

func TestFetcherRetriesOnlyRateLimitedPages(t *testing.T) {
	config := serveFlagPages(t, false, []flagPageResponse{
		{body: `{"items":[{"key":"first"}]}`},
		{offset: 100, status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
		{offset: 100, status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
		{offset: 100, body: `{"items":[{"key":"second"}]}`},
		{offset: 200, body: `{"items":[]}`},
	})
	fetcher, _ := newTestFetcher(t)
	waits := []time.Duration{}
	setTestBackoff(fetcher, func(wait time.Duration) time.Duration {
		waits = append(waits, wait)
		return 0
	})

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, keys(flags))
	assert.Equal(t, 5, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, 2, fetcher.diagnostics.retries)
	assert.Equal(t, 2, fetcher.diagnostics.responses429)
	assert.Equal(t, []time.Duration{2 * time.Second, 4 * time.Second}, waits)
}

func TestFetcherRetriesAfterExhaustedQuotaReset(t *testing.T) {
	testDeadline := time.Now().Add(30 * time.Second).Truncate(time.Millisecond)
	parentCtx, cancel := context.WithDeadline(context.Background(), testDeadline)
	t.Cleanup(cancel)
	fetcher, logs := newTestFetcherWithContext(t, parentCtx)
	deadline, ok := fetcher.ctx.Deadline()
	require.True(t, ok)
	now := deadline.Add(-5 * time.Second)
	fetcher.clock = func() time.Time { return now }

	config := serveFlagPages(t, false, []flagPageResponse{
		{
			status: http.StatusTooManyRequests,
			headers: http.Header{
				"X-Ratelimit-Global-Remaining":     {"1"},
				"X-Ratelimit-Route-Remaining":      {"0"},
				"X-Ratelimit-Auth-Token-Remaining": {"5"},
				"X-Ratelimit-Reset": {
					strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10),
				},
				"X-Ratelimit-Auth-Token-Reset": {
					strconv.FormatInt(now.Add(8*time.Second).UnixMilli(), 10),
				},
			},
			body: "{\"message\":\"rate limited\"}",
		},
		{body: "{\"items\":[]}"},
	})

	var waits []time.Duration
	setTestBackoff(fetcher, func(wait time.Duration) time.Duration {
		waits = append(waits, wait)
		return 0
	})
	fetcher.diagnostics.emitStart()

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)
	fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)

	require.NoError(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 2, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, 1, fetcher.diagnostics.retries)
	assert.Equal(t, 1, fetcher.diagnostics.responses429)
	assert.Equal(t, 1, fetcher.diagnostics.retryWaits)
	assert.Equal(t, []time.Duration{3 * time.Second}, waits)

	records := diagnosticRecords(t, *logs)
	var summary map[string]any
	var wait map[string]any
	for _, record := range records {
		switch record["event"] {
		case "summary":
			summary = record
		case "wait":
			wait = record
		}
	}
	require.NotNil(t, wait)
	assert.Equal(t, "rate_limit_reset", wait["reason"])
	assert.Equal(t, float64(3000), wait["intended_wait_ms"])
	require.NotNil(t, summary)
	assert.Equal(t, "success", summary["outcome"])
	assert.Equal(t, true, summary["inventory_complete"])
}

func TestFetcherRetriesAtEveryPaginationPosition(t *testing.T) {
	tests := []struct {
		name            string
		includeArchived bool
		pages           []flagPageResponse
		wantKeys        []string
	}{
		{
			name: "first active",
			pages: []flagPageResponse{
				{status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{body: `{"items":[{"key":"active-first"}]}`},
				{offset: 100, body: `{"items":[]}`},
			},
			wantKeys: []string{"active-first"},
		},
		{
			name: "middle active",
			pages: []flagPageResponse{
				{body: `{"items":[{"key":"active-first"}]}`},
				{offset: 100, status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{offset: 100, body: `{"items":[{"key":"active-middle"}]}`},
				{offset: 200, body: `{"items":[]}`},
			},
			wantKeys: []string{"active-first", "active-middle"},
		},
		{
			name: "terminal empty page",
			pages: []flagPageResponse{
				{body: `{"items":[{"key":"active-first"}]}`},
				{offset: 100, status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{offset: 100, body: `{"items":[]}`},
			},
			wantKeys: []string{"active-first"},
		},
		{
			name:            "first archived",
			includeArchived: true,
			pages: []flagPageResponse{
				{body: `{"items":[]}`},
				{filter: "state:archived", status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{filter: "state:archived", body: `{"items":[{"key":"archived-first"}]}`},
				{filter: "state:archived", offset: 100, body: `{"items":[]}`},
			},
			wantKeys: []string{"archived-first"},
		},
		{
			name:            "middle archived",
			includeArchived: true,
			pages: []flagPageResponse{
				{body: `{"items":[]}`},
				{filter: "state:archived", body: `{"items":[{"key":"archived-first"}]}`},
				{filter: "state:archived", offset: 100, status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{filter: "state:archived", offset: 100, body: `{"items":[{"key":"archived-middle"}]}`},
				{filter: "state:archived", offset: 200, body: `{"items":[]}`},
			},
			wantKeys: []string{"archived-first", "archived-middle"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := serveFlagPages(t, tt.includeArchived, tt.pages)
			fetcher, _ := newTestFetcher(t)
			params := urlValuesForFlags()
			flags, err := fetcher.getFlags(config, params, flagCollectionActive, tt.includeArchived)
			require.NoError(t, err)
			if tt.includeArchived {
				params.Set("filter", "state:archived")
				archived, archivedErr := fetcher.getFlags(config, params, flagCollectionArchived, false)
				require.NoError(t, archivedErr)
				flags = append(flags, archived...)
			}
			assert.Equal(t, tt.wantKeys, keys(flags))
		})
	}
}

func TestFetcherDoesNotRetryNon429Statuses(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			config := serveFlagPages(t, false, []flagPageResponse{{status: status, body: `{"message":"sentinel"}`}})
			fetcher, _ := newTestFetcher(t)
			flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)
			require.Error(t, err)
			assert.Empty(t, flags)
			assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
			assert.Contains(t, err.Error(), "status="+strconv.Itoa(status))
		})
	}
}

func TestFetcherDoesNotRetryMalformedSuccessJSON(t *testing.T) {
	config := serveFlagPages(t, false, []flagPageResponse{{status: http.StatusOK, body: "not JSON"}})
	fetcher, _ := newTestFetcher(t)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, "decode_error", fetcher.outcome(err))
	var syntaxErr *json.SyntaxError
	assert.ErrorAs(t, err, &syntaxErr)
}

func TestFetcherClassifiesBodyReadErrors(t *testing.T) {
	readSentinel := errors.New("body-read-sentinel")
	canceledReadErr := fmt.Errorf("body-read-secret: %w", context.Canceled)
	deadlineReadErr := fmt.Errorf("body-read-secret: %w", context.DeadlineExceeded)
	timeoutErr := &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: os.ErrDeadlineExceeded,
	}
	timeoutReadErr := fmt.Errorf("body-read-secret: %w", timeoutErr)
	canceledDuringReadErr := errors.New("body-read-cancel-sentinel")
	tests := []struct {
		name             string
		readErr          error
		wantReason       fetchErrorReason
		wantOutcome      string
		wantContext      error
		sentinel         string
		cancelDuringRead bool
		wantTimeout      bool
	}{
		{
			name:        "wrapped cancellation",
			readErr:     canceledReadErr,
			wantReason:  reasonCanceled,
			wantOutcome: "canceled",
			wantContext: context.Canceled,
			sentinel:    "body-read-secret",
		},
		{
			name:        "wrapped deadline",
			readErr:     deadlineReadErr,
			wantReason:  reasonDeadline,
			wantOutcome: "deadline",
			wantContext: context.DeadlineExceeded,
			sentinel:    "body-read-secret",
		},
		{
			name:        "timeout net error",
			readErr:     timeoutReadErr,
			wantReason:  reasonDeadline,
			wantOutcome: "deadline",
			sentinel:    "body-read-secret",
			wantTimeout: true,
		},
		{
			name:        "non-timeout read error",
			readErr:     readSentinel,
			wantReason:  reasonDecode,
			wantOutcome: "decode_error",
			sentinel:    "body-read-sentinel",
		},
		{
			name:             "context canceled during read",
			readErr:          canceledDuringReadErr,
			wantReason:       reasonCanceled,
			wantOutcome:      "canceled",
			wantContext:      context.Canceled,
			sentinel:         "body-read-cancel-sentinel",
			cancelDuringRead: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			var closed atomic.Int32
			var reader io.Reader = iotest.ErrReader(tt.readErr)
			if tt.cancelDuringRead {
				reader = readFunc(func([]byte) (int, error) {
					cancel()
					return 0, tt.readErr
				})
			}
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{},
					Body: &trackingBody{
						Reader: reader,
						closed: &closed,
					},
					Request: request,
				}, nil
			})
			fetcher, logs := newTestFetcherWithContext(t, parentCtx)
			fetcher.client.HTTPClient.Transport = transport
			fetcher.diagnostics.emitStart()

			flags, err := fetcher.getFlags(
				&lcr.Config{
					LdInstance:    "https://example.test",
					LdProject:     "project",
					LdEnvironment: "production",
					ApiToken:      "token-sentinel",
				},
				urlValuesForFlags(),
				flagCollectionActive,
				false,
			)
			fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)

			require.Error(t, err)
			assert.Empty(t, flags)
			var fetchErr *flagFetchError
			require.ErrorAs(t, err, &fetchErr)
			assert.Equal(t, http.StatusOK, fetchErr.status)
			assert.Equal(t, tt.wantReason, fetchErr.reason)
			assert.Equal(t, tt.wantOutcome, fetcher.outcome(err))
			assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
			assert.Equal(t, 0, fetcher.diagnostics.retries)
			assert.Equal(t, 0, fetcher.diagnostics.successfulPages)
			assert.Equal(t, int32(1), closed.Load())
			assert.ErrorIs(t, err, tt.readErr)
			if tt.wantContext != nil {
				assert.ErrorIs(t, err, tt.wantContext)
			}
			if tt.wantTimeout {
				var timeout net.Error
				require.ErrorAs(t, err, &timeout)
				assert.True(t, timeout.Timeout())
			}
			assert.NotContains(t, err.Error(), tt.sentinel)

			records := diagnosticRecords(t, *logs)
			var summary map[string]any
			for _, record := range records {
				if record["event"] == "summary" {
					summary = record
				}
				assert.NotContains(t, fmt.Sprint(record), tt.sentinel)
			}
			require.NotNil(t, summary)
			assert.Equal(t, false, summary["inventory_complete"])
			assert.Equal(t, float64(http.StatusOK), summary["last_http_status"])
			assert.Equal(t, flagCollectionActive, summary["failed_collection"])
			assert.Equal(t, float64(0), summary["failed_offset"])
		})
	}
}

func TestFetcherReturnsNoPartialInventoryAfterLaterPageReadCancellation(t *testing.T) {
	var closed atomic.Int32
	readErr := fmt.Errorf("later-page-secret: %w", context.Canceled)
	var requestNumber atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		index := requestNumber.Add(1)
		var reader io.Reader = strings.NewReader("{\"items\":[{\"key\":\"partial\"}]}")
		if index == 2 {
			reader = iotest.ErrReader(readErr)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body: &trackingBody{
				Reader: reader,
				closed: &closed,
			},
			Request: request,
		}, nil
	})
	fetcher, logs := newTestFetcher(t)
	fetcher.client.HTTPClient.Transport = transport
	fetcher.diagnostics.emitStart()

	flags, err := fetcher.getFlags(
		&lcr.Config{
			LdInstance:    "https://example.test",
			LdProject:     "project",
			LdEnvironment: "production",
			ApiToken:      "token-sentinel",
		},
		urlValuesForFlags(),
		flagCollectionActive,
		false,
	)
	fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, readErr)
	assert.Equal(t, "canceled", fetcher.outcome(err))
	assert.Equal(t, 2, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, 1, fetcher.diagnostics.successfulPages)
	assert.Equal(t, int32(2), closed.Load())
	var fetchErr *flagFetchError
	require.ErrorAs(t, err, &fetchErr)
	assert.Equal(t, http.StatusOK, fetchErr.status)
	assert.Equal(t, 100, fetchErr.offset)

	records := diagnosticRecords(t, *logs)
	var summary map[string]any
	for _, record := range records {
		if record["event"] == "summary" {
			summary = record
		}
		assert.NotContains(t, fmt.Sprint(record), "later-page-secret")
	}
	require.NotNil(t, summary)
	assert.Equal(t, false, summary["inventory_complete"])
	assert.Equal(t, float64(http.StatusOK), summary["last_http_status"])
	assert.Equal(t, flagCollectionActive, summary["failed_collection"])
	assert.Equal(t, float64(100), summary["failed_offset"])
}

func TestFetcherDoesNotRetryTransportErrors(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport sentinel")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	fetcher := newFlagFetcher(ctx, fetcherOptions{httpClient: &http.Client{Transport: transport}})
	config := &lcr.Config{LdInstance: "https://example.test", LdProject: "project", LdEnvironment: "production", ApiToken: "token-sentinel"}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, "transport_error", fetcher.outcome(err))
	assert.NotContains(t, err.Error(), "example.test")
	assert.NotContains(t, err.Error(), "token-sentinel")
}

func TestFetcherRateLimitExhaustionReturnsEmptyInventory(t *testing.T) {
	pages := make([]flagPageResponse, maxRetries+1)
	for i := range pages {
		pages[i] = flagPageResponse{
			offset: 0,
			status: http.StatusTooManyRequests,
			body:   `{"message":"rate limited"}`,
		}
	}
	config := serveFlagPages(t, false, pages)
	fetcher, _ := newTestFetcher(t)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 5, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, 4, fetcher.diagnostics.retries)
	assert.Equal(t, 5, fetcher.diagnostics.responses429)
	assert.Equal(t, "rate_limit_exhausted", fetcher.outcome(err))
}

func TestFetcherDoesNotRetryPermanentHTTPStatus(t *testing.T) {
	config := serveFlagPages(t, false, []flagPageResponse{
		{status: http.StatusServiceUnavailable, body: `{"message":"unavailable"}`},
	})
	fetcher, _ := newTestFetcher(t)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
	assert.Equal(t, "status=503 reason=http_error", errorSuffix(err))
}

func TestFetcherAppliesProactiveWaitBeforeNextPage(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	waits := []time.Duration{}
	config := serveFlagPages(t, false, []flagPageResponse{
		{
			body: `{"items":[{"key":"first"}]}`,
			headers: http.Header{
				"X-Ratelimit-Route-Remaining": {"0"},
				"X-Ratelimit-Reset":           {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
			},
		},
		{offset: 100, body: `{"items":[]}`},
	})
	fetcher, _ := newTestFetcher(t)
	fetcher.clock = func() time.Time { return now }
	fetcher.wait = func(_ context.Context, wait time.Duration) error {
		waits = append(waits, wait)
		now = now.Add(wait)
		return nil
	}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.NoError(t, err)
	assert.Len(t, flags, 1)
	assert.Equal(t, []time.Duration{3 * time.Second}, waits)
	assert.Equal(t, 1, fetcher.diagnostics.proactiveWaits)
	assert.Equal(t, int64(3000), fetcher.diagnostics.scheduledWait.Milliseconds())
}

func TestFetcherWaitsAtActiveArchivedBoundary(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	waits := 0
	config := serveFlagPages(t, true, []flagPageResponse{
		{
			filter: "",
			body:   `{"items":[]}`,
			headers: http.Header{
				"X-Ratelimit-Global-Remaining": {"0"},
				"X-Ratelimit-Reset":            {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
			},
		},
		{filter: "state:archived", body: `{"items":[]}`},
	})
	fetcher, _ := newTestFetcher(t)
	fetcher.clock = func() time.Time { return now }
	fetcher.wait = func(_ context.Context, wait time.Duration) error {
		waits++
		now = now.Add(wait)
		return nil
	}

	active, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, true)
	require.NoError(t, err)
	params := urlValuesForFlags()
	params.Set("filter", "state:archived")
	archived, err := fetcher.getFlags(config, params, flagCollectionArchived, false)

	require.NoError(t, err)
	assert.Empty(t, active)
	assert.Empty(t, archived)
	assert.Equal(t, 1, waits)
}

func TestFetcherDoesNotWaitAfterFinalArchivedPage(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	waits := 0
	config := serveFlagPages(t, true, []flagPageResponse{
		{filter: "state:archived", body: `{"items":[]}`, headers: http.Header{
			"X-Ratelimit-Global-Remaining": {"0"},
			"X-Ratelimit-Reset":            {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
		}},
	})
	fetcher, _ := newTestFetcher(t)
	fetcher.clock = func() time.Time { return now }
	fetcher.wait = func(_ context.Context, wait time.Duration) error {
		waits++
		return nil
	}

	params := urlValuesForFlags()
	params.Set("filter", "state:archived")
	flags, err := fetcher.getFlags(config, params, flagCollectionArchived, false)

	require.NoError(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, 0, waits)
	assert.Equal(t, 0, fetcher.diagnostics.proactiveWaits)
}

func TestFetcherFailsBeforeArchivedRequestWhenResetCannotFit(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	config := serveFlagPages(t, true, []flagPageResponse{
		{
			body: `{"items":[]}`,
			headers: http.Header{
				"X-Ratelimit-Global-Remaining": {"0"},
				"X-Ratelimit-Reset": {
					strconv.FormatInt(now.Add(60*time.Second).UnixMilli(), 10),
				},
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	fetcher, _ := newTestFetcherWithContext(t, ctx)
	fetcher.clock = func() time.Time { return now }

	active, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, true)
	require.NoError(t, err)
	assert.Empty(t, active)

	params := urlValuesForFlags()
	params.Set("filter", "state:archived")
	archived, err := fetcher.getFlags(config, params, flagCollectionArchived, false)

	require.Error(t, err)
	assert.Empty(t, archived)
	assert.Equal(t, "rate_limit_deadline", fetcher.outcome(err))
	assert.Contains(t, err.Error(), "collection=archived offset=0")
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
}

func TestFetcherCancellationDuringRetryWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	config := serveFlagPages(t, false, []flagPageResponse{
		{status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
	})
	fetcher, _ := newTestFetcherWithContext(t, ctx)
	setTestBackoff(fetcher, func(wait time.Duration) time.Duration {
		cancel()
		return wait
	})

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "canceled", fetcher.outcome(err))
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
}

func TestFetcherCancellationDuringProactiveWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	now := time.Now().Truncate(time.Second)
	config := serveFlagPages(t, false, []flagPageResponse{
		{
			body: `{"items":[{"key":"first"}]}`,
			headers: http.Header{
				"X-Ratelimit-Route-Remaining": {"0"},
				"X-Ratelimit-Reset":           {strconv.FormatInt(now.Add(2*time.Second).UnixMilli(), 10)},
			},
		},
	})
	fetcher, _ := newTestFetcherWithContext(t, ctx)
	fetcher.clock = func() time.Time { return now }
	fetcher.wait = func(waitContext context.Context, _ time.Duration) error {
		cancel()
		return waitContext.Err()
	}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "canceled", fetcher.outcome(err))
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
}

func TestCallerDeadlineWinsRateLimitRetryDeadline(t *testing.T) {
	config := serveFlagPages(t, false, []flagPageResponse{{
		status: http.StatusTooManyRequests,
		headers: http.Header{
			"Retry-After": {strconv.FormatInt(60, 10)},
		},
		body: `{"message":"rate limited"}`,
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	flags, err := GetAllFlags(ctx, config)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "reason=rate_limit_deadline")
}

func TestCallerDeadlineCoversActiveAndArchivedCollections(t *testing.T) {
	var requests atomic.Int32
	startedArchived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") == "state:archived" {
			requests.Add(1)
			close(startedArchived)
			<-r.Context().Done()
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(server.Close)
	config := &lcr.Config{
		LdInstance:           server.URL,
		LdProject:            "test-project",
		LdEnvironment:        "production",
		ApiToken:             "token-sentinel",
		IncludeArchivedFlags: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	flags, err := GetAllFlags(ctx, config)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(2), requests.Load())
	select {
	case <-startedArchived:
	default:
		t.Fatal("archived collection was not requested")
	}
}

func TestFetcherCancellationDuringHTTP(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	fetcher, _ := newTestFetcherWithContext(t, ctx)
	config := &lcr.Config{
		LdInstance:    server.URL,
		LdProject:     "test-project",
		LdEnvironment: "production",
		ApiToken:      "token-sentinel",
	}
	result := make(chan error, 1)
	go func() {
		_, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)
		result <- err
	}()
	<-started
	cancel()
	err := <-result

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "canceled", fetcher.outcome(err))
	assert.Equal(t, 1, fetcher.diagnostics.httpAttempts)
}

func TestFetcherCancellationBeforeFirstRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	config := serveFlagPages(t, false, nil)
	fetcher, _ := newTestFetcherWithContext(t, ctx)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "canceled", fetcher.outcome(err))
	assert.Equal(t, 0, fetcher.diagnostics.httpAttempts)
}

func TestFetcherResponseBodiesCloseOnSuccessAndRetry(t *testing.T) {
	var closed atomic.Int32
	responses := []trackingResponse{
		{status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
		{status: http.StatusOK, body: `{"items":[]}`},
	}
	transport := &trackingRoundTripper{responses: responses, closed: &closed}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	fetcher := newFlagFetcher(ctx, fetcherOptions{
		httpClient: &http.Client{Transport: transport},
		jitter:     func(_, maximum time.Duration) time.Duration { return maximum },
	})
	setTestBackoff(fetcher, func(time.Duration) time.Duration { return 0 })
	config := &lcr.Config{LdInstance: "https://example.test", LdProject: "project", LdEnvironment: "production", ApiToken: "sentinel-token"}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.NoError(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, int32(2), closed.Load())
}

func TestFetcherClosesFinalResponseOnDecodeError(t *testing.T) {
	var closed atomic.Int32
	transport := &trackingRoundTripper{
		responses: []trackingResponse{{status: http.StatusOK, body: "not JSON"}},
		closed:    &closed,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	fetcher := newFlagFetcher(ctx, fetcherOptions{
		httpClient: &http.Client{Transport: transport},
	})
	config := &lcr.Config{
		LdInstance:    "https://example.test",
		LdProject:     "project",
		LdEnvironment: "production",
		ApiToken:      "sentinel-token",
	}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, int32(1), closed.Load())
}

func TestFetcherClosesFinalResponseReturnedWithError(t *testing.T) {
	var closed atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     http.Header{"Retry-After": {"60"}},
			Body: &trackingBody{
				Reader: strings.NewReader(`{"message":"rate limited"}`),
				closed: &closed,
			},
			Request: request,
		}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	fetcher := newFlagFetcher(ctx, fetcherOptions{
		httpClient: &http.Client{Transport: transport},
	})
	config := &lcr.Config{
		LdInstance:    "https://example.test",
		LdProject:     "project",
		LdEnvironment: "production",
		ApiToken:      "sentinel-token",
	}

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.Equal(t, "rate_limit_deadline", fetcher.outcome(err))
	assert.Equal(t, int32(1), closed.Load())
}

func TestFetchErrorsAndDiagnosticsDoNotEchoSensitiveValues(t *testing.T) {
	const sentinel = "flag-secret-name-authorization-token"
	config := serveFlagPages(t, false, []flagPageResponse{
		{status: http.StatusBadGateway, body: `{"message":"` + sentinel + `"}`},
	})
	fetcher, logs := newTestFetcher(t)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.Error(t, err)
	assert.Empty(t, flags)
	assert.NotContains(t, err.Error(), sentinel)
	assert.NotContains(t, strings.Join(*logs, "\n"), sentinel)
}

func TestMalformedRateLimitHeadersAreNotLogged(t *testing.T) {
	const sentinel = "malformed-header-secret"
	config := serveFlagPages(t, false, []flagPageResponse{
		{
			status: http.StatusTooManyRequests,
			body:   `{"message":"rate limited"}`,
			headers: http.Header{
				"Retry-After":       {sentinel},
				"X-Ratelimit-Reset": {sentinel},
			},
		},
		{body: `{"items":[]}`},
	})
	fetcher, logs := newTestFetcher(t)

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)

	require.NoError(t, err)
	assert.Empty(t, flags)
	assert.NotContains(t, strings.Join(*logs, "\n"), sentinel)
}

func TestFetcherDiagnosticsHaveOneStartAndSummary(t *testing.T) {
	tests := []struct {
		name             string
		pages            []flagPageResponse
		wantAttempts     float64
		wantRetries      float64
		want429          float64
		wantSuccessful   float64
		wantNonempty     float64
		wantRetryWaits   float64
		wantProactive    float64
		wantWaitEvents   int
		wantRateLimitLog int
	}{
		{
			name: "healthy",
			pages: []flagPageResponse{
				{body: `{"items":[{"key":"healthy"}]}`},
				{offset: 100, body: `{"items":[]}`},
			},
			wantAttempts:   2,
			wantSuccessful: 2,
			wantNonempty:   1,
		},
		{
			name: "recovered rate limit",
			pages: []flagPageResponse{
				{status: http.StatusTooManyRequests, body: `{"message":"rate limited"}`},
				{body: `{"items":[{"key":"recovered"}]}`},
				{offset: 100, body: `{"items":[]}`},
			},
			wantAttempts:     3,
			wantRetries:      1,
			want429:          1,
			wantSuccessful:   2,
			wantNonempty:     1,
			wantRetryWaits:   1,
			wantWaitEvents:   1,
			wantRateLimitLog: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := serveFlagPages(t, false, tt.pages)
			fetcher, logs := newTestFetcher(t)
			fetcher.diagnostics.emitStart()

			_, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)
			fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)

			records := diagnosticRecords(t, *logs)
			counts := map[string]int{}
			var summary map[string]any
			for _, record := range records {
				event, ok := record["event"].(string)
				require.True(t, ok)
				counts[event]++
				if event == "summary" {
					summary = record
				}
			}

			assert.Equal(t, 1, counts["start"])
			assert.Equal(t, 1, counts["summary"])
			assert.Equal(t, tt.wantWaitEvents, counts["wait"])
			assert.Equal(t, tt.wantRateLimitLog, counts["rate_limit"])
			require.NotNil(t, summary)
			assert.Equal(t, "success", summary["outcome"])
			assert.Equal(t, true, summary["inventory_complete"])
			assert.Equal(t, tt.wantAttempts, summary["http_attempts"])
			assert.Equal(t, tt.wantRetries, summary["retries"])
			assert.Equal(t, tt.want429, summary["responses_429"])
			assert.Equal(t, tt.wantSuccessful, summary["successful_pages"])
			assert.Equal(t, tt.wantNonempty, summary["nonempty_pages"])
			assert.Equal(t, tt.wantRetryWaits, summary["retry_waits"])
			assert.Equal(t, tt.wantProactive, summary["proactive_waits"])
		})
	}
}

func TestFetcherDiagnosticsReportRateLimitFailure(t *testing.T) {
	pages := make([]flagPageResponse, maxRetries+1)
	for index := range pages {
		pages[index] = flagPageResponse{
			status: http.StatusTooManyRequests,
			body:   `{"message":"rate limited"}`,
		}
	}
	config := serveFlagPages(t, false, pages)
	fetcher, logs := newTestFetcher(t)
	fetcher.diagnostics.emitStart()

	flags, err := fetcher.getFlags(config, urlValuesForFlags(), flagCollectionActive, false)
	fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)

	require.Error(t, err)
	assert.Empty(t, flags)
	records := diagnosticRecords(t, *logs)
	counts := map[string]int{}
	var summary map[string]any
	for _, record := range records {
		event, ok := record["event"].(string)
		require.True(t, ok)
		counts[event]++
		if event == "summary" {
			summary = record
		}
	}

	assert.Equal(t, 1, counts["start"])
	assert.Equal(t, 1, counts["summary"])
	assert.Equal(t, maxRetries, counts["wait"])
	assert.Equal(t, maxRetries+1, counts["rate_limit"])
	require.NotNil(t, summary)
	assert.Equal(t, "rate_limit_exhausted", summary["outcome"])
	assert.Equal(t, false, summary["inventory_complete"])
	assert.Equal(t, float64(maxRetries+1), summary["http_attempts"])
	assert.Equal(t, float64(maxRetries), summary["retries"])
	assert.Equal(t, float64(maxRetries+1), summary["responses_429"])
	assert.Equal(t, float64(0), summary["successful_pages"])
	assert.Equal(t, float64(0), summary["nonempty_pages"])
	assert.Equal(t, "active", summary["failed_collection"])
	assert.Equal(t, float64(0), summary["failed_offset"])
}

func diagnosticRecords(t *testing.T, logs []string) []map[string]any {
	t.Helper()
	records := make([]map[string]any, 0, len(logs))
	for _, logLine := range logs {
		const prefix = "LD_FLAG_FETCH "
		require.True(t, strings.HasPrefix(logLine, prefix))
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(logLine, prefix))), &record))
		records = append(records, record)
	}
	return records
}

func newTestFetcher(t *testing.T) (*flagFetcher, *[]string) {
	return newTestFetcherWithContext(t, context.Background())
}

func newTestFetcherWithContext(t *testing.T, parent context.Context) (*flagFetcher, *[]string) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	t.Cleanup(cancel)
	logs := &[]string{}
	fetcher := newFlagFetcher(ctx, fetcherOptions{
		jitter: func(_, maximum time.Duration) time.Duration { return maximum },
		log: func(format string, args ...any) {
			*logs = append(*logs, fmt.Sprintf(format, args...))
		},
	})
	setTestBackoff(fetcher, func(time.Duration) time.Duration { return 0 })
	return fetcher, logs
}

func setTestBackoff(fetcher *flagFetcher, transform func(time.Duration) time.Duration) {
	fetcher.client.Backoff = func(minimum, maximum time.Duration, attempt int, response *http.Response) time.Duration {
		return transform(fetcher.retryBackoff(minimum, maximum, attempt, response))
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func urlValuesForFlags() url.Values {
	return url.Values{"env": {"production"}, "limit": {"100"}}
}

func keys(flags []ldapi.FeatureFlag) []string {
	keys := make([]string, len(flags))
	for i, flag := range flags {
		keys[i] = flag.Key
	}
	return keys
}

func errorSuffix(err error) string {
	message := err.Error()
	if index := strings.Index(message, "status="); index >= 0 {
		return message[index:]
	}
	return message
}

type trackingResponse struct {
	status  int
	body    string
	headers http.Header
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(buffer []byte) (int, error) {
	return f(buffer)
}

type trackingRoundTripper struct {
	responses []trackingResponse
	closed    *atomic.Int32
	index     atomic.Int32
}

func (t *trackingRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	index := int(t.index.Add(1)) - 1
	response := t.responses[index]
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	headers := response.headers
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status),
		Header:     headers,
		Body: &trackingBody{
			Reader: strings.NewReader(response.body),
			closed: t.closed,
		},
		Request: &http.Request{},
	}, nil
}

type trackingBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b *trackingBody) Close() error {
	b.closed.Add(1)
	return nil
}

var _ io.ReadCloser = (*trackingBody)(nil)
