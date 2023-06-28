package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v7"
	ghc "github.com/launchdarkly/cr-flags/comments"
	lcr "github.com/launchdarkly/cr-flags/config"
	ldiff "github.com/launchdarkly/cr-flags/diff"
	gha "github.com/launchdarkly/cr-flags/internal/github_actions"
	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/spf13/viper"
)

func main() {
	ctx := context.Background()
	config, err := lcr.ValidateInputandParse(ctx)
	failExit(err)

	event, err := parseEvent(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		log.Printf("error parsing GitHub event payload at %q: %v", os.Getenv("GITHUB_EVENT_PATH"), err)
		os.Exit(1)
	}

	// Query for flags
	flags, flagKeys, err := getFlags(config)
	failExit(err)

	if len(flags.Items) == 0 {
		log.Println("No flags found.")
		os.Exit(0)
	}

	aliases, err := getAliases(config, flagKeys)
	failExit(err)

	multiFiles, err := getDiffs(ctx, config, *event.PullRequest.Number)
	failExit(err)

	flagsRef := ghc.FlagsRef{
		FlagsAdded:   make(map[string][]string),
		FlagsRemoved: make(map[string][]string),
	}

	for _, parsedDiff := range multiFiles {
		getPath := ldiff.CheckDiff(parsedDiff, config.Workspace)
		if getPath.Skip {
			continue
		}
		for _, raw := range parsedDiff.Hunks {
			ldiff.ProcessDiffs(raw, flagsRef, flags, aliases, config.MaxFlags)
		}
	}

	// Set outputs
	setOutputs(flagsRef)

	// Add comment
	existingComment := checkExistingComments(event, config, ctx)
	buildComment := ghc.ProcessFlags(flagsRef, flags, config)
	postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	if postedComments == "" {
		return
	}
	comment := github.IssueComment{
		Body: &postedComments,
	}

	err = postGithubComment(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, comment)
	failExit(err)
}

func getFlags(config *lcr.Config) (ldapi.FeatureFlags, []string, error) {
	var envString string
	for idx, env := range config.LdEnvironment {
		envString = envString + fmt.Sprintf("env=%s", env)
		if idx != (len(config.LdEnvironment) - 1) {
			envString = envString + "&"
		}
	}
	url := config.LdInstance + "/api/v2/flags/" + config.LdProject + "?" + envString + "&summary=0"
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}
	req.Header.Add("Authorization", config.ApiToken)

	resp, err := client.Do(req)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}

	defer resp.Body.Close()

	flags := ldapi.FeatureFlags{}
	err = json.NewDecoder(resp.Body).Decode(&flags)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}

	flagKeys := make([]string, 0, len(flags.Items))
	for _, flag := range flags.Items {
		flagKeys = append(flagKeys, flag.Key)
	}
	return flags, flagKeys, nil
}

func checkExistingComments(event *github.PullRequestEvent, config *lcr.Config, ctx context.Context) *github.IssueComment {
	comments, _, err := config.GHClient.Issues.ListComments(ctx, config.Owner, config.Repo[1], *event.PullRequest.Number, nil)
	if err != nil {
		log.Println(err)
	}

	for _, comment := range comments {
		if strings.Contains(*comment.Body, "LaunchDarkly flag references") {
			return comment
		}
	}

	return nil
}

func postGithubComment(ctx context.Context, flagsRef ghc.FlagsRef, config *lcr.Config, existingComment *github.IssueComment, prNumber int, comment github.IssueComment) error {
	var existingCommentId int64
	if existingComment != nil {
		existingCommentId = existingComment.GetID()
	}

	if flagsRef.Found() {
		if existingCommentId > 0 {
			_, _, err := config.GHClient.Issues.EditComment(ctx, config.Owner, config.Repo[1], existingCommentId, &comment)
			return err
		}

		_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, &comment)
		return err
	}

	// Check if this is already the body, flags could have originally been included then removed in later commit
	if existingCommentId > 0 {
		if strings.Contains(*existingComment.Body, "No flag references found in PR") {
			return nil
		}

		_, _, err := config.GHClient.Issues.EditComment(ctx, config.Owner, config.Repo[1], existingCommentId, ghc.GithubNoFlagComment())
		return err
	}

	_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, ghc.GithubNoFlagComment())
	return err
}

func getDiffs(ctx context.Context, config *lcr.Config, prNumber int) ([]*diff.FileDiff, error) {
	rawOpts := github.RawOptions{Type: github.Diff}
	raw, _, err := config.GHClient.PullRequests.GetRaw(ctx, config.Owner, config.Repo[1], prNumber, rawOpts)
	if err != nil {
		return nil, err
	}
	return diff.ParseMultiFileDiff([]byte(raw))
}

func getAliases(config *lcr.Config, flagKeys []string) (map[string][]string, error) {
	// Needed for ld-find-code-refs to work as a library
	viper.Set("dir", config.Workspace)
	viper.Set("accessToken", config.ApiToken)

	err := options.InitYAML()
	if err != nil {
		log.Println(err)
	}
	opts, err := options.GetOptions()
	if err != nil {
		log.Println(err)
	}

	return aliases.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)

}

func setOutputs(flagsRef ghc.FlagsRef) {
	err := gha.SetOutput("flags_modified", fmt.Sprintf("%d", len(flagsRef.FlagsAdded)))
	if err != nil {
		log.Println("Failed to set outputs.flags_modified")
	}
	err = gha.SetOutput("flags_removed", fmt.Sprintf("%d", len(flagsRef.FlagsRemoved)))
	if err != nil {
		log.Println("Failed to set outputs.flags_removed")
	}
}

func failExit(err error) {
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
