package comments

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"html/template"
	"reflect"
	"sort"
	"strings"
	"time"

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	refs "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
)

type Comment struct {
	Flag       ldapi.FeatureFlag
	ArchivedAt time.Time
	Added      bool
	Extinct    bool
	Aliases    []string
	ChangeType string
	Primary    ldapi.FeatureFlagConfig
	LDInstance string
}

func isNil(a interface{}) bool {
	defer func() { recover() }() //nolint:errcheck
	return a == nil || reflect.ValueOf(a).IsNil()
}

func githubFlagComment(flag ldapi.FeatureFlag, aliases []string, added, extinct bool, config *lcr.Config) (string, error) {
	commentTemplate := Comment{
		Flag:       flag,
		Added:      added,
		Extinct:    config.CheckExtinctions && extinct,
		Aliases:    aliases,
		Primary:    flag.Environments[config.LdEnvironment],
		LDInstance: config.LdInstance,
	}
	var commentBody bytes.Buffer
	if flag.ArchivedDate != nil {
		commentTemplate.ArchivedAt = time.UnixMilli(*flag.ArchivedDate)
	}
	// All whitespace for template is required to be there or it will not render properly nested.
	tmplSetup := `| [{{.Flag.Name}}]({{.LDInstance}}{{.Primary.Site.Href}}) | ` +
		"`" + `{{.Flag.Key}}` + "` |" +
		`{{- if ne (len .Aliases) 0}}` +
		`{{range $i, $e := .Aliases }}` + `{{if $i}},{{end}}` + " `" + `{{$e}}` + "`" + `{{end}}` +
		`{{- end}} | ` +
		`{{- if eq .Extinct true}} :white_check_mark: all references removed{{- end}} ` +
		`{{- if eq .Flag.Archived true}}{{- if eq .Extinct true}}<br>{{end}}{{- if eq .Added true}} :warning:{{else}} :information_source:{{- end}} archived on {{.ArchivedAt | date "2006-01-02"}}{{- end}} |`

	tmpl := template.Must(template.New("comment").Funcs(template.FuncMap{"trim": strings.TrimSpace, "isNil": isNil}).Funcs(sprig.FuncMap()).Parse(tmplSetup))
	err := tmpl.Execute(&commentBody, commentTemplate)
	if err != nil {
		return "", err
	}
	commentStr := html.UnescapeString(commentBody.String())

	return commentStr, nil
}

func GithubNoFlagComment() *github.IssueComment {
	commentStr := `## LaunchDarkly flag references

 **No flag references found in PR**`
	comment := github.IssueComment{
		Body: &commentStr,
	}
	return &comment
}

type FlagComments struct {
	CommentsAdded   []string
	CommentsRemoved []string
}

func BuildFlagComment(buildComment FlagComments, flagsRef refs.ReferenceSummary, existingComment *github.IssueComment) string {
	tableHeader := "| Name | Key | Aliases found | Info |\n| --- | --- | --- | --- |"

	var commentStr []string
	commentStr = append(commentStr, "## LaunchDarkly flag references")

	numFlagsAdded := len(flagsRef.FlagsAdded)
	if numFlagsAdded > 0 {
		commentStr = append(commentStr, fmt.Sprintf("### :mag: %s added or modified\n", pluralize("flag", numFlagsAdded)))
		commentStr = append(commentStr, tableHeader)
		commentStr = append(commentStr, buildComment.CommentsAdded...)
		commentStr = append(commentStr, "\n")
	}

	numFlagsRemoved := len(flagsRef.FlagsRemoved)
	if numFlagsRemoved > 0 {
		commentStr = append(commentStr, fmt.Sprintf("### :x: %s removed\n", pluralize("flag", numFlagsRemoved)))
		commentStr = append(commentStr, tableHeader)
		commentStr = append(commentStr, buildComment.CommentsRemoved...)
	}
	allFlagKeys := uniqueFlagKeys(flagsRef.FlagsAdded, flagsRef.FlagsRemoved)
	if len(allFlagKeys) > 0 {
		sort.Strings(allFlagKeys)
		commentStr = append(commentStr, fmt.Sprintf(" <!-- flags:%s -->", strings.Join(allFlagKeys, ",")))
	}
	postedComments := strings.Join(commentStr, "\n")

	hash := md5.Sum([]byte(postedComments))
	if existingComment != nil && strings.Contains(*existingComment.Body, hex.EncodeToString(hash[:])) {
		gha.Log("comment already exists")
		return ""
	}

	postedComments = postedComments + "\n <!-- comment hash: " + hex.EncodeToString(hash[:]) + " -->"
	return postedComments
}

func ProcessFlags(flagsRef refs.ReferenceSummary, flags []ldapi.FeatureFlag, config *lcr.Config) FlagComments {
	buildComment := FlagComments{}

	for _, flagKey := range flagsRef.AddedKeys() {
		flagAliases := flagsRef.FlagsAdded[flagKey]
		idx, _ := find(flags, flagKey)
		createComment, err := githubFlagComment(flags[idx], flagAliases, true, false, config)
		if err != nil {
			gha.LogError(err)
		}
		buildComment.CommentsAdded = append(buildComment.CommentsAdded, createComment)
	}

	for _, flagKey := range flagsRef.RemovedKeys() {
		flagAliases := flagsRef.FlagsRemoved[flagKey]
		idx, _ := find(flags, flagKey)
		extinct := flagsRef.IsExtinct(flagKey)
		removedComment, err := githubFlagComment(flags[idx], flagAliases, false, extinct, config)
		if err != nil {
			gha.LogError(err)
		}
		buildComment.CommentsRemoved = append(buildComment.CommentsRemoved, removedComment)
	}

	return buildComment
}

func find(slice []ldapi.FeatureFlag, val string) (int, bool) {
	for i, item := range slice {
		if item.Key == val {
			return i, true
		}
	}
	return -1, false
}

func uniqueFlagKeys(a, b refs.FlagAliasMap) []string {
	maxKeys := len(a) + len(b)
	allKeys := make([]string, 0, maxKeys)
	for k := range a {
		allKeys = append(allKeys, k)
	}

	for k := range b {
		if _, ok := a[k]; !ok {
			allKeys = append(allKeys, k)
		}
	}

	return allKeys
}

func pluralize(str string, strLength int) string {
	tmpl := "%d %s"
	if strLength != 1 {
		tmpl += "s"
	}

	return fmt.Sprintf(tmpl, strLength, str)
}
