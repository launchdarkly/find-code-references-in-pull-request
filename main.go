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
	ghc "github.com/launchdarkly/cr-flags/comments"
	lcr "github.com/launchdarkly/cr-flags/config"
	ldiff "github.com/launchdarkly/cr-flags/diff"
	e "github.com/launchdarkly/cr-flags/errors"
	lflags "github.com/launchdarkly/cr-flags/flags"
	gha "github.com/launchdarkly/cr-flags/internal/github_actions"
	ldclient "github.com/launchdarkly/cr-flags/internal/ldclient"
	"github.com/launchdarkly/cr-flags/search"
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

	matcher, err := search.GetMatcher(config, opts, flags)
	failExit(err)

	multiFiles, err := getDiffs(ctx, config, *event.PullRequest.Number)
	failExit(err)

	flagsRef := lflags.FlagsRef{
		FlagsAdded:   make(lflags.FlagAliasMap),
		FlagsRemoved: make(lflags.FlagAliasMap),
	}

	for _, parsedDiff := range multiFiles {
		getPath := ldiff.CheckDiff(parsedDiff, config.Workspace)
		if getPath.Skip {
			continue
		}
		for _, hunk := range parsedDiff.Hunks {
			ldiff.ProcessDiffs(matcher, hunk, flagsRef, config.MaxFlags)
		}
	}

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

func postGithubComment(ctx context.Context, flagsRef lflags.FlagsRef, config *lcr.Config, existingComment *github.IssueComment, prNumber int, comment github.IssueComment) error {
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

func setOutputs(flagsRef lflags.FlagsRef) {
	flagsAddedCount := len(flagsRef.FlagsAdded)

	if err := gha.SetOutput("any-modified", fmt.Sprintf("%t", flagsAddedCount > 0)); err != nil {
		log.Println("Failed to set outputs.any-modified")
	}
	if err := gha.SetOutput("modified-flags-count", fmt.Sprintf("%d", flagsAddedCount)); err != nil {
		log.Println("Failed to set outputs.modified-flags-count")
	}
	flagKeysAdded := make([]string, 0, len(flagsRef.FlagsAdded))
	for k := range flagsRef.FlagsAdded {
		flagKeysAdded = append(flagKeysAdded, k)
	}
	sort.Strings(flagKeysAdded)
	if err := gha.SetOutput("modified-flags", strings.Join(flagKeysAdded, " ")); err != nil {
		log.Println("Failed to set outputs.modified-flags")
	}

	flagsRemovedCount := len(flagsRef.FlagsRemoved)

	if err := gha.SetOutput("any-removed", fmt.Sprintf("%t", flagsRemovedCount > 0)); err != nil {
		log.Println("Failed to set outputs.any-removed")
	}
	if err := gha.SetOutput("removed-flags-count", fmt.Sprintf("%d", flagsRemovedCount)); err != nil {
		log.Println("Failed to set outputs.removed-flags-count")
	}

	flagKeysRemoved := make([]string, 0, len(flagsRef.FlagsRemoved))
	for k := range flagsRef.FlagsRemoved {
		flagKeysRemoved = append(flagKeysRemoved, k)
	}
	sort.Strings(flagKeysRemoved)
	if err := gha.SetOutput("removed-flags", strings.Join(flagKeysRemoved, " ")); err != nil {
		log.Println("Failed to set outputs.removed-flags")
	}
}

func failExit(err error) {
	if err != nil {
		gha.LogError(err.Error())
		log.Println(err)
		os.Exit(1)
	}
}
