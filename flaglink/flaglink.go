package flaglink

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/google/go-github/github"
	lcr "github.com/launchdarkly/cr-flags/config"
)

type flagLinkMetadata struct{
	Number *int `json:"number"`
	Avatar *string `json:"avatar"`
	State *string `json:"state"`
	ContextMessage string `json:"contextMessage"`
}

type flagLink struct {
	DeepLink       string `json:"deepLink"`
	Key            string `json:"key"`
	IntegrationKey string `json:"integrationKey"`
	Title          *string `json:"title"`
	Metadata       *flagLinkMetadata `json:"metadata"`
}

// data: {
// 	deepLink: data.deepLink,
// 	key: data.id, // unique key
// 	integrationKey: 'trello',
// 	title: data.title,
// 	metadata: {
// 		creator: data.fullName,
// 		cardTitle: data.title,
// 		avatar: data.avatar,
// 	},
// },

func CreateFlagLinks(added map[string][]string, removed map[string][]string, pr *github.PullRequest, config *lcr.Config) {
	if pr == nil || pr.HTMLURL == nil || pr.ID == nil {
		return
	}

	link := flagLink{
		DeepLink: *pr.HTMLURL,
		Key: strconv.FormatInt(*pr.ID, 10),
		IntegrationKey: "github",
		Title: pr.Title,
		Metadata: &flagLinkMetadata{
			ContextMessage: "added flag",
			Number: pr.Number,
			Avatar: pr.User.AvatarURL,
			State: pr.State,
		},
	}

	log.Printf("added ****** %+v", added)
	log.Printf("removed ****** %+v", removed)
	log.Printf("link ****** %+v", link)

	requestBody, err := json.Marshal(link)
	// requestBody, err := json.Marshal(map[string]string{
	// 	"deepLink": *pr.HTMLURL,
	// 	"key": string(*pr.ID),
	// 	"integrationKey": "github",
	// 	"title": strPtr(pr.Title),
	// 	"metadata": json.Marshal([string]string{
	// 		"contextMessage": "added flag",
	// 		"number": strPtr(pr.Number),
	// 		"avatar": strPtr(pr.User.AvatarURL),
	// 		"state": strPtr(pr.State),
	// 	}),
	// })
	if err != nil {
		log.Println("Unable to construct flag link payload")
		return
	}

	req, err := http.NewRequest(http.MethodPost, config.LdInstance, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Println("Could not to create flag link request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LD-API-Version", "beta")
	req.Header.Set("Authorization", config.ApiToken)

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

	
	
	// log.Printf("Title ****** %+v", pr.Title)
	// log.Printf("User.AvatarURL ****** %+v", pr.User.AvatarURL)
	// log.Printf("HTMLURL ****** %+v", pr.HTMLURL)
	// log.Printf("State ****** %+v", pr.State)
	// log.Printf("PR ****** %+v", pr)

	// pr.Number // #15
	// pr.Title //Testing setup
	// pr.User.AvatarURL
	// pr.HTMLURL - "https://github.com/launchdarkly/cr-flags/pull/15"
	// pr.State

}
