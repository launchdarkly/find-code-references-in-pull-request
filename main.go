package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

const (
	MAX_429_RETRIES = 10
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

	existingComment := checkExistingComments(event, config, ctx)
	// buildComment := ghc.ProcessFlags(flagsRef, flags, config)
	// postedComments := ghc.BuildFlagComment(buildComment, flagsRef, existingComment)
	// if postedComments == "" {
	// 	return
	// }
	// comment := github.IssueComment{
	// 	Body: &postedComments,
	// }

	// postGithubComments(ctx, flagsRef, config, existingComment, *event.PullRequest.Number, comment)

	// All keys are added to flagsRef.Added for simpler looping of custom props
	mergeKeys(flagsRef.FlagsAdded, flagsRef.FlagsRemoved)
	var existingFlagKeys []string
	if existingComment != nil && strings.Contains(*existingComment.Body, "<!-- flags") {
		lines := strings.Split(*existingComment.Body, "\n")
		for _, line := range lines {
			if strings.Contains(line, "<!-- flags:") {
				fmt.Println(line)
				flagLine := strings.SplitN(line, ":", 2)
				fmt.Println(flagLine)
				existingFlagKeys = append(existingFlagKeys, strings.FieldsFunc(flagLine[1], split)...)
				existingFlagKeys = existingFlagKeys[:len(existingFlagKeys)-1]
				fmt.Println(existingFlagKeys)
			}
		}
		customProp := "ldcrc:" + strings.Join(config.Repo, "/")
	FlagRefLoop:
		for k := range flagsRef.FlagsAdded {
			for i, v := range existingFlagKeys {
				if v == k {
					existingFlagKeys = append(existingFlagKeys[:i], existingFlagKeys[i+1:]...)
					break
				}
			}
			for i := range flags.Items {
				if flags.Items[i].Key == k {
					existingProps := flags.Items[i].CustomProperties
					for _, v := range existingProps[customProp].Value {
						if v == strconv.Itoa(*event.PullRequest.Number) {
							fmt.Println("prop exists")
							continue FlagRefLoop
						}
					}
				}
			}
			customProperty := ldapi.CustomProperty{
				Name:  customProp,
				Value: []string{strconv.Itoa(*event.PullRequest.Number)},
			}
			customPatch := make(map[string]ldapi.CustomProperty)
			customPatch[customProp] = customProperty
			patch := ldapi.PatchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("/customProperties/%s", customProp),
				Value: ptr(customPatch),
			}
			ldClient, err := lc.NewClient(config.ApiToken, config.LdInstance, false)
			if err != nil {
				fmt.Println(err)
			}
			patchComment := ldapi.PatchComment{
				Patch:   []ldapi.PatchOperation{patch},
				Comment: "PR Commentor",
			}
			_, _, err = handleRateLimit(func() (interface{}, *http.Response, error) {
				return ldClient.Ld.FeatureFlagsApi.PatchFeatureFlag(ldClient.Ctx, config.LdProject, k, patchComment)
			})
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println("check keys")
			fmt.Println(existingFlagKeys)
			for _, orphanKey := range existingFlagKeys {
				customProperty := ldapi.CustomProperty{
					Name:  customProp,
					Value: []string{strconv.Itoa(*event.PullRequest.Number)},
				}
				customPatch := make(map[string]ldapi.CustomProperty)
				customPatch[customProp] = customProperty
				patch := ldapi.PatchOperation{
					Op:    "remove",
					Path:  fmt.Sprintf("/customProperties/%s", customProp),
					Value: ptr(customPatch),
				}
				ldClient, err := lc.NewClient(config.ApiToken, config.LdInstance, false)
				if err != nil {
					fmt.Println(err)
				}
				patchComment := ldapi.PatchComment{
					Patch:   []ldapi.PatchOperation{patch},
					Comment: "PR Commentor",
				}
				_, _, err = handleRateLimit(func() (interface{}, *http.Response, error) {
					return ldClient.Ld.FeatureFlagsApi.PatchFeatureFlag(ldClient.Ctx, config.LdProject, orphanKey, patchComment)
				})
			}
		}
	}
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

func mergeKeys(a map[string][]string, b map[string][]string) {
	for k, v := range b {
		a[k] = v
	}
}

func handleRateLimit(apiCall func() (interface{}, *http.Response, error)) (interface{}, *http.Response, error) {
	obj, res, err := apiCall()
	for retryCount := 0; res != nil && res.StatusCode == http.StatusTooManyRequests && retryCount < MAX_429_RETRIES; retryCount++ {
		log.Println("[DEBUG] received a 429 Too Many Requests error. retrying")
		resetStr := res.Header.Get("X-RateLimit-Reset")
		resetInt, parseErr := strconv.ParseInt(resetStr, 10, 64)
		if parseErr != nil {
			log.Println("[DEBUG] could not parse X-RateLimit-Reset header. Sleeping for a random interval.")
			randomRetrySleep()
		} else {
			resetTime := time.Unix(0, resetInt*int64(time.Millisecond))
			sleepDuration := time.Until(resetTime)

			// We have observed situations where LD-s retry header results in a negative sleep duration. In this case,
			// multiply the duration by -1 and add a random 200-500ms
			if sleepDuration <= 0 {
				log.Printf("[DEBUG] received a negative rate limit retry duration of %s. Sleeping for an additional 200-500ms", sleepDuration)
				sleepDuration = -1*sleepDuration + getRandomSleepDuration()
			}
			log.Println("[DEBUG] sleeping", sleepDuration)
			time.Sleep(sleepDuration)
		}
		obj, res, err = apiCall()
	}
	return obj, res, err

}

var randomRetrySleepSeeded = false

// Sleep for a random interval between 200ms and 500ms
func getRandomSleepDuration() time.Duration {
	if !randomRetrySleepSeeded {
		rand.Seed(time.Now().UnixNano())
	}
	n := rand.Intn(300) + 200
	return time.Duration(n) * time.Millisecond
}

func randomRetrySleep() {
	time.Sleep(getRandomSleepDuration())
}

func split(r rune) bool {
	return r == ',' || r == ' '
}
