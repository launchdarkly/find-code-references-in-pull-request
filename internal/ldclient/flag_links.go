package ldapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v13"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"

	flags "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
)

func CreateFlagLinks(config *lcr.Config, flagsRef flags.ReferenceSummary, event *github.PullRequestEvent) error {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return nil
	}

	for key := range flagsRef.FlagsAdded {
		link := makeFlagLinkRep(event, key, "added")
		sendFlagRequest(config, *link, key)
	}

	for key := range flagsRef.FlagsRemoved {
		message := "removed"
		if flagsRef.IsExtinct(key) {
			message = "extinct"
		}
		link := makeFlagLinkRep(event, key, message)
		sendFlagRequest(config, *link, key)
	}

	return nil
}

// TODO handle errs etc.
func sendFlagRequest(config *lcr.Config, link ldapi.FlagLinkPost, flagKey string) {
	requestBody, err := json.Marshal(link)
	if err != nil {
		log.Println("Unable to construct flag link payload")
		return
	}

	url := fmt.Sprintf("%s/api/v2/flag-links/projects/%s/flags/%s", config.LdInstance, config.LdProject, flagKey)

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
		log.Println("Errored when sending flag link request")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		// TODO update duplicate links? maybe just title and status
		gha.Debug(url)
		gha.Debug("Flag link already exists [url=%s]", *link.DeepLink)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Could not parse flag link request")
	}

	log.Println(string(body))
}

func makeFlagLinkRep(event *github.PullRequestEvent, flagKey, change string) *ldapi.FlagLinkPost {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return nil
	}

	// TODO update metadata info https://github.com/launchdarkly/integration-framework/blob/main/integrations/slack-app/manifest.json
	metadata := make(map[string]string, 6)

	// maybe rename
	metadata["contextMessage"] = change

	if pr.Number != nil {
		metadata["prNumber"] = strconv.Itoa(*pr.Number)
	}

	if pr.State != nil {
		metadata["state"] = *pr.State
	}

	if pr.User.AvatarURL != nil {
		metadata["avatarUrl"] = *pr.User.AvatarURL
	}

	if pr.User.Name != nil {
		metadata["authorName"] = *pr.User.Name
	}

	if pr.User.Login != nil {
		metadata["authorLogin"] = *pr.User.Login
	}

	var timestamp *int64
	if pr.CreatedAt != nil {
		m := pr.CreatedAt.UnixMilli()
		timestamp = &m
	}

	// TODO integration := "github"
	id := strconv.FormatInt(*pr.ID, 10)
	// key must be unique
	key := fmt.Sprintf("github-pr-%s-%s", id, flagKey)

	return &ldapi.FlagLinkPost{
		DeepLink: pr.HTMLURL,
		Key:      &key,
		// IntegrationKey: &integration,
		Timestamp:   timestamp,
		Title:       getPrTitle(event),
		Description: pr.Body,
		Metadata:    &metadata,
	}
}

func getPrTitle(event *github.PullRequestEvent) *string {
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
