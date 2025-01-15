package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/google/go-github/v68/github"
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
		gha.SetNotice("No flags found in project %s", config.LdProject)
		os.Exit(0)
	}

	opts, err := getOptions(config)
	failExit(err)

	flagKeys := make([]string, 0, len(flags))
	for _, flag := range flags {
		flagKeys = append(flagKeys, flag.Key)
	}

	gha.StartLogGroup("Preprocessing diffs...")
	multiFiles, err := getDiffs(ctx, config, *event.PullRequest.Number)
	failExit(err)

	diffMap := ldiff.PreprocessDiffs(opts.Dir, multiFiles)

	matcher, err := search.GetMatcher(opts, flagKeys, diffMap)
	gha.EndLogGroup()
	failExit(err)

	builder := references.NewReferenceSummaryBuilder(config.MaxFlags, config.CheckExtinctions)
	gha.StartLogGroup("Scanning diff for references...")
	gha.Log("Searching for %d flags", len(flagKeys))
	for _, contents := range diffMap {
		ldiff.ProcessDiffs(matcher, contents, builder)
	}
	gha.EndLogGroup()

	if config.CheckExtinctions {
		if err := extinctions.CheckExtinctions(opts, builder); err != nil {
			gha.SetWarning("Error checking for extinct flags")
			gha.LogError(err)
		}
	}

	gha.Log("Summarizing results")
	flagsRef := builder.Build()

	// Set outputs
	setOutputs(config, flagsRef)

	// Add comment
	gha.StartLogGroup("Processing comment...")
	existingComment := checkExistingComments(event, config, ctx)
	buildComment := ghc.ProcessFlags(flagsRef, flags, config)
	postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	if postedComments != "" {
		comment := github.IssueComment{
			Body: &postedComments,
		}

		err = postGithubComment(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, comment)
	}
	gha.EndLogGroup()

	// Add flag links
	if config.CreateFlagLinks && postedComments != "" {
		// if postedComments is empty, we probably already created the flag links
		gha.StartLogGroup("Adding flag links...")
		ldclient.CreateFlagLinks(config, flagsRef, event)
		gha.EndLogGroup()
	}

	failExit(err)
}

func checkExistingComments(event *github.PullRequestEvent, config *lcr.Config, ctx context.Context) *github.IssueComment {
	comments, _, err := config.GHClient.Issues.ListComments(ctx, config.Owner, config.Repo, *event.PullRequest.Number, nil)
	if err != nil {
		gha.LogError(err)
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

	if flagsRef.AnyFound() {
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
	gha.Debug("Getting pull request diff...")
	rawOpts := github.RawOptions{Type: github.Diff}
	raw, resp, err := config.GHClient.PullRequests.GetRaw(ctx, config.Owner, config.Repo, prNumber, rawOpts)
	if err != nil {
		// TODO use this elsewhere
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, e.UnauthorizedError
		}

		if resp.StatusCode == http.StatusNotAcceptable {
			gha.Debug("PR %d is too large to process - falling back to git command", prNumber)
			raw, err := getPullRequestDiffUsingGitCommand(ctx, config, config.Owner, config.Repo, prNumber)
			if err != nil {
				return nil, err
			}
			multi, err := diff.ParseMultiFileDiff(raw)
			if err != nil {
				return nil, err
			}
			gha.Debug("Got %d diff files", len(multi))

			return multi, nil
		}

		return nil, err
	}

	multi, err := diff.ParseMultiFileDiff([]byte(raw))
	if err != nil {
		return nil, err
	}
	gha.Debug("Got %d diff files", len(multi))

	return multi, nil
}

// Get options from config. Note: dir will be set to workspace
func getOptions(config *lcr.Config) (options.Options, error) {
	// Needed for ld-find-code-refs to work as a library
	viper.Set("dir", config.Workspace)
	viper.Set("accessToken", config.ApiToken)

	if err := options.InitYAML(); err != nil {
		gha.LogError(err)
	}
	return options.GetOptions()
}

func setOutputs(config *lcr.Config, flagsRef references.ReferenceSummary) {
	gha.Debug("Setting outputs...")
	flagsModified := flagsRef.AddedKeys()
	setOutputsForChangedFlags("modified", flagsModified)

	flagsRemoved := flagsRef.RemovedKeys()
	setOutputsForChangedFlags("removed", flagsRemoved)

	if config.CheckExtinctions {
		setOutputsForChangedFlags("extinct", flagsRef.ExtinctKeys())
	}

	allChangedFlags := make([]string, 0, len(flagsModified)+len(flagsRemoved))
	allChangedFlags = append(allChangedFlags, flagsModified...)
	allChangedFlags = append(allChangedFlags, flagsRemoved...)
	sort.Strings(allChangedFlags)
	setOutputsForChangedFlags("changed", allChangedFlags)
}

func setOutputsForChangedFlags(modifier string, changedFlags []string) {
	count := len(changedFlags)
	gha.SetOutput(fmt.Sprintf("any-%s", modifier), fmt.Sprintf("%t", count > 0))
	gha.SetOutput(fmt.Sprintf("%s-flags-count", modifier), fmt.Sprintf("%d", count))

	sort.Strings(changedFlags)
	gha.SetOutput(fmt.Sprintf("%s-flags", modifier), strings.Join(changedFlags, " "))
}

func failExit(err error) {
	if err != nil {
		gha.LogError(err)
		gha.SetError("%s", err.Error())
		os.Exit(1)
	}
}

// getPullRequestDiffUsingGitCommand returns a diff of PullRequest using git command.
func getPullRequestDiffUsingGitCommand(ctx context.Context, config *lcr.Config, owner, repo string, number int) ([]byte, error) {
	pr, _, err := config.GHClient.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}

	head := pr.GetHead()
	headSha := head.GetSHA()

	commitsComparison, _, err := config.GHClient.Repositories.CompareCommits(ctx, owner, repo, headSha, pr.GetBase().GetSHA(), nil)
	if err != nil {
		return nil, err
	}

	mergeBaseSha := commitsComparison.GetMergeBaseCommit().GetSHA()
	for _, sha := range []string{mergeBaseSha, headSha} {
		_, err := exec.Command("git", "fetch", "--depth=1", head.GetRepo().GetHTMLURL(), sha).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to run git fetch: %w", err)
		}
	}

	bytes, err := exec.Command("git", "diff", "--find-renames", mergeBaseSha, headSha).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff: %w", err)
	}

	return bytes, nil
}
