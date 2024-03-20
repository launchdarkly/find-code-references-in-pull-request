package ldapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"

	flags "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
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
		gha.SetWarning("Failed to create flag link for %s", flagKey)
		gha.Log("[POST %s] Flag link request failed [status=%d]", url, resp.StatusCode)
	}

	gha.Debug("Response:\n\n%s", string(body))
}

func makeFlagLinkRep(event *github.PullRequestEvent, flagKey, message string) *ldapi.FlagLinkPost {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return nil
	}

	metadata := map[string]string{
		"message":   message,
		"prNumber":  strconv.Itoa(*pr.Number),
		"prTitle":   *pr.Title,
		"prBody":    *pr.Body,
		"state":     *pr.State,
		"avatarUrl": *pr.User.AvatarURL,
		"repoName":  *event.Repo.FullName,
		"repoUrl":   *event.Repo.HTMLURL,
	}

	if pr.User.Name != nil {
		metadata["authorName"] = *pr.User.Name
		metadata["authorDisplayName"] = *pr.User.Name
	} else {
		metadata["authorDisplayName"] = *pr.User.Login
		metadata["authorLogin"] = *pr.User.Login
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

	return &ldapi.FlagLinkPost{
		DeepLink:       pr.HTMLURL,
		Key:            &key,
		IntegrationKey: &integration,
		Timestamp:      timestamp,
		Title:          getLinkTitle(event),
		Description:    pr.Body,
		Metadata:       &metadata,
	}
}

func getLinkTitle(event *github.PullRequestEvent) *string {
	builder := new(strings.Builder)
	builder.WriteString(fmt.Sprintf("[%s]", *event.Repo.FullName))

	pr := event.PullRequest
	if pr.Title != nil {
		builder.WriteString(" ")
		builder.WriteString(*pr.Title)
		if pr.Number != nil {
			builder.WriteString(fmt.Sprintf(" (#%d)", *pr.Number))
		}
	} else if pr.Number != nil {
		builder.WriteString(fmt.Sprintf(" PR #%d", *pr.Number))
	} else {
		builder.WriteString(" pull request")
	}

	title := builder.String()

	return &title
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
