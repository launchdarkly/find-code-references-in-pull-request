package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/antihax/optional"
	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go"
	lc "github.com/launchdarkly/cr-flags/client"
	ghc "github.com/launchdarkly/cr-flags/comments"
	ldiff "github.com/launchdarkly/cr-flags/diff"
	"github.com/launchdarkly/ld-find-code-refs/coderefs"
	"github.com/launchdarkly/ld-find-code-refs/options"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

type config struct {
	ldProject     string
	ldEnvironment string
	ldInstance    string
	owner         string
	repo          []string
	apiToken      string
}

func main() {
	config := validateInput()
	event, err := parseEvent(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		fmt.Printf("error parsing GitHub event payload at %q: %v", os.Getenv("GITHUB_EVENT_PATH"), err)
	}

	// Query for flags
	flags, err := getFlags(config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if len(flags.Items) == 0 {
		fmt.Println("No flags found.")
		os.Exit(0)
	}

	flagKeys := make([]string, 0, len(flags.Items))
	for _, flag := range append(flags.Items) {
		flagKeys = append(flagKeys, flag.Key)
	}

	workspace := os.Getenv("GITHUB_WORKSPACE")
	viper.Set("dir", workspace)
	viper.Set("accessToken", config.apiToken)

	err = options.InitYAML()
	opts, err := options.GetOptions()
	if err != nil {
		fmt.Println(err)
	}

	aliases, err := coderefs.GenerateAliases(flagKeys, opts.Aliases, workspace)
	if err != nil {
		fmt.Println(err)
		fmt.Println("failed to create flag key aliases")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	prService := client.PullRequests
	issuesService := client.Issues

	rawOpts := github.RawOptions{Type: github.Diff}
	raw, _, err := prService.GetRaw(ctx, config.owner, config.repo[1], *event.PullRequest.Number, rawOpts)
	multiFiles, err := diff.ParseMultiFileDiff([]byte(raw))

	flagsRef := ghc.FlagsRef{
		FlagsAdded:   make(map[string][]string),
		FlagsRemoved: make(map[string][]string),
	}

	for _, parsedDiff := range multiFiles {
		getPath := ldiff.CheckDiff(parsedDiff, workspace)
		if getPath.Skip {
			continue
		}
		for _, raw := range parsedDiff.Hunks {
			ldiff.ProcessDiffs(raw, flagsRef, flags, aliases)
		}

	}
	if err != nil {
		fmt.Println(err)
	}

	var existingComment int64
	var existingCommentBody string
	existingComment, existingCommentBody = checkExistingComments(event, config, issuesService, ctx)

	addedKeys := make([]string, 0, len(flagsRef.FlagsAdded))
	for key := range flagsRef.FlagsAdded {
		addedKeys = append(addedKeys, key)
	}
	// sort keys so hashing can work for checking if comment already exists
	sort.Strings(addedKeys)
	buildComment := ghc.FlagComments{}
	for _, flagKey := range addedKeys {
		// If flag is in both added and removed then it is being modified
		delete(flagsRef.FlagsRemoved, flagKey)
		aliases := flagsRef.FlagsAdded[flagKey]

		flagAliases := aliases[:0]
		for _, alias := range aliases {
			if !(len(strings.TrimSpace(alias)) == 0) {
				flagAliases = append(flagAliases, alias)
			}
		}
		idx, _ := find(flags.Items, flagKey)
		createComment, err := ghc.GithubFlagComment(flags.Items[idx], flagAliases, config.ldEnvironment, config.ldInstance)
		buildComment.CommentsAdded = append(buildComment.CommentsAdded, createComment)
		if err != nil {
			fmt.Println(err)
		}
	}
	removedKeys := make([]string, 0, len(flagsRef.FlagsRemoved))
	for key := range flagsRef.FlagsRemoved {
		removedKeys = append(removedKeys, key)
	}
	sort.Strings(removedKeys)
	for _, flagKey := range removedKeys {
		aliases := flagsRef.FlagsRemoved[flagKey]
		flagAliases := aliases[:0]
		for _, alias := range aliases {
			if !(len(strings.TrimSpace(alias)) == 0) {
				flagAliases = append(flagAliases, alias)
			}
		}
		idx, _ := find(flags.Items, flagKey)
		removedComment, err := ghc.GithubFlagComment(flags.Items[idx], flagAliases, config.ldEnvironment, config.ldInstance)
		buildComment.CommentsRemoved = append(buildComment.CommentsRemoved, removedComment)
		if err != nil {
			fmt.Println(err)
		}
	}
	postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingCommentBody)
	if postedComments == "" {
		return
	}
	comment := github.IssueComment{
		Body: &postedComments,
	}

	if !(len(flagsRef.FlagsAdded) == 0 && len(flagsRef.FlagsRemoved) == 0) {
		if existingComment > 0 {
			_, _, err = issuesService.EditComment(ctx, config.owner, config.repo[1], existingComment, &comment)
		} else {
			_, _, err = issuesService.CreateComment(ctx, config.owner, config.repo[1], *event.PullRequest.Number, &comment)
		}
		if err != nil {
			fmt.Println(err)
		}
	} else if len(flagsRef.FlagsAdded) == 0 && len(flagsRef.FlagsRemoved) == 0 && os.Getenv("PLACEHOLDER_COMMENT") == "true" {
		// Check if this is already the body, flags could have originally been included then removed in later commit
		if strings.Contains(existingCommentBody, "No flag references found in PR") {
			return
		}
		createComment := ghc.GithubNoFlagComment()
		_, _, err = issuesService.CreateComment(ctx, config.owner, config.repo[1], *event.PullRequest.Number, createComment)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Println("No flags found.")
	}
}

func validateInput() *config {
	var config config
	config.ldProject = os.Getenv("INPUT_PROJKEY")
	if config.ldProject == "" {
		fmt.Println("`project` is required.")
		os.Exit(1)
	}
	config.ldEnvironment = os.Getenv("INPUT_ENVKEY")
	if config.ldEnvironment == "" {
		fmt.Println("`environment` is required.")
		os.Exit(1)
	}
	config.ldInstance = os.Getenv("INPUT_BASEURI")
	if config.ldInstance == "" {
		fmt.Println("`baseUri` is required.")
		os.Exit(1)
	}
	config.owner = os.Getenv("GITHUB_REPOSITORY_OWNER")
	config.repo = strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")

	config.apiToken = os.Getenv("INPUT_ACCESSTOKEN")
	if config.apiToken == "" {
		fmt.Println("`accessToken` is required.")
		os.Exit(1)
	}

	return &config
}

func getFlags(config *config) (ldapi.FeatureFlags, error) {
	ldClient, err := lc.NewClient(config.apiToken, config.ldInstance, false)
	if err != nil {
		fmt.Println(err)
	}
	flagOpts := ldapi.FeatureFlagsApiGetFeatureFlagsOpts{
		Env:     optional.NewInterface(config.ldEnvironment),
		Summary: optional.NewBool(false),
	}
	flags, _, err := ldClient.Ld.FeatureFlagsApi.GetFeatureFlags(ldClient.Ctx, config.ldProject, &flagOpts)
	if err != nil {
		return ldapi.FeatureFlags{}, err
	}

	return flags, nil
}

func checkExistingComments(event *github.PullRequestEvent, config *config, issuesService *github.IssuesService, ctx context.Context) (int64, string) {
	comments, _, err := issuesService.ListComments(ctx, config.owner, config.repo[1], *event.PullRequest.Number, nil)
	if err != nil {
		fmt.Println(err)
	}

	for _, comment := range comments {
		if strings.Contains(*comment.Body, "LaunchDarkly Flag Details") {
			return int64(comment.GetID()), *comment.Body
		}
	}

	return int64(0), ""
}
