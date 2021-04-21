package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go"
	lc "github.com/launchdarkly/cr-flags/client"
	ghc "github.com/launchdarkly/cr-flags/comments"
	lcr "github.com/launchdarkly/cr-flags/config"
	ldiff "github.com/launchdarkly/cr-flags/diff"
	"github.com/launchdarkly/ld-find-code-refs/coderefs"
	"github.com/launchdarkly/ld-find-code-refs/options"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/spf13/viper"
)

func main() {
	ctx := context.Background()
	config, err := lcr.ValidateInputandParse(ctx)
	failExit(err)

	event, err := parseEvent(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		fmt.Printf("error parsing GitHub event payload at %q: %v", os.Getenv("GITHUB_EVENT_PATH"), err)
		os.Exit(1)
	}

	// Query for flags
	flags, flagKeys, err := getFlags(config)
	failExit(err)

	if len(flags.Items) == 0 {
		fmt.Println("No flags found.")
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
			ldiff.ProcessDiffs(raw, flagsRef, flags, aliases)
		}
	}
	if err != nil {
		fmt.Println(err)
	}

	// existingComment := checkExistingComments(event, config, ctx)
	// buildComment := ghc.ProcessFlags(flagsRef, flags, config)
	// postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	// if postedComments == "" {
	// 	return
	// }
	//comment := github.IssueComment{
	// 	Body: &postedComments,
	// }

	//postGithubComments(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, comment)

	customProp := strings.Join(config.Repo, "/")
	fmt.Println("patching")
	ldClient, err := lc.NewClient(config.ApiToken, config.LdInstance, false)
	customProperty := ldapi.CustomProperty{
		Name:  customProp,
		Value: []string{strconv.Itoa(*event.PullRequest.Number)},
	}
	customPatch := make(map[string]ldapi.CustomProperty)
	customPatch[customProp] = customProperty
	patch := ldapi.PatchOperation{
		Op:    "add",
		Path:  "/customProperties",
		Value: ptr(customPatch),
	}
	patchComment := ldapi.PatchComment{
		Patch:   []ldapi.PatchOperation{patch},
		Comment: "PR Commentor",
	}
	updatedFlag, resp, err := ldClient.Ld.FeatureFlagsApi.PatchFeatureFlag(ldClient.Ctx, config.LdProject, "chatbox", patchComment)
	fmt.Println(updatedFlag)
	fmt.Println(resp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("patched")
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
	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)

	req.Header.Add("Authorization", config.ApiToken)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}
	resp, err := client.Do(req)
	defer resp.Body.Close()
	// flagOpts := ldapi.FeatureFlagsApiGetFeatureFlagsOpts{
	// 	Env:     optional.NewInterface(os.Getenv("INPUT_ENVKEY")),
	// 	Summary: optional.NewBool(false),
	// }
	//flags, _, err := ldClient.Ld.FeatureFlagsApi.GetFeatureFlags(ldClient.Ctx, config.LdProject, &flagOpts)
	flags := ldapi.FeatureFlags{}
	err = json.NewDecoder(resp.Body).Decode(&flags)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}

	flagKeys := make([]string, 0, len(flags.Items))
	for _, flag := range append(flags.Items) {
		flagKeys = append(flagKeys, flag.Key)
	}
	return flags, flagKeys, nil
}

func checkExistingComments(event *github.PullRequestEvent, config *lcr.Config, ctx context.Context) *github.IssueComment {
	comments, _, err := config.GHClient.Issues.ListComments(ctx, config.Owner, config.Repo[1], *event.PullRequest.Number, nil)
	if err != nil {
		fmt.Println(err)
	}

	for _, comment := range comments {
		if strings.Contains(*comment.Body, "LaunchDarkly Flag Details") {
			return comment
		}
	}

	return nil
}

func postGithubComments(ctx context.Context, flagsRef ghc.FlagsRef, config *lcr.Config, existingComment *github.IssueComment, prNumber int, comment github.IssueComment) {
	if !(len(flagsRef.FlagsAdded) == 0 && len(flagsRef.FlagsRemoved) == 0) {
		var existingCommentId int64
		if existingComment != nil {
			existingCommentId = existingComment.GetID()
		} else {
			existingCommentId = 0
		}
		if existingCommentId > 0 {
			_, _, err := config.GHClient.Issues.EditComment(ctx, config.Owner, config.Repo[1], existingCommentId, &comment)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, &comment)
			if err != nil {
				fmt.Println(err)
			}
		}

	} else if len(flagsRef.FlagsAdded) == 0 && len(flagsRef.FlagsRemoved) == 0 && os.Getenv("PLACEHOLDER_COMMENT") == "true" {
		// Check if this is already the body, flags could have originally been included then removed in later commit
		if existingComment != nil && strings.Contains(*existingComment.Body, "No flag references found in PR") {
			return
		}
		createComment := ghc.GithubNoFlagComment()
		_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, createComment)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Println("No flags found.")
	}
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
		fmt.Println(err)
	}
	opts, err := options.GetOptions()
	if err != nil {
		fmt.Println(err)
	}

	return coderefs.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)

}

func failExit(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func ptr(v interface{}) *interface{} { return &v }
