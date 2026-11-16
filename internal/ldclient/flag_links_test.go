package ldapi

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPREvent(title, body string) *github.PullRequestEvent {
	return &github.PullRequestEvent{
		Repo: &github.Repository{
			FullName: github.Ptr("launchdarkly/example"),
			HTMLURL:  github.Ptr("https://github.com/launchdarkly/example"),
		},
		PullRequest: &github.PullRequest{
			ID:        github.Ptr(int64(12345)),
			Number:    github.Ptr(42),
			HTMLURL:   github.Ptr("https://github.com/launchdarkly/example/pull/42"),
			Title:     github.Ptr(title),
			Body:      github.Ptr(body),
			State:     github.Ptr("open"),
			CreatedAt: &github.Timestamp{Time: time.Now().Add(-time.Hour)},
			User: &github.User{
				Login:     github.Ptr("octocat"),
				AvatarURL: github.Ptr("https://example.com/avatar.png"),
			},
		},
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		max      int
		suffix   string
		expected string
	}{
		{name: "fits", in: "hello", max: 5, suffix: "...", expected: "hello"},
		{name: "ascii over limit", in: "hello world", max: 8, suffix: "...", expected: "hello..."},
		{name: "no suffix", in: "hello world", max: 5, suffix: "", expected: "hello"},
		{name: "suffix too long is dropped", in: "hello world", max: 3, suffix: "...", expected: "hel"},
		// "é" is 2 bytes; cutting at byte 2 would split it, so back off to byte 1.
		{name: "does not split multi-byte rune", in: "aéb", max: 2, suffix: "", expected: "a"},
		// "😀" is 4 bytes.
		{name: "does not split emoji", in: "a😀b", max: 4, suffix: "", expected: "a"},
		{name: "zero max", in: "hello", max: 0, suffix: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8(tt.in, tt.max, tt.suffix)
			assert.Equal(t, tt.expected, got)
			assert.LessOrEqual(t, len(got), max(tt.max, 0))
			assert.True(t, utf8.ValidString(got))
		})
	}
}

func TestMakeFlagLinkRepDescription(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		truncated bool
	}{
		{name: "empty body", body: ""},
		{name: "short body", body: "A short description"},
		{name: "exactly at limit", body: strings.Repeat("a", flagLinkDescriptionMaxBytes)},
		{name: "ascii over limit", body: strings.Repeat("a", 10000), truncated: true},
		// 3-byte runes push byte length over the limit well before the character count does.
		{name: "multi-byte under char limit but over byte limit", body: strings.Repeat("─", 2000), truncated: true},
		{name: "emoji over limit", body: strings.Repeat("🚀 ", 2000), truncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := makeFlagLinkRep(testPREvent("Title", tt.body), "my-flag", "Flag added")
			require.NotNil(t, link)
			require.NotNil(t, link.Description)
			desc := *link.Description

			assert.LessOrEqual(t, len(desc), flagLinkDescriptionMaxBytes)
			assert.True(t, utf8.ValidString(desc))
			assert.NotEmpty(t, desc)

			if tt.truncated {
				assert.True(t, strings.HasSuffix(desc, descriptionTruncationSuffix))
			} else if tt.body != "" {
				assert.Equal(t, tt.body, desc)
			}

			// Metadata keeps the full body.
			assert.Equal(t, tt.body, (*link.Metadata)["prBody"])
		})
	}
}

func TestGetLinkTitle(t *testing.T) {
	t.Run("short title", func(t *testing.T) {
		title := getLinkTitle(testPREvent("Add flag", ""))
		assert.Equal(t, "[launchdarkly/example] Add flag (#42)", *title)
	})

	t.Run("long title keeps repo and PR number", func(t *testing.T) {
		title := *getLinkTitle(testPREvent(strings.Repeat("é", 300), ""))
		assert.LessOrEqual(t, len(title), flagLinkTitleMaxBytes)
		assert.True(t, utf8.ValidString(title))
		assert.True(t, strings.HasPrefix(title, "[launchdarkly/example] "))
		assert.True(t, strings.HasSuffix(title, titleTruncationSuffix+" (#42)"))
	})
}

func TestAPIErrorMessage(t *testing.T) {
	assert.Equal(t, `"description" must not exceed 3761 characters`,
		apiErrorMessage([]byte(`{"code":"invalid_request","message":"\"description\" must not exceed 3761 characters"}`)))
	assert.Equal(t, "", apiErrorMessage([]byte("not json")))
	assert.Equal(t, "", apiErrorMessage(nil))
}
