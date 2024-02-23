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

func CreateFlagLinks(config *lcr.Config, flagsRef flags.ReferenceSummary, pr *github.PullRequest) error {
	link := MakeFlagLinkRep(pr)
	if link == nil {
		return nil
	}

	for k := range flagsRef.FlagsAdded {
		m := *link.Metadata
		m["contextMessage"] = "added"
		link.Metadata = &m
		sendFlagRequest(config, *link, k)
	}

	for k := range flagsRef.FlagsRemoved {
		m := *link.Metadata
		m["contextMessage"] = "added"
		link.Metadata = &m
		sendFlagRequest(config, *link, k)
	}

	for k := range flagsRef.ExtinctFlags {
		m := *link.Metadata
		m["contextMessage"] = "extinct"
		link.Metadata = &m
		sendFlagRequest(config, *link, k)
	}

	return nil
}

// TODO handle errs etc.
func sendFlagRequest(config *lcr.Config, link ldapi.FlagLinkRep, flagKey string) {
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
		gha.Debug("Flag link already exists [url=%s]", link.DeepLink)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Could not parse flag link request")
	}

	log.Println(string(body))
}

func MakeFlagLinkRep(pr *github.PullRequest) *ldapi.FlagLinkRep {
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

	m := map[string]string{
		"prNumber": strconv.Itoa(prNumber),
		"avatar":   avatar,
		"state":    state,
	}

	integration := "github"
	prIdAsKey := strconv.FormatInt(*pr.ID, 10)

	timestamp := ldapi.NewTimestampRep()
	if pr.CreatedAt != nil {
		timestamp.SetMilliseconds(pr.CreatedAt.UnixMilli())
	}

	prTitle := ""
	if pr.Title != nil {
		prTitle = fmt.Sprintf("%s (#%d)", *pr.Title, prNumber)
	}

	return &ldapi.FlagLinkRep{
		DeepLink:       *pr.HTMLURL,
		Key:            &prIdAsKey,
		IntegrationKey: &integration,
		Timestamp:      *timestamp,
		Title:          &prTitle,
		Description:    pr.Body,
		Metadata:       &m,
	}
}
