package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/antihax/optional"
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
	"golang.org/x/oauth2"
)

func main() {
	config := lcr.ValidateInputandParse()
	event, err := parseEvent(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		fmt.Printf("error parsing GitHub event payload at %q: %v", os.Getenv("GITHUB_EVENT_PATH"), err)
	}

	// Query for flags
	flags, flagKeys, err := getFlags(config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if len(flags.Items) == 0 {
		fmt.Println("No flags found.")
		os.Exit(0)
	}

	// Needed for ld-find-code-refs to work as a library
	viper.Set("dir", config.Workspace)
	viper.Set("accessToken", config.ApiToken)

	err = options.InitYAML()
	opts, err := options.GetOptions()
	if err != nil {
		fmt.Println(err)
	}

	aliases, err := coderefs.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)
	if err != nil {
		fmt.Println(err)
		fmt.Println("failed to create flag key aliases")
	}
	ctx := context.Background()
	client := getGithubClient(ctx)

	rawOpts := github.RawOptions{Type: github.Diff}
	raw, _, err := client.PullRequests.GetRaw(ctx, config.Owner, config.Repo[1], *event.PullRequest.Number, rawOpts)
	multiFiles, err := diff.ParseMultiFileDiff([]byte(raw))

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

	existingComment := checkExistingComments(event, config, client.Issues, ctx)
	buildComment := ghc.ProcessFlags(flagsRef, flags, config)

	postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	if postedComments == "" {
		return
	}
	comment := github.IssueComment{
		Body: &postedComments,
	}

	postGithubComments(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, client.Issues, comment)
}

func getFlags(config *lcr.Config) (ldapi.FeatureFlags, []string, error) {
	ldClient, err := lc.NewClient(config.ApiToken, config.LdInstance, false)
	if err != nil {
		fmt.Println(err)
	}
	flagOpts := ldapi.FeatureFlagsApiGetFeatureFlagsOpts{
		Env:     optional.NewInterface(config.LdEnvironment),
		Summary: optional.NewBool(false),
	}
	flags, _, err := ldClient.Ld.FeatureFlagsApi.GetFeatureFlags(ldClient.Ctx, config.LdProject, &flagOpts)
	if err != nil {
		return ldapi.FeatureFlags{}, []string{}, err
	}

	flagKeys := make([]string, 0, len(flags.Items))
	for _, flag := range append(flags.Items) {
		flagKeys = append(flagKeys, flag.Key)
	}
	return flags, flagKeys, nil
}

func checkExistingComments(event *github.PullRequestEvent, config *lcr.Config, issuesService *github.IssuesService, ctx context.Context) *github.IssueComment {
	comments, _, err := issuesService.ListComments(ctx, config.Owner, config.Repo[1], *event.PullRequest.Number, nil)
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

func postGithubComments(ctx context.Context, flagsRef ghc.FlagsRef, config *lcr.Config, existingComment *github.IssueComment, prNumber int, issuesService *github.IssuesService, comment github.IssueComment) {
	if !(len(flagsRef.FlagsAdded) == 0 && len(flagsRef.FlagsRemoved) == 0) {
		var existingCommentId int64
		if existingComment != nil {
			existingCommentId = int64(existingComment.GetID())
		} else {
			existingCommentId = 0
		}
		if existingCommentId > 0 {
			_, _, err := issuesService.EditComment(ctx, config.Owner, config.Repo[1], existingCommentId, &comment)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			_, _, err := issuesService.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, &comment)
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
		_, _, err := issuesService.CreateComment(ctx, config.Owner, config.Repo[1], prNumber, createComment)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Println("No flags found.")
	}
}

func getGithubClient(ctx context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}
