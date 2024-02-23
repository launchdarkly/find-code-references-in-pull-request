package ldapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v13"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"

	flags "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
)

func CreateFlagLinks(config *lcr.Config, flagsRef flags.ReferenceSummary, event *github.PullRequestEvent) error {
	link := makeFlagLinkRep(event)
	if link == nil {
		return nil
	}

	for key := range flagsRef.FlagsAdded {
		m := *link.Metadata
		m["contextMessage"] = "added"
		link.SetMetadata(m)
		sendFlagRequest(config, *link, key)
	}

	for key := range flagsRef.FlagsRemoved {
		m := *link.Metadata
		m["contextMessage"] = "removed"
		link.SetMetadata(m)
		sendFlagRequest(config, *link, key)
	}

	for key := range flagsRef.ExtinctFlags {
		m := *link.Metadata
		m["contextMessage"] = "extinct"
		link.SetMetadata(m)
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
		gha.Debug("Flag link already exists [url=%s]", *link.DeepLink)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Could not parse flag link request")
	}

	log.Println(string(body))
}

func makeFlagLinkRep(event *github.PullRequestEvent) *ldapi.FlagLinkPost {
	pr := event.PullRequest
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return nil
	}

	avatar := ""
	if pr.User.AvatarURL != nil {
		avatar = *pr.User.AvatarURL
	}

	state := ""
	if pr.State != nil {
		state = *pr.State
	}

	var prNumber int
	if pr.Number != nil {
		prNumber = *pr.Number
	}

	// TODO update metadata info https://github.com/launchdarkly/integration-framework/blob/main/integrations/slack-app/manifest.json
	m := map[string]string{
		"prNumber": strconv.Itoa(prNumber),
		"avatar":   avatar,
		"state":    state,
	}

	timestamp := pr.CreatedAt.UnixMilli()

	// TODO integration := "github"
	prIdAsKey := strconv.FormatInt(*pr.ID, 10)

	prTitle := ""
	if pr.Title != nil {
		prTitle = fmt.Sprintf("%s (#%d)", *pr.Title, prNumber)
	} else if pr.Number != nil {
		prTitle = fmt.Sprintf("PR #%d", prNumber)
	} else {
		prTitle = fmt.Sprintf("%s pull request", *event.Repo.Name)
	}

	return &ldapi.FlagLinkPost{
		DeepLink: pr.HTMLURL,
		Key:      &prIdAsKey,
		// IntegrationKey: &integration,
		Timestamp:   &timestamp,
		Title:       &prTitle,
		Description: pr.Body,
		Metadata:    &m,
	}
}
