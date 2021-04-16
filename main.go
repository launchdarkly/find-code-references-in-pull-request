package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"github.com/antihax/optional"
	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go"
	lc "github.com/launchdarkly/cr-flags/client"
	ghc "github.com/launchdarkly/cr-flags/comments"
	"github.com/launchdarkly/cr-flags/ignore"
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
		fmt.Println(err)
		os.Exit(1)
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
		getPath := checkDiff(parsedDiff, workspace)
		if getPath.skip {
			continue
		}
		for _, raw := range parsedDiff.Hunks {
			processDiffs(raw, flagsRef, flags, aliases)
		}

	}
	if err != nil {
		fmt.Println(err)
	}

	comments, _, err := issuesService.ListComments(ctx, config.owner, config.repo[1], *event.PullRequest.Number, nil)
	if err != nil {
		fmt.Println(err)
	}
	var existingComment int64
	var existingCommentBody string
	for _, comment := range comments {
		if strings.Contains(*comment.Body, "LaunchDarkly Flag Details") {
			existingComment = int64(comment.GetID())
			existingCommentBody = *comment.Body
		}
	}
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

func remove(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func parseEvent(path string) (*github.PullRequestEvent, error) {
	/* #nosec */
	eventJsonFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	eventJsonBytes, err := ioutil.ReadAll(eventJsonFile)
	if err != nil {
		return nil, err
	}
	var evt github.PullRequestEvent
	err = json.Unmarshal(eventJsonBytes, &evt)
	if err != nil {
		return nil, err
	}
	return &evt, err
}

func find(slice []ldapi.FeatureFlag, val string) (int, bool) {
	for i, item := range slice {
		if item.Key == val {
			return i, true
		}
	}
	return -1, false
}

type diffPaths struct {
	fileToParse string
	skip        bool
}

func checkDiff(parsedDiff *diff.FileDiff, workspace string) *diffPaths {
	diffPaths := diffPaths{}
	allIgnores := ignore.NewIgnore(workspace)

	// If file is being renamed we don't want to check it for flags.
	parsedFileA := strings.SplitN(parsedDiff.OrigName, "/", 2)
	parsedFileB := strings.SplitN(parsedDiff.NewName, "/", 2)
	fullPathToA := workspace + "/" + parsedFileA[1]
	fullPathToB := workspace + "/" + parsedFileB[1]
	info, err := os.Stat(fullPathToB)
	var isDir bool
	var fileToParse string
	// If there is no 'b' parse 'a', means file is deleted.
	if info == nil {
		isDir = false
		diffPaths.fileToParse = fullPathToA
	} else {
		isDir = info.IsDir()
		diffPaths.fileToParse = fullPathToB
	}
	if err != nil {
		fmt.Println(err)
	}
	// Similar to ld-find-code-refs do not match dotfiles, and read in ignore files.
	if strings.HasPrefix(parsedFileB[1], ".") || allIgnores.Match(fileToParse, isDir) {
		diffPaths.skip = true
	}

	// We don't want to run on renaming of files.
	if (parsedFileA[1] != parsedFileB[1]) && (!strings.Contains(parsedFileB[1], "dev/null") && !strings.Contains(parsedFileA[1], "dev/null")) {
		diffPaths.skip = true
	}

	return &diffPaths
}

func processDiffs(raw *diff.Hunk, flagsRef ghc.FlagsRef, flags ldapi.FeatureFlags, aliases map[string][]string) {
	diffRows := strings.Split(string(raw.Body), "\n")
	for _, row := range diffRows {
		if strings.HasPrefix(row, "+") {
			for _, flag := range flags.Items {
				if strings.Contains(row, flag.Key) {
					currentKeys := flagsRef.FlagsAdded[flag.Key]
					currentKeys = append(currentKeys, "")
					flagsRef.FlagsAdded[flag.Key] = currentKeys
				}
				if len(aliases[flag.Key]) > 0 {
					for _, alias := range aliases[flag.Key] {
						if strings.Contains(row, alias) {
							currentKeys := flagsRef.FlagsAdded[flag.Key]
							currentKeys = append(currentKeys, alias)
							flagsRef.FlagsAdded[flag.Key] = currentKeys
						}
					}
				}
			}
		} else if strings.HasPrefix(row, "-") {
			for _, flag := range flags.Items {
				if strings.Contains(row, flag.Key) {
					currentKeys := flagsRef.FlagsRemoved[flag.Key]
					currentKeys = append(currentKeys, "")
					flagsRef.FlagsRemoved[flag.Key] = currentKeys
				}
				if len(aliases[flag.Key]) > 0 {
					for _, alias := range aliases[flag.Key] {
						if strings.Contains(row, alias) {
							currentKeys := flagsRef.FlagsRemoved[flag.Key]
							currentKeys = append(currentKeys, alias)
							flagsRef.FlagsRemoved[flag.Key] = currentKeys
						}
					}
				}
			}
		}
	}
}
