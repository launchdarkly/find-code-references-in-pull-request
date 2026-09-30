package ldapi

import (
	"encoding/json"
	"time"
)

const diagnosticsSchema = 1

type diagnosticLog func(string, ...any)

type fetchDiagnostics struct {
	log       diagnosticLog
	startedAt time.Time

	httpAttempts    int
	retries         int
	responses429    int
	successfulPages int
	nonemptyPages   int
	activeFlags     int
	archivedFlags   int
	retryWaits      int
	proactiveWaits  int
	scheduledWait   time.Duration
	lastHTTPStatus  *int

	failedCollection *string
	failedOffset     *int
}

func newFetchDiagnostics(startedAt time.Time, log diagnosticLog) *fetchDiagnostics {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &fetchDiagnostics{log: log, startedAt: startedAt}
}

func (d *fetchDiagnostics) emitStart() {
	d.emit(struct {
		Schema int    `json:"schema"`
		Event  string `json:"event"`
	}{
		Schema: diagnosticsSchema,
		Event:  "start",
	})
}

type rateLimitDiagnostic struct {
	Schema               int    `json:"schema"`
	Event                string `json:"event"`
	Collection           string `json:"collection"`
	Offset               int    `json:"offset"`
	Attempt              int    `json:"attempt"`
	Status               int    `json:"status"`
	GlobalResetMillis    *int64 `json:"global_reset_ms"`
	AuthTokenResetMillis *int64 `json:"auth_token_reset_ms"`
	RetryAfterSeconds    *int64 `json:"retry_after_seconds"`
	RetryAfterDateMillis *int64 `json:"retry_after_date_ms"`
	GlobalRemaining      *int64 `json:"global_remaining"`
	RouteRemaining       *int64 `json:"route_remaining"`
	AuthTokenRemaining   *int64 `json:"auth_token_remaining"`
}

func (d *fetchDiagnostics) emitRateLimit(collection string, offset, attempt, status int, hints parsedRateLimitHeaders) {
	d.emit(rateLimitDiagnostic{
		Schema:               diagnosticsSchema,
		Event:                "rate_limit",
		Collection:           collection,
		Offset:               offset,
		Attempt:              attempt,
		Status:               status,
		GlobalResetMillis:    hints.globalResetMillis(),
		AuthTokenResetMillis: hints.authTokenResetMillis(),
		RetryAfterSeconds:    hints.retryAfterSecondsValue(),
		RetryAfterDateMillis: hints.retryAfterDateMillis(),
		GlobalRemaining:      hints.globalRemainingValue(),
		RouteRemaining:       hints.routeRemainingValue(),
		AuthTokenRemaining:   hints.authTokenRemainingValue(),
	})
}

type waitDiagnostic struct {
	Schema             int    `json:"schema"`
	Event              string `json:"event"`
	Collection         string `json:"collection"`
	Offset             int    `json:"offset"`
	NextAttempt        int    `json:"next_attempt"`
	Reason             string `json:"reason"`
	IntendedWaitMillis int64  `json:"intended_wait_ms"`
}

func (d *fetchDiagnostics) emitWait(collection string, offset, nextAttempt int, reason string, wait time.Duration) {
	d.emit(waitDiagnostic{
		Schema:             diagnosticsSchema,
		Event:              "wait",
		Collection:         collection,
		Offset:             offset,
		NextAttempt:        nextAttempt,
		Reason:             reason,
		IntendedWaitMillis: wait.Milliseconds(),
	})
}

type summaryDiagnostic struct {
	Schema            int     `json:"schema"`
	Event             string  `json:"event"`
	Outcome           string  `json:"outcome"`
	InventoryComplete bool    `json:"inventory_complete"`
	HTTPAttempts      int     `json:"http_attempts"`
	Retries           int     `json:"retries"`
	Responses429      int     `json:"responses_429"`
	SuccessfulPages   int     `json:"successful_pages"`
	NonemptyPages     int     `json:"nonempty_pages"`
	ActiveFlags       int     `json:"active_flags"`
	ArchivedFlags     int     `json:"archived_flags"`
	RetryWaits        int     `json:"retry_waits"`
	ProactiveWaits    int     `json:"proactive_waits"`
	ScheduledWaitMs   int64   `json:"scheduled_wait_ms"`
	ElapsedMs         int64   `json:"elapsed_ms"`
	LastHTTPStatus    *int    `json:"last_http_status"`
	FailedCollection  *string `json:"failed_collection"`
	FailedOffset      *int    `json:"failed_offset"`
}

func (d *fetchDiagnostics) emitSummary(now time.Time, outcome string, complete bool) {
	elapsed := now.Sub(d.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	d.emit(summaryDiagnostic{
		Schema:            diagnosticsSchema,
		Event:             "summary",
		Outcome:           outcome,
		InventoryComplete: complete,
		HTTPAttempts:      d.httpAttempts,
		Retries:           d.retries,
		Responses429:      d.responses429,
		SuccessfulPages:   d.successfulPages,
		NonemptyPages:     d.nonemptyPages,
		ActiveFlags:       d.activeFlags,
		ArchivedFlags:     d.archivedFlags,
		RetryWaits:        d.retryWaits,
		ProactiveWaits:    d.proactiveWaits,
		ScheduledWaitMs:   d.scheduledWait.Milliseconds(),
		ElapsedMs:         elapsed.Milliseconds(),
		LastHTTPStatus:    d.lastHTTPStatus,
		FailedCollection:  d.failedCollection,
		FailedOffset:      d.failedOffset,
	})
}

func (d *fetchDiagnostics) emit(value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	d.log("LD_FLAG_FETCH %s\n", encoded)
}

func (d *fetchDiagnostics) recordAttempt(retryNumber int) {
	d.httpAttempts++
	if retryNumber > 0 {
		d.retries++
	}
}

func (d *fetchDiagnostics) recordStatus(status int) {
	d.lastHTTPStatus = intPointer(status)
}

func (d *fetchDiagnostics) recordPage(collection string, itemCount int) {
	d.successfulPages++
	if itemCount > 0 {
		d.nonemptyPages++
	}
	if collection == flagCollectionArchived {
		d.archivedFlags += itemCount
	} else {
		d.activeFlags += itemCount
	}
}

func (d *fetchDiagnostics) recordFailure(collection string, offset int) {
	d.failedCollection = stringPointer(collection)
	d.failedOffset = intPointer(offset)
}

func (d *fetchDiagnostics) recordRetryWait(wait time.Duration) {
	d.retryWaits++
	d.scheduledWait += wait
}

func (d *fetchDiagnostics) recordProactiveWait(wait time.Duration) {
	d.proactiveWaits++
	d.scheduledWait += wait
}

func intPointer(value int) *int {
	return &value
}

func stringPointer(value string) *string {
	return &value
}
