package flaglink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/google/go-github/github"
	lcr "github.com/launchdarkly/cr-flags/config"
)

type flagLinkMetadata struct{
	Number string `json:"number"`
	Avatar *string `json:"avatar"`
	State *string `json:"state"`
	ContextMessage string `json:"contextMessage"`
}

type flagLink struct {
	DeepLink       string `json:"deepLink"`
	Key            string `json:"key"`
	Timestamp            string `json:"timestamp"`
	IntegrationKey string `json:"integrationKey"`
	Title          *string `json:"title"`
	Metadata       *flagLinkMetadata `json:"metadata"`
}

func CreateFlagLinks(added map[string][]string, removed map[string][]string, pr *github.PullRequest, config *lcr.Config) {
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return
	}

	link := flagLink{
		DeepLink: *pr.HTMLURL,
		Key: strconv.FormatInt(*pr.ID, 10),
		IntegrationKey: "github",
		Timestamp: strconv.FormatInt(pr.CreatedAt.UnixMilli(), 10),
		Title: pr.Title,
		Metadata: &flagLinkMetadata{
			ContextMessage: "",
			Number: strconv.Itoa(*pr.Number),
			Avatar: pr.User.AvatarURL,
			State: pr.State,
		},
	}

	for k := range added {
		link.Metadata.ContextMessage = "added"
		sendFlagRequest(link, k, config.LdInstance, config.LdProject, config.ApiToken)
	}

	for k := range removed {
		link.Metadata.ContextMessage = "removed"
		sendFlagRequest(link, k, config.LdInstance, config.LdProject, config.ApiToken)
	}

	log.Printf("added ****** %+v", added)
	log.Printf("removed ****** %+v", removed)
	log.Printf("link ****** %+v", link)
	log.Printf("link ****** %+v", pr)
}

func sendFlagRequest(link flagLink, flagKey, ldHost, projKey, token string) {
	requestBody, err := json.Marshal(link)
	if err != nil {
		log.Println("Unable to construct flag link payload")
		return
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v2/flag-links/projects/%s/flags/%s", ldHost, projKey, flagKey), bytes.NewBuffer(requestBody))
	if err != nil {
		log.Println("Could not to create flag link request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LD-API-Version", "beta")
	req.Header.Set("Authorization", token)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Errored when sending flag link request")
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Could not parse flag link request")
	}

	log.Println(string(body))
}