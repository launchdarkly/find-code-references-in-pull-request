package ldapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"
)

const pageSize = 100

type flagPage struct {
	items   []ldapi.FeatureFlag
	headers http.Header
}

func GetAllFlags(ctx context.Context, config *lcr.Config) (flags []ldapi.FeatureFlag, err error) {
	inventoryContext, cancel := context.WithTimeout(ctx, inventoryTimeout)
	defer cancel()

	fetcher := newFlagFetcher(inventoryContext, fetcherOptions{
		log: func(format string, args ...any) {
			gha.Log(format, args...)
		},
	})
	fetcher.diagnostics.emitStart()
	defer func() {
		fetcher.diagnostics.emitSummary(fetcher.clock(), fetcher.outcome(err), err == nil)
	}()

	gha.Debug("Fetching all flags for project")
	params := url.Values{}
	params.Add("env", config.LdEnvironment)
	activeFlags, err := fetcher.getFlags(config, params, flagCollectionActive, config.IncludeArchivedFlags)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}

	flags = make([]ldapi.FeatureFlag, 0, len(activeFlags))
	flags = append(flags, activeFlags...)

	if config.IncludeArchivedFlags {
		params.Add("filter", "state:archived")
		archivedFlags, archivedErr := fetcher.getFlags(config, params, flagCollectionArchived, false)
		if archivedErr != nil {
			return []ldapi.FeatureFlag{}, archivedErr
		}
		flags = append(flags, archivedFlags...)
	}

	gha.Debug("Fetched %d flags", len(flags))
	return flags, nil
}

func (f *flagFetcher) getFlags(
	config *lcr.Config,
	params url.Values,
	collection string,
	nextCollection bool,
) ([]ldapi.FeatureFlag, error) {
	pageParams := make(url.Values, len(params)+2)
	for key, values := range params {
		pageParams[key] = append([]string(nil), values...)
	}
	pageParams.Set("limit", strconv.Itoa(pageSize))

	flags := []ldapi.FeatureFlag{}
	for offset := 0; ; offset += pageSize {
		f.collection = collection
		f.offset = offset
		f.attempt = 0
		f.waitingRetry = false
		f.pendingRetryWait = 0
		if err := f.waitForNextPage(); err != nil {
			f.diagnostics.recordFailure(collection, offset)
			return []ldapi.FeatureFlag{}, err
		}
		pageParams.Set("offset", strconv.Itoa(offset))
		page, err := f.getFlagPage(config, pageParams)
		if err != nil {
			f.diagnostics.recordFailure(collection, offset)
			return []ldapi.FeatureFlag{}, err
		}

		f.diagnostics.recordPage(collection, len(page.items))
		if len(page.items) == 0 {
			if nextCollection {
				f.scheduleProactiveWait(page.headers, collection, offset)
			}
			return flags, nil
		}

		flags = append(flags, page.items...)
		f.scheduleProactiveWait(page.headers, collection, offset)
	}
}

func (f *flagFetcher) getFlagPage(config *lcr.Config, params url.Values) (flagPage, error) {
	if err := f.contextError(); err != nil {
		return flagPage{}, err
	}

	endpoint := fmt.Sprintf("%s/api/v2/flags/%s", config.LdInstance, config.LdProject)
	retryRequest, err := retryablehttp.NewRequestWithContext(f.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return flagPage{}, f.newError(reasonRequest, 0, nil)
	}
	retryRequest.URL.RawQuery = params.Encode()
	retryRequest.Header.Set("Authorization", config.ApiToken)
	retryRequest.Header.Set("LD-API-Version", "20240415")
	retryRequest.Header.Set("User-Agent", fmt.Sprintf("find-code-references-pr/%s", version.Version))

	response, requestErr := f.client.Do(retryRequest)
	if requestErr != nil {
		closeResponseBody(response)
		if existing := new(flagFetchError); errors.As(requestErr, &existing) {
			return flagPage{}, requestErr
		}
		return flagPage{}, f.wrapRequestError(requestErr)
	}
	if response == nil {
		return flagPage{}, f.newError(reasonTransport, 0, nil)
	}

	if response.StatusCode != http.StatusOK {
		status := response.StatusCode
		closeResponseBody(response)
		if status == http.StatusTooManyRequests {
			return flagPage{}, f.newError(reasonRateLimitExhausted, status, nil)
		}
		return flagPage{}, f.newError(reasonHTTP, status, nil)
	}
	if response.Body == nil {
		closeResponseBody(response)
		return flagPage{}, f.newError(reasonDecode, response.StatusCode, nil)
	}

	var flags ldapi.FeatureFlags
	decodeErr := json.NewDecoder(response.Body).Decode(&flags)
	headers := response.Header.Clone()
	closeResponseBody(response)
	if decodeErr != nil {
		return flagPage{}, f.wrapDecodeError(decodeErr)
	}
	return flagPage{items: flags.Items, headers: headers}, nil
}

func (f *flagFetcher) waitForNextPage() error {
	if !f.hasNotBefore {
		return nil
	}
	if f.notBeforeUnfulfillable {
		f.hasNotBefore = false
		f.notBeforeUnfulfillable = false
		return f.newError(reasonRateLimitDeadline, 0, context.DeadlineExceeded)
	}

	now := f.clock()
	wait := f.notBefore.Sub(now)
	if wait <= 0 {
		f.hasNotBefore = false
		f.notBeforeUnfulfillable = false
		return nil
	}
	if deadline, ok := f.ctx.Deadline(); !ok || fitsDeadline(now, &deadline, wait) {
		f.waitingProactive = true
		waitErr := f.wait(f.ctx, wait)
		f.waitingProactive = false
		f.hasNotBefore = false
		f.notBeforeUnfulfillable = false
		if waitErr != nil {
			if contextErr := f.ctx.Err(); contextErr != nil {
				reason := reasonDeadline
				if errors.Is(contextErr, context.Canceled) {
					reason = reasonCanceled
				}
				if errors.Is(contextErr, context.DeadlineExceeded) {
					reason = reasonRateLimitDeadline
				}
				return f.newError(reason, 0, contextErr)
			}
			return f.newError(reasonTransport, 0, waitErr)
		}
		return nil
	}

	f.hasNotBefore = false
	f.notBeforeUnfulfillable = false
	return f.newError(reasonRateLimitDeadline, 0, context.DeadlineExceeded)
}

func (f *flagFetcher) scheduleProactiveWait(headers http.Header, collection string, offset int) {
	deadline, hasDeadline := f.ctx.Deadline()
	decision := proactiveWait(
		f.clock(),
		deadlinePointer(deadline, hasDeadline),
		parseRateLimitHeaders(headers),
		f.jitter,
	)
	if decision.unfulfillable {
		f.notBeforeUnfulfillable = true
		f.hasNotBefore = true
		return
	}
	if decision.delay <= 0 {
		return
	}

	f.notBefore = f.clock().Add(decision.delay)
	f.hasNotBefore = true
	f.diagnostics.recordProactiveWait(decision.delay)
	f.diagnostics.emitWait(collection, offset, 1, decision.reason, decision.delay)
}

func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
