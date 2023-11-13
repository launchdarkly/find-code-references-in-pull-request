package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/google/go-github/github"
	ghc "github.com/launchdarkly/find-code-references-in-pull-request/comments"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	ldiff "github.com/launchdarkly/find-code-references-in-pull-request/diff"
	e "github.com/launchdarkly/find-code-references-in-pull-request/errors"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/extinctions"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	ldclient "github.com/launchdarkly/find-code-references-in-pull-request/internal/ldclient"
	references "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/spf13/viper"
)

func main() {
	ctx := context.Background()
	config, err := lcr.ValidateInputandParse(ctx)
	failExit(err)

	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	event, err := parseEvent(eventPath)
	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("error parsing GitHub event payload at %q", eventPath))
		failExit(err)
	}

	flags, err := ldclient.GetAllFlags(config)
	failExit(err)

	if len(flags) == 0 {
		gha.LogNotice("No flags found in project %s", config.LdProject)
		os.Exit(0)
	}

	opts, err := getOptions(config)
	failExit(err)

	flagKeys := make([]string, 0, len(flags))
	for _, flag := range flags {
		flagKeys = append(flagKeys, flag.Key)
	}

	multiFiles, err := getDiffs(ctx, config, *event.PullRequest.Number)
	failExit(err)

	diffMap := ldiff.PreprocessDiffs(opts.Dir, multiFiles)

	matcher, err := search.GetMatcher(opts, flagKeys, diffMap)
	failExit(err)

	builder := references.NewReferenceSummaryBuilder(config.MaxFlags, config.CheckExtinctions)
	for _, contents := range diffMap {
		ldiff.ProcessDiffs(matcher, contents, builder)
	}

	if config.CheckExtinctions {
		if err := extinctions.CheckExtinctions(opts, builder); err != nil {
			gha.LogWarning("Error checking for extinct flags")
			log.Println(err)
		}
	}
	flagsRef := builder.Build()

	// Add comment
	existingComment := checkExistingComments(event, config, ctx)
	buildComment := ghc.ProcessFlags(flagsRef, flags, config)
	postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	if postedComments != "" {
		comment := github.IssueComment{
			Body: &postedComments,
		}

		err = postGithubComment(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, comment)
	}

	// Set outputs
	setOutputs(flagsRef)

	failExit(err)
}

func checkExistingComments(event *github.PullRequestEvent, config *lcr.Config, ctx context.Context) *github.IssueComment {
	comments, _, err := config.GHClient.Issues.ListComments(ctx, config.Owner, config.Repo, *event.PullRequest.Number, nil)
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

func postGithubComment(ctx context.Context, flagsRef references.ReferenceSummary, config *lcr.Config, existingComment *github.IssueComment, prNumber int, comment github.IssueComment) error {
	var existingCommentId int64
	if existingComment != nil {
		existingCommentId = existingComment.GetID()
	}

	if flagsRef.Found() {
		if existingCommentId > 0 {
			_, _, err := config.GHClient.Issues.EditComment(ctx, config.Owner, config.Repo, existingCommentId, &comment)
			return err
		}

		_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo, prNumber, &comment)
		return err
	}

	// Check if this is already the body, flags could have originally been included then removed in later commit
	if existingCommentId > 0 {
		if config.PlaceholderComment {
			if strings.Contains(*existingComment.Body, "No flag references found in PR") {
				return nil
			}

			_, _, err := config.GHClient.Issues.EditComment(ctx, config.Owner, config.Repo, existingCommentId, ghc.GithubNoFlagComment())
			return err
		}

		_, err := config.GHClient.Issues.DeleteComment(ctx, config.Owner, config.Repo, existingCommentId)
		return err
	}

	if config.PlaceholderComment {
		_, _, err := config.GHClient.Issues.CreateComment(ctx, config.Owner, config.Repo, prNumber, ghc.GithubNoFlagComment())
		return err
	}

	return nil
}

func getDiffs(ctx context.Context, config *lcr.Config, prNumber int) ([]*diff.FileDiff, error) {
	rawOpts := github.RawOptions{Type: github.Diff}
	raw, resp, err := config.GHClient.PullRequests.GetRaw(ctx, config.Owner, config.Repo, prNumber, rawOpts)
	if err != nil {
		// TODO use this elsewhere
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, e.UnauthorizedError
		}

		return nil, err
	}
	return diff.ParseMultiFileDiff([]byte(raw))
}

// Get options from config. Note: dir will be set to workspace
func getOptions(config *lcr.Config) (options.Options, error) {
	// Needed for ld-find-code-refs to work as a library
	viper.Set("dir", config.Workspace)
	viper.Set("accessToken", config.ApiToken)

	err := options.InitYAML()
	if err != nil {
		log.Println(err)
	}
	return options.GetOptions()
}

func setOutputs(flagsRef references.ReferenceSummary) {
	flagsModified := make([]string, 0, len(flagsRef.FlagsAdded))
	for k := range flagsRef.FlagsAdded {
		flagsModified = append(flagsModified, k)
	}
	setOutputsForChangedFlags("modified", flagsModified)

	flagsRemoved := make([]string, 0, len(flagsRef.FlagsRemoved))
	for k := range flagsRef.FlagsRemoved {
		flagsRemoved = append(flagsRemoved, k)
	}
	setOutputsForChangedFlags("removed", flagsModified)

	allChangedFlags := make([]string, 0, len(flagsModified)+len(flagsRemoved))
	allChangedFlags = append(allChangedFlags, flagsModified...)
	allChangedFlags = append(allChangedFlags, flagsRemoved...)
	setOutputsForChangedFlags("changed", allChangedFlags)
}

func setOutputsForChangedFlags(modifier string, changedFlags []string) {
	count := len(changedFlags)
	gha.SetOutputOrLogError(fmt.Sprintf("any-%s", modifier), fmt.Sprintf("%t", count > 0))
	gha.SetOutputOrLogError(fmt.Sprintf("%s-flags-count", modifier), fmt.Sprintf("%d", count))

	sort.Strings(changedFlags)
	gha.SetOutputOrLogError(fmt.Sprintf("%s-flags", modifier), strings.Join(changedFlags, " "))
}

func failExit(err error) {
	if err != nil {
		gha.LogError(err.Error())
		log.Println(err)
		os.Exit(1)
	}
}
