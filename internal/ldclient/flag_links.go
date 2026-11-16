package ldapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/go-github/v68/github"

	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"

	flags "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
)

// Limits enforced by the LaunchDarkly flag links API. The API compares byte
// length (Go len), not character count, so multi-byte UTF-8 counts fully.
const (
	flagLinkTitleMaxBytes       = 256
	flagLinkDescriptionMaxBytes = 3761

	descriptionTruncationSuffix = "\n\n…(truncated, see the pull request for the full description)"
	titleTruncationSuffix       = "…"
)

func CreateFlagLinks(config *lcr.Config, flagsRef flags.ReferenceSummary, event *github.PullRequestEvent) {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		gha.Debug("No pull request found in event")
		return
	}

	numAdded := len(flagsRef.FlagsAdded)
	numRemoved := len(flagsRef.FlagsRemoved)

	for key, aliases := range flagsRef.FlagsAdded {
		message := buildLinkMessage(key, aliases, "added", numAdded, numRemoved)
		link := makeFlagLinkRep(event, key, message)
		postFlagLink(config, *link, key)
	}

	for key, aliases := range flagsRef.FlagsRemoved {
		action := "removed"
		if flagsRef.IsExtinct(key) {
			action = "extinct"
		}
		message := buildLinkMessage(key, aliases, action, numAdded, numRemoved)
		link := makeFlagLinkRep(event, key, message)
		postFlagLink(config, *link, key)
	}
}

func postFlagLink(config *lcr.Config, link ldapi.FlagLinkPost, flagKey string) {
	requestBody, err := json.Marshal(link)
	if err != nil {
		gha.SetWarning("Failed to create flag link for %s", flagKey)
		gha.Debug("Unable to construct flag link payload")
		return
	}

	url := fmt.Sprintf("%s/api/v2/flag-links/projects/%s/flags/%s", config.LdInstance, config.LdProject, flagKey)

	gha.Debug("[POST %s]\n\n%s", url, string(requestBody))

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestBody))
	if err != nil {
		gha.SetWarning("Could not to create flag link request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LD-API-Version", "beta")
	req.Header.Set("Authorization", config.ApiToken)
	req.Header.Add("User-Agent", fmt.Sprintf("find-code-references-pr/%s", version.Version))

	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		gha.SetWarning("Failed to create flag link for %s", flagKey)
		gha.Debug("Error when sending flag link request:\n\n%s", err.Error())
		return
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		gha.Debug("Could not parse flag link request")
	}

	switch resp.StatusCode {
	case http.StatusCreated:
		gha.Log("[POST %s] Flag link created [url=%s]", url, *link.DeepLink)
	case http.StatusConflict:
		gha.Log("[POST %s] Flag link already exists [url=%s]", url, *link.DeepLink)
	default:
		if msg := apiErrorMessage(body); msg != "" {
			gha.SetWarning("Failed to create flag link for %s: %s", flagKey, msg)
		} else {
			gha.SetWarning("Failed to create flag link for %s", flagKey)
		}
		gha.Log("[POST %s] Flag link request failed [status=%d]", url, resp.StatusCode)
	}

	gha.Debug("Response:\n\n%s", string(body))
}

func makeFlagLinkRep(event *github.PullRequestEvent, flagKey, message string) *ldapi.FlagLinkPost {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return nil
	}

	authorLogin := utils.SafeString(pr.User.Login)
	metadata := map[string]string{
		"message":   message,
		"prNumber":  strconv.Itoa(*pr.Number),
		"prTitle":   utils.SafeString(pr.Title),
		"prBody":    utils.SafeString(pr.Body),
		"state":     utils.SafeString(pr.State),
		"avatarUrl": utils.SafeString(pr.User.AvatarURL),
		"repoName":  utils.SafeString(event.Repo.FullName),
		"repoUrl":   utils.SafeString(event.Repo.HTMLURL),
	}

	if pr.User.Name != nil {
		metadata["authorName"] = utils.SafeString(pr.User.Name)
		metadata["authorDisplayName"] = utils.SafeString(pr.User.Name)
	} else {
		metadata["authorDisplayName"] = authorLogin
		metadata["authorLogin"] = authorLogin
	}

	var timestamp *int64
	if pr.CreatedAt != nil {
		m := pr.CreatedAt.UnixMilli()
		timestamp = &m
	}

	integration := "github"
	id := strconv.FormatInt(*pr.ID, 10)
	// key must be unique
	key := fmt.Sprintf("github-pr-%s-%s", id, flagKey)

	description := utils.SafeString(pr.Body)
	// Flag links require a description
	if description == "" {
		description = "Empty PR Body"
	}
	description = truncateUTF8(description, flagLinkDescriptionMaxBytes, descriptionTruncationSuffix)
	return &ldapi.FlagLinkPost{
		DeepLink:       pr.HTMLURL,
		Key:            &key,
		IntegrationKey: &integration,
		Timestamp:      timestamp,
		Title:          getLinkTitle(event),
		Description:    &description,
		Metadata:       &metadata,
	}
}

func getLinkTitle(event *github.PullRequestEvent) *string {
	prefix := fmt.Sprintf("[%s]", utils.SafeString(event.Repo.FullName))

	pr := event.PullRequest
	var title string
	if pr.Title != nil {
		suffix := ""
		if pr.Number != nil {
			suffix = fmt.Sprintf(" (#%d)", *pr.Number)
		}
		// Truncate the PR title itself so the repo name and PR number survive.
		prTitle := truncateUTF8(utils.SafeString(pr.Title), flagLinkTitleMaxBytes-len(prefix)-len(" ")-len(suffix), titleTruncationSuffix)
		title = prefix + " " + prTitle + suffix
	} else if pr.Number != nil {
		title = fmt.Sprintf("%s PR #%d", prefix, *pr.Number)
	} else {
		title = prefix + " pull request"
	}

	// Safety net for pathological repo names.
	title = truncateUTF8(title, flagLinkTitleMaxBytes, titleTruncationSuffix)

	return &title
}

// truncateUTF8 returns s unchanged if it fits in maxBytes. Otherwise it cuts s
// on a UTF-8 rune boundary and appends suffix so the result is at most
// maxBytes. If suffix does not fit, it is omitted.
func truncateUTF8(s string, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	if len(suffix) >= maxBytes {
		suffix = ""
	}
	limit := maxBytes - len(suffix)
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit] + suffix
}

// apiErrorMessage extracts the "message" field from a LaunchDarkly API error body.
func apiErrorMessage(body []byte) string {
	var rep struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &rep); err != nil {
		return ""
	}
	return rep.Message
}

func buildLinkMessage(key string, aliases []string, action string, added, removed int) string {
	builder := new(strings.Builder)
	builder.WriteString(fmt.Sprintf("Flag %s", action))
	if len(aliases) > 0 {
		builder.WriteString(fmt.Sprintf(" (aliases: %s)", strings.Join(aliases, ", ")))
	}

	if added > 0 {
		count := added
		if action == "added" {
			count--
		}
		if count > 0 {
			builder.WriteString(fmt.Sprintf("\n\t- Added %d other flags", count))
		}
	}

	if removed > 0 {
		count := removed
		if action == "added" {
			count--
		}
		if count > 0 {
			builder.WriteString(fmt.Sprintf("\n\t- Removed %d other flags", count))
		}
	}

	return builder.String()
}
