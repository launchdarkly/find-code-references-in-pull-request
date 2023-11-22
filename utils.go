package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/google/go-github/github"
)

func clearJSONRepoOrgField(byteString []byte) []byte {
	// workaround for https://github.com/google/go-github/issues/131
	var o map[string]interface{}
	_ = json.Unmarshal(byteString, &o)
	if o != nil {
		repo := o["repository"]
		if repo != nil {
			if repo, ok := repo.(map[string]interface{}); ok {
				delete(repo, "organization")
			}
		}
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	return b
}

// pulled from ld-find-code-refs github action
func parseEvent(path string) (*github.PullRequestEvent, error) {
	/* #nosec */
	eventJsonFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	eventJsonBytes, err := io.ReadAll(eventJsonFile)
	if err != nil {
		return nil, err
	}
	eventWithoutOrg := clearJSONRepoOrgField(eventJsonBytes)
	var evt github.PullRequestEvent
	err = json.Unmarshal(eventWithoutOrg, &evt)
	if err != nil {
		return nil, err
	}
	return &evt, err
}
