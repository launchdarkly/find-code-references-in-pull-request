package ldapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllFlagsPagination(t *testing.T) {
	tests := []struct {
		name          string
		activePages   []int
		archivedPages []int
	}{
		{name: "empty", activePages: []int{0}},
		{name: "one flag", activePages: []int{1, 0}},
		{name: "exactly one full page", activePages: []int{100, 0}},
		{name: "multiple active pages", activePages: []int{100, 1, 0}},
		{name: "multiple active and archived pages", activePages: []int{100, 1, 0}, archivedPages: []int{100, 2, 0}},
		{name: "empty active collection", activePages: []int{0}, archivedPages: []int{2, 0}},
		{name: "empty archived collection", activePages: []int{1, 0}, archivedPages: []int{0}},
		{name: "short pages", activePages: []int{2, 1, 0}, archivedPages: []int{1, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pages []flagPageResponse
			wantKeys := []string{}
			for collection, sizes := range [][]int{tt.activePages, tt.archivedPages} {
				filter, prefix := "", "active"
				if collection == 1 {
					filter, prefix = "state:archived", "archived"
				}
				keyOffset := 0
				requestOffset := 0
				for _, count := range sizes {
					items := make([]map[string]string, count)
					for i := range items {
						key := fmt.Sprintf("%s-%d", prefix, keyOffset+i)
						items[i] = map[string]string{"key": key}
						wantKeys = append(wantKeys, key)
					}
					body, err := json.Marshal(map[string]any{"items": items})
					require.NoError(t, err)
					pages = append(pages, flagPageResponse{offset: requestOffset, filter: filter, body: string(body)})
					keyOffset += count
					requestOffset += 100
				}
			}
			config := serveFlagPages(t, tt.archivedPages != nil, pages)

			flags, err := GetAllFlags(config)

			require.NoError(t, err)
			gotKeys := make([]string, len(flags))
			for i, flag := range flags {
				gotKeys[i] = flag.Key
			}
			assert.Equal(t, wantKeys, gotKeys)
		})
	}
}

func TestGetAllFlagsPreservesMetadata(t *testing.T) {
	config := serveFlagPages(t, false, []flagPageResponse{
		{body: `{"items":[{"key":"checkout","name":"Checkout","kind":"boolean","tags":["billing"],
			"environments":{"production":{"on":true,"_environmentName":"Production",
			"_site":{"href":"/test-project/production/features/checkout","type":"text/html"}}}}]}`},
		{offset: 100, body: `{"items":[]}`},
	})

	flags, err := GetAllFlags(config)

	require.NoError(t, err)
	require.Len(t, flags, 1)
	assert.Equal(t, ldapi.FeatureFlag{
		Key:  "checkout",
		Name: "Checkout",
		Kind: "boolean",
		Tags: []string{"billing"},
		Environments: map[string]ldapi.FeatureFlagConfig{
			"production": {
				On:              true,
				EnvironmentName: "Production",
				Site: ldapi.Link{
					Href: ldapi.PtrString("/test-project/production/features/checkout"),
					Type: ldapi.PtrString("text/html"),
				},
			},
		},
	}, flags[0])
}

func TestGetFlagsPreservesQueryParameters(t *testing.T) {
	config := serveFlagPages(t, true, []flagPageResponse{
		{filter: "state:archived", body: `{"items":[{"key":"archived"}]}`},
		{offset: 100, filter: "state:archived", body: `{"items":[]}`},
	})
	params := url.Values{
		"env":    {"production"},
		"filter": {"state:archived"},
		"limit":  {"20"},
		"offset": {"42"},
	}
	want := url.Values{
		"env":    {"production"},
		"filter": {"state:archived"},
		"limit":  {"20"},
		"offset": {"42"},
	}

	flags, err := getFlags(config, params)

	require.NoError(t, err)
	assert.Len(t, flags, 1)
	assert.Equal(t, want, params)
}

func TestGetAllFlagsErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{
			name: "unauthorized", status: http.StatusUnauthorized,
			body: `{"message":"unauthorized"}`, wantError: "401",
		},
		{
			name: "server error", status: http.StatusInternalServerError,
			body: `{"message":"unavailable"}`, wantError: "500",
		},
		{
			name: "non-JSON error", status: http.StatusBadGateway,
			body: "bad gateway", wantError: "502. unable to parse response",
		},
		{
			name: "invalid success JSON", status: http.StatusOK,
			body: "not JSON", wantError: "invalid character",
		},
	}
	for _, tt := range tests {
		for _, archived := range []bool{false, true} {
			for _, laterPage := range []bool{false, true} {
				name := fmt.Sprintf("%s/archived=%t/laterPage=%t", tt.name, archived, laterPage)
				t.Run(name, func(t *testing.T) {
					var pages []flagPageResponse
					filter := ""
					if archived {
						pages = append(pages,
							flagPageResponse{body: `{"items":[{"key":"active"}]}`},
							flagPageResponse{offset: 100, body: `{"items":[]}`},
						)
						filter = "state:archived"
					}
					offset := 0
					if laterPage {
						pages = append(pages, flagPageResponse{filter: filter, body: `{"items":[{"key":"first"}]}`})
						offset = 100
					}
					pages = append(pages, flagPageResponse{offset: offset, filter: filter, status: tt.status, body: tt.body})
					config := serveFlagPages(t, archived, pages)

					flags, err := GetAllFlags(config)

					require.ErrorContains(t, err, tt.wantError)
					assert.Empty(t, flags)
				})
			}
		}
	}
}

type flagPageResponse struct {
	offset int
	filter string
	status int
	body   string
}

func serveFlagPages(t *testing.T, includeArchived bool, pages []flagPageResponse) *lcr.Config {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := int(requests.Add(1)) - 1
		if !assert.Less(t, index, len(pages), "unexpected flag-list request") {
			http.Error(w, "unexpected flag-list request", http.StatusInternalServerError)
			return
		}
		page := pages[index]
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/flags/test-project", r.URL.Path)
		assert.Equal(t, "test-api-token", r.Header.Get("Authorization"))
		assert.Equal(t, "20240415", r.Header.Get("LD-API-Version"))
		assert.Equal(t, "find-code-references-pr/"+version.Version, r.Header.Get("User-Agent"))
		query := url.Values{
			"env":    {"production"},
			"limit":  {"100"},
			"offset": {strconv.Itoa(page.offset)},
		}
		if page.filter != "" {
			query.Set("filter", page.filter)
		}
		assert.Equal(t, query, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		status := page.status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, err := w.Write([]byte(page.body))
		assert.NoError(t, err)
	}))
	t.Cleanup(func() {
		server.Close()
		assert.Equal(t, len(pages), int(requests.Load()), "flag-list request count")
	})
	return &lcr.Config{
		LdInstance:           server.URL,
		LdProject:            "test-project",
		LdEnvironment:        "production",
		ApiToken:             "test-api-token",
		IncludeArchivedFlags: includeArchived,
	}
}
