package ldapi

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

const (
	inventoryTimeout = 120 * time.Second
	attemptTimeout   = 30 * time.Second
	maxRetries       = 4

	flagCollectionActive   = "active"
	flagCollectionArchived = "archived"
)

const maxInt64Value = int64(1<<63 - 1)

type fetcherOptions struct {
	clock      func() time.Time
	jitter     func(time.Duration, time.Duration) time.Duration
	wait       func(context.Context, time.Duration) error
	httpClient *http.Client
	log        diagnosticLog
}

type flagFetcher struct {
	ctx context.Context

	client      *retryablehttp.Client
	clock       func() time.Time
	jitter      func(time.Duration, time.Duration) time.Duration
	wait        func(context.Context, time.Duration) error
	diagnostics *fetchDiagnostics

	collection string
	offset     int
	attempt    int

	pendingRetryWait       time.Duration
	waitingRetry           bool
	waitingProactive       bool
	notBefore              time.Time
	hasNotBefore           bool
	notBeforeUnfulfillable bool
}

func newFlagFetcher(ctx context.Context, options fetcherOptions) *flagFetcher {
	if options.clock == nil {
		options.clock = time.Now
	}
	if options.jitter == nil {
		options.jitter = uniformJitter
	}
	if options.wait == nil {
		options.wait = waitWithContext
	}

	client := retryablehttp.NewClient()
	if options.httpClient != nil {
		client.HTTPClient = options.httpClient
	}
	client.HTTPClient.Timeout = attemptTimeout
	client.Logger = nil
	client.RetryMax = maxRetries
	fetcher := &flagFetcher{
		ctx:         ctx,
		client:      client,
		clock:       options.clock,
		jitter:      options.jitter,
		wait:        options.wait,
		diagnostics: newFetchDiagnostics(options.clock(), options.log),
	}
	client.CheckRetry = fetcher.checkRetry
	client.Backoff = fetcher.retryBackoff
	client.RequestLogHook = fetcher.requestLogHook
	client.ResponseLogHook = fetcher.responseLogHook
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	return fetcher
}

func (f *flagFetcher) requestLogHook(_ retryablehttp.Logger, _ *http.Request, retryNumber int) {
	f.attempt = retryNumber + 1
	f.waitingRetry = false
	f.pendingRetryWait = 0
	f.diagnostics.recordAttempt(retryNumber)
}

func (f *flagFetcher) responseLogHook(_ retryablehttp.Logger, response *http.Response) {
	if response != nil {
		f.diagnostics.recordStatus(response.StatusCode)
	}
}

func (f *flagFetcher) retryBackoff(_ time.Duration, _ time.Duration, _ int, _ *http.Response) time.Duration {
	wait := f.pendingRetryWait
	f.pendingRetryWait = 0
	return wait
}

func (f *flagFetcher) checkRetry(ctx context.Context, response *http.Response, requestErr error) (bool, error) {
	if response != nil {
		f.diagnostics.recordStatus(response.StatusCode)
	}

	isRateLimited := response != nil && response.StatusCode == http.StatusTooManyRequests
	if isRateLimited {
		hints := parseRateLimitHeaders(response.Header)
		f.diagnostics.responses429++
		f.diagnostics.emitRateLimit(f.collection, f.offset, f.currentAttempt(), response.StatusCode, hints)
	}

	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if requestErr != nil {
		return false, nil
	}
	if !isRateLimited {
		return false, nil
	}

	attempt := f.currentAttempt()
	if attempt > maxRetries+1 {
		return true, nil
	}
	if attempt == maxRetries+1 {
		// Keep shouldRetry true so PassthroughErrorHandler returns the final 429.
		return true, nil
	}

	hints := parseRateLimitHeaders(response.Header)
	deadline, hasDeadline := f.ctx.Deadline()
	if !hasDeadline {
		deadline = time.Time{}
	}
	decision := retryWait(
		f.clock(),
		deadlinePointer(deadline, hasDeadline),
		attempt-1,
		hints,
		f.jitter,
	)
	if decision.unfulfillable {
		return false, f.newError(reasonRateLimitDeadline, http.StatusTooManyRequests, context.DeadlineExceeded)
	}

	f.pendingRetryWait = decision.delay
	f.waitingRetry = true
	f.diagnostics.recordRetryWait(decision.delay)
	f.diagnostics.emitWait(f.collection, f.offset, attempt+1, decision.reason, decision.delay)
	return true, nil
}

func (f *flagFetcher) currentAttempt() int {
	if f.attempt > 0 {
		return f.attempt
	}
	return 1
}

type fetchErrorReason string

const (
	reasonCanceled           fetchErrorReason = "canceled"
	reasonDeadline           fetchErrorReason = "deadline"
	reasonDecode             fetchErrorReason = "decode_error"
	reasonHTTP               fetchErrorReason = "http_error"
	reasonRateLimitDeadline  fetchErrorReason = "rate_limit_deadline"
	reasonRateLimitExhausted fetchErrorReason = "rate_limit_exhausted"
	reasonRequest            fetchErrorReason = "request_error"
	reasonTransport          fetchErrorReason = "transport_error"
)

type flagFetchError struct {
	reason     fetchErrorReason
	collection string
	offset     int
	attempt    int
	status     int
	cause      error
}

func (e *flagFetchError) Error() string {
	message := "feature flag fetch failed"
	if e.collection != "" {
		message += " collection=" + e.collection
	}
	message += " offset=" + strconv.Itoa(e.offset)
	if e.attempt > 0 {
		message += " attempt=" + strconv.Itoa(e.attempt)
	}
	if e.status > 0 {
		message += " status=" + strconv.Itoa(e.status)
	}
	message += " reason=" + string(e.reason)
	return message
}

func (e *flagFetchError) Unwrap() error {
	return e.cause
}

func (f *flagFetcher) newError(reason fetchErrorReason, status int, cause error) *flagFetchError {
	if reason == reasonRateLimitDeadline && cause == nil {
		cause = context.DeadlineExceeded
	}
	return &flagFetchError{
		reason:     reason,
		collection: f.collection,
		offset:     f.offset,
		attempt:    f.currentAttempt(),
		status:     status,
		cause:      cause,
	}
}

func (f *flagFetcher) contextError() error {
	contextErr := f.ctx.Err()
	if contextErr == nil {
		return nil
	}
	reason := reasonDeadline
	if errors.Is(contextErr, context.Canceled) {
		reason = reasonCanceled
	}
	if (f.waitingRetry || f.waitingProactive) && errors.Is(contextErr, context.DeadlineExceeded) {
		reason = reasonRateLimitDeadline
	}
	return f.newError(reason, 0, contextErr)
}

func (f *flagFetcher) wrapRequestError(requestErr error) error {
	if requestErr == nil {
		return nil
	}
	if existing := new(flagFetchError); errors.As(requestErr, &existing) {
		return requestErr
	}
	if contextErr := f.ctx.Err(); contextErr != nil {
		reason := reasonDeadline
		if errors.Is(contextErr, context.Canceled) {
			reason = reasonCanceled
		}
		if (f.waitingRetry || f.waitingProactive) && errors.Is(contextErr, context.DeadlineExceeded) {
			reason = reasonRateLimitDeadline
		}
		return f.newError(reason, 0, requestErr)
	}
	return f.newError(reasonTransport, 0, requestErr)
}

func (f *flagFetcher) wrapDecodeError(decodeErr error) *flagFetchError {
	reason := reasonDecode
	cause := decodeErr

	switch {
	case errors.Is(decodeErr, context.Canceled):
		reason = reasonCanceled
	case errors.Is(decodeErr, context.DeadlineExceeded):
		reason = reasonDeadline
	default:
		var timeoutErr net.Error
		if errors.As(decodeErr, &timeoutErr) && timeoutErr.Timeout() {
			reason = reasonDeadline
		} else if contextErr := f.ctx.Err(); contextErr != nil {
			cause = errors.Join(decodeErr, contextErr)
			if errors.Is(contextErr, context.Canceled) {
				reason = reasonCanceled
			} else {
				reason = reasonDeadline
			}
		}
	}

	return f.newError(reason, http.StatusOK, cause)
}

func (f *flagFetcher) outcome(err error) string {
	if err == nil {
		return "success"
	}
	var fetchErr *flagFetchError
	if errors.As(err, &fetchErr) {
		switch fetchErr.reason {
		case reasonCanceled:
			return "canceled"
		case reasonDeadline:
			return "deadline"
		case reasonDecode:
			return "decode_error"
		case reasonHTTP:
			return "http_error"
		case reasonRateLimitDeadline:
			return "rate_limit_deadline"
		case reasonRateLimitExhausted:
			return "rate_limit_exhausted"
		case reasonTransport, reasonRequest:
			return "transport_error"
		}
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline"
	}
	return "transport_error"
}

func deadlinePointer(deadline time.Time, ok bool) *time.Time {
	if !ok {
		return nil
	}
	return &deadline
}

func waitWithContext(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func uniformJitter(minimum, maximum time.Duration) time.Duration {
	if maximum <= minimum {
		return minimum
	}
	return minimum + time.Duration(rand.Int63n(int64(maximum-minimum)+1))
}

type parsedRateLimitHeaders struct {
	globalResetValues    []int64
	authTokenResetValues []int64
	globalResetOverflow  bool
	authResetOverflow    bool

	retryAfterSeconds  []uint64
	retryAfterDates    []time.Time
	retryAfterOverflow bool

	globalRemaining    *int64
	routeRemaining     *int64
	authTokenRemaining *int64
}

func parseRateLimitHeaders(headers http.Header) parsedRateLimitHeaders {
	parsed := parsedRateLimitHeaders{}
	for _, value := range headers.Values("X-Ratelimit-Reset") {
		parsedValue, valid, overflow := parseUnsignedHeader(value)
		if overflow || valid && parsedValue > uint64(maxInt64Value) {
			parsed.globalResetOverflow = true
		}
		if valid && parsedValue <= uint64(maxInt64Value) {
			parsed.globalResetValues = append(parsed.globalResetValues, int64(parsedValue))
		}
	}
	for _, value := range headers.Values("X-Ratelimit-Auth-Token-Reset") {
		parsedValue, valid, overflow := parseUnsignedHeader(value)
		if overflow || valid && parsedValue > uint64(maxInt64Value) {
			parsed.authResetOverflow = true
		}
		if valid && parsedValue <= uint64(maxInt64Value) {
			parsed.authTokenResetValues = append(parsed.authTokenResetValues, int64(parsedValue))
		}
	}
	for _, value := range headers.Values("Retry-After") {
		trimmed := strings.TrimSpace(value)
		parsedValue, valid, overflow := parseUnsignedHeader(trimmed)
		if overflow {
			parsed.retryAfterOverflow = true
		}
		if valid {
			parsed.retryAfterSeconds = append(parsed.retryAfterSeconds, parsedValue)
			continue
		}
		if date, err := http.ParseTime(trimmed); err == nil {
			parsed.retryAfterDates = append(parsed.retryAfterDates, date)
		}
	}
	parsed.globalRemaining = parseRemainingHeader(headers, "X-Ratelimit-Global-Remaining")
	parsed.routeRemaining = parseRemainingHeader(headers, "X-Ratelimit-Route-Remaining")
	parsed.authTokenRemaining = parseRemainingHeader(headers, "X-Ratelimit-Auth-Token-Remaining")
	return parsed
}

func parseUnsignedHeader(value string) (uint64, bool, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.HasPrefix(trimmed, "-") {
		return 0, false, false
	}
	parsed, err := strconv.ParseUint(trimmed, 10, 64)
	if err == nil {
		return parsed, true, false
	}
	if errors.Is(err, strconv.ErrRange) {
		return 0, false, true
	}
	return 0, false, false
}

func parseRemainingHeader(headers http.Header, name string) *int64 {
	for _, value := range headers.Values(name) {
		parsed, valid, _ := parseUnsignedHeader(value)
		if valid && parsed <= uint64(maxInt64Value) {
			converted := int64(parsed)
			return &converted
		}
	}
	return nil
}

func (h parsedRateLimitHeaders) globalResetMillis() *int64 {
	return maxInt64Pointer(h.globalResetValues)
}

func (h parsedRateLimitHeaders) authTokenResetMillis() *int64 {
	return maxInt64Pointer(h.authTokenResetValues)
}

func (h parsedRateLimitHeaders) retryAfterSecondsValue() *int64 {
	var selected *int64
	for _, value := range h.retryAfterSeconds {
		if value > uint64(maxInt64Value) {
			continue
		}
		converted := int64(value)
		if selected == nil || converted > *selected {
			selected = &converted
		}
	}
	return selected
}

func (h parsedRateLimitHeaders) retryAfterDateMillis() *int64 {
	var selected *int64
	for _, date := range h.retryAfterDates {
		converted := date.UnixMilli()
		if selected == nil || converted > *selected {
			selected = &converted
		}
	}
	return selected
}

func (h parsedRateLimitHeaders) globalRemainingValue() *int64 {
	return h.globalRemaining
}

func (h parsedRateLimitHeaders) routeRemainingValue() *int64 {
	return h.routeRemaining
}

func (h parsedRateLimitHeaders) authTokenRemainingValue() *int64 {
	return h.authTokenRemaining
}

func maxInt64Pointer(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return &maximum
}

type waitDecision struct {
	delay         time.Duration
	reason        string
	unfulfillable bool
}

func retryWait(
	now time.Time,
	deadline *time.Time,
	retryNumber int,
	hints parsedRateLimitHeaders,
	jitter func(time.Duration, time.Duration) time.Duration,
) waitDecision {
	latest, hasFuture, unfulfillable := latestFutureHint(now, deadline, hints)
	if unfulfillable {
		return waitDecision{reason: "rate_limit_reset", unfulfillable: true}
	}

	if hasFuture {
		wait := latest.Sub(now)
		jitterWait := boundedJitter(jitter, 100*time.Millisecond, time.Second)
		if wait > time.Duration(maxInt64Value)-jitterWait {
			return waitDecision{reason: "rate_limit_reset", unfulfillable: true}
		}
		wait += jitterWait
		if !fitsDeadline(now, deadline, wait) {
			return waitDecision{reason: "rate_limit_reset", unfulfillable: true}
		}
		return waitDecision{delay: wait, reason: "rate_limit_reset"}
	}

	window := retryWindow(retryNumber)
	wait := boundedJitter(jitter, window/2, window)
	if !fitsDeadline(now, deadline, wait) {
		return waitDecision{reason: "rate_limit_backoff", unfulfillable: true}
	}
	return waitDecision{delay: wait, reason: "rate_limit_backoff"}
}

func latestFutureHint(now time.Time, deadline *time.Time, hints parsedRateLimitHeaders) (time.Time, bool, bool) {
	latest, hasFuture, unfulfillable := latestRetryAfterHint(now, deadline, hints)
	if unfulfillable {
		return time.Time{}, false, true
	}
	consider := func(at time.Time) bool {
		if !at.After(now) {
			return true
		}
		if deadline != nil && at.After(*deadline) {
			return false
		}
		if !hasFuture || at.After(latest) {
			latest = at
			hasFuture = true
		}
		return true
	}
	considerResets := func(values []int64, overflow bool) bool {
		if overflow {
			return false
		}
		for _, millis := range values {
			if deadline != nil && millis > deadline.UnixMilli() {
				return false
			}
			if !consider(time.UnixMilli(millis)) {
				return false
			}
		}
		return true
	}

	sharedResetApplicable := (hints.globalRemaining != nil && *hints.globalRemaining == 0) ||
		(hints.routeRemaining != nil && *hints.routeRemaining == 0) ||
		(hints.globalRemaining == nil && hints.routeRemaining == nil)
	if sharedResetApplicable && !considerResets(hints.globalResetValues, hints.globalResetOverflow) {
		return time.Time{}, false, true
	}
	tokenResetApplicable := hints.authTokenRemaining == nil || *hints.authTokenRemaining == 0
	if tokenResetApplicable && !considerResets(hints.authTokenResetValues, hints.authResetOverflow) {
		return time.Time{}, false, true
	}
	return latest, hasFuture, false
}

func retryWindow(retryNumber int) time.Duration {
	window := 2 * time.Second
	for i := 0; i < retryNumber && window < 30*time.Second; i++ {
		if window > 15*time.Second {
			return 30 * time.Second
		}
		window *= 2
	}
	if window > 30*time.Second {
		return 30 * time.Second
	}
	return window
}

func fitsDeadline(now time.Time, deadline *time.Time, wait time.Duration) bool {
	if deadline == nil {
		return true
	}
	remaining := deadline.Sub(now)
	return wait >= 0 && wait < remaining
}

func boundedJitter(jitter func(time.Duration, time.Duration) time.Duration, minimum, maximum time.Duration) time.Duration {
	value := jitter(minimum, maximum)
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func proactiveWait(
	now time.Time,
	deadline *time.Time,
	hints parsedRateLimitHeaders,
	jitter func(time.Duration, time.Duration) time.Duration,
) waitDecision {
	var wait time.Duration
	serviceDelay := false

	considerQuota := func(remaining *int64, resetValues []int64, resetOverflow bool) bool {
		if remaining == nil || *remaining != 0 {
			return true
		}
		if resetOverflow {
			return false
		}
		if len(resetValues) == 0 {
			fallback := boundedJitter(jitter, time.Second, 2*time.Second)
			if fallback > wait {
				wait = fallback
			}
			return true
		}
		var futureReset time.Time
		for _, millis := range resetValues {
			if deadline != nil && millis > deadline.UnixMilli() {
				return false
			}
			reset := time.UnixMilli(millis)
			if reset.After(now) && (futureReset.IsZero() || reset.After(futureReset)) {
				futureReset = reset
			}
		}
		if futureReset.IsZero() {
			return true
		}
		resetWait := futureReset.Sub(now)
		if resetWait > wait {
			wait = resetWait
		}
		serviceDelay = true
		return true
	}

	if !considerQuota(hints.globalRemaining, hints.globalResetValues, hints.globalResetOverflow) {
		return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
	}
	if !considerQuota(hints.routeRemaining, hints.globalResetValues, hints.globalResetOverflow) {
		return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
	}
	if !considerQuota(hints.authTokenRemaining, hints.authTokenResetValues, hints.authResetOverflow) {
		return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
	}

	if hints.globalRemaining != nil && *hints.globalRemaining == 0 ||
		hints.routeRemaining != nil && *hints.routeRemaining == 0 ||
		hints.authTokenRemaining != nil && *hints.authTokenRemaining == 0 {
		latest, hasFuture, unfulfillable := latestRetryAfterHint(now, deadline, hints)
		if unfulfillable {
			return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
		}
		if hasFuture {
			retryAfterWait := latest.Sub(now)
			if retryAfterWait > wait {
				wait = retryAfterWait
			}
			serviceDelay = serviceDelay || retryAfterWait > 0
		}
	}

	if wait <= 0 {
		return waitDecision{reason: "proactive_rate_limit"}
	}
	if serviceDelay {
		jitterWait := boundedJitter(jitter, 100*time.Millisecond, time.Second)
		if wait > time.Duration(maxInt64Value)-jitterWait {
			return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
		}
		wait += jitterWait
	}
	if !fitsDeadline(now, deadline, wait) {
		return waitDecision{reason: "proactive_rate_limit", unfulfillable: true}
	}
	return waitDecision{delay: wait, reason: "proactive_rate_limit"}
}

func latestRetryAfterHint(now time.Time, deadline *time.Time, hints parsedRateLimitHeaders) (time.Time, bool, bool) {
	if hints.retryAfterOverflow {
		return time.Time{}, false, true
	}
	var latest time.Time
	hasFuture := false
	consider := func(at time.Time) bool {
		if !at.After(now) {
			return true
		}
		if deadline != nil && at.After(*deadline) {
			return false
		}
		if !hasFuture || at.After(latest) {
			latest = at
			hasFuture = true
		}
		return true
	}
	for _, seconds := range hints.retryAfterSeconds {
		if seconds > uint64(maxInt64Value)/uint64(time.Second) {
			return time.Time{}, false, true
		}
		wait := time.Duration(seconds) * time.Second
		if deadline != nil && !fitsDeadline(now, deadline, wait) {
			return time.Time{}, false, true
		}
		if !consider(now.Add(wait)) {
			return time.Time{}, false, true
		}
	}
	for _, date := range hints.retryAfterDates {
		if !consider(date) {
			return time.Time{}, false, true
		}
	}
	return latest, hasFuture, false
}
