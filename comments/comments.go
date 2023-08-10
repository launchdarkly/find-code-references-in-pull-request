package comments

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html"
	"html/template"
	"log"
	"reflect"
	"sort"
	"strings"
	"time"

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v13"
	"github.com/launchdarkly/cr-flags/config"
	lcr "github.com/launchdarkly/cr-flags/config"
	lflags "github.com/launchdarkly/cr-flags/flags"
)

type Comment struct {
	Flag       ldapi.FeatureFlag
	ArchivedAt time.Time
	Added      bool
	Aliases    []string
	ChangeType string
	Primary    ldapi.FeatureFlagConfig
	LDInstance string
}

func isNil(a interface{}) bool {
	defer func() { recover() }() //nolint:errcheck
	return a == nil || reflect.ValueOf(a).IsNil()
}

func githubFlagComment(flag ldapi.FeatureFlag, aliases []string, added bool, config *config.Config) (string, error) {
	commentTemplate := Comment{
		Flag:       flag,
		Added:      added,
		Aliases:    aliases,
		Primary:    flag.Environments[config.LdEnvironment],
		LDInstance: config.LdInstance,
	}
	var commentBody bytes.Buffer
	if flag.ArchivedDate != nil {
		commentTemplate.ArchivedAt = time.UnixMilli(*flag.ArchivedDate)
	}
	// All whitespace for template is required to be there or it will not render properly nested.
	tmplSetup := `| {{- if eq .Flag.Archived true}}{{- if eq .Added true}} :warning:{{- end}}{{- end}}` +
		` [{{.Flag.Name}}]({{.LDInstance}}{{.Primary.Site.Href}})` +
		`{{- if eq .Flag.Archived true}}` +
		` (archived on {{.ArchivedAt | date "2006-01-02"}})` +
		`{{- end}} | ` +
		"`" + `{{.Flag.Key}}` + "` |" +
		`{{- if ne (len .Aliases) 0}}` +
		`{{range $i, $e := .Aliases }}` + `{{if $i}},{{end}}` + " `" + `{{$e}}` + "`" + `{{end}}` +
		`{{- end}} |`

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

func BuildFlagComment(buildComment FlagComments, flagsRef lflags.FlagsRef, existingComment *github.IssueComment) string {
	tableHeader := "| Name | Key | Aliases found |\n| --- | --- | --- |"

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
		log.Println("comment already exists")
		return ""
	}

	postedComments = postedComments + "\n <!-- comment hash: " + hex.EncodeToString(hash[:]) + " -->"
	return postedComments
}

func ProcessFlags(flagsRef lflags.FlagsRef, flags []ldapi.FeatureFlag, config *lcr.Config) FlagComments {
	buildComment := FlagComments{}
	addedKeys := make([]string, 0, len(flagsRef.FlagsAdded))
	for key := range flagsRef.FlagsAdded {
		addedKeys = append(addedKeys, key)
	}
	// sort keys so hashing can work for checking if comment already exists
	sort.Strings(addedKeys)
	for _, flagKey := range addedKeys {
		// If flag is in both added and removed then it is being modified
		delete(flagsRef.FlagsRemoved, flagKey)
		flagAliases := uniqueAliases(flagsRef.FlagsAdded[flagKey])
		idx, _ := find(flags, flagKey)
		createComment, err := githubFlagComment(flags[idx], flagAliases, true, config)
		buildComment.CommentsAdded = append(buildComment.CommentsAdded, createComment)
		if err != nil {
			log.Println(err)
		}
	}
	removedKeys := make([]string, 0, len(flagsRef.FlagsRemoved))
	for key := range flagsRef.FlagsRemoved {
		removedKeys = append(removedKeys, key)
	}
	sort.Strings(removedKeys)
	for _, flagKey := range removedKeys {
		flagAliases := uniqueAliases(flagsRef.FlagsRemoved[flagKey])
		idx, _ := find(flags, flagKey)
		removedComment, err := githubFlagComment(flags[idx], flagAliases, false, config)
		buildComment.CommentsRemoved = append(buildComment.CommentsRemoved, removedComment)
		if err != nil {
			log.Println(err)
		}
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

func uniqueFlagKeys(a, b lflags.FlagAliasMap) []string {
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

func uniqueAliases(aliases lflags.AliasSet) []string {
	flagAliases := make([]string, 0, len(aliases))
	for alias := range aliases {
		if len(strings.TrimSpace(alias)) > 0 {
			flagAliases = append(flagAliases, alias)
		}
	}
	return flagAliases
}

func pluralize(str string, strLength int) string {
	tmpl := "%d %s"
	if strLength != 1 {
		tmpl += "s"
	}

	return fmt.Sprintf(tmpl, strLength, str)
}
