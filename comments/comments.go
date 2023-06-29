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

	sprig "github.com/Masterminds/sprig/v3"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go/v7"
	"github.com/launchdarkly/cr-flags/config"
	lcr "github.com/launchdarkly/cr-flags/config"
)

type Comment struct {
	Flag         ldapi.FeatureFlag
	Aliases      []string
	ChangeType   string
	Primary      ldapi.FeatureFlagConfig
	Environments map[string]ldapi.FeatureFlagConfig
	LDInstance   string
}

func isNil(a interface{}) bool {
	defer func() { recover() }() //nolint:errcheck
	return a == nil || reflect.ValueOf(a).IsNil()
}

func githubFlagComment(flag ldapi.FeatureFlag, aliases []string, config *config.Config) (string, error) {
	commentTemplate := Comment{
		Flag:         flag,
		Aliases:      aliases,
		Primary:      flag.Environments[config.LdEnvironment[0]],
		Environments: flag.Environments,
		LDInstance:   config.LdInstance,
	}
	var commentBody bytes.Buffer
	// All whitespace for template is required to be there or it will not render properly nested.
	tmplSetup := `| [{{.Flag.Name}}]({{.LDInstance}}{{.Primary.Site.Href}}) | ` + "`" + `{{.Flag.Key}}` + "` |" +
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

type FlagsRef struct {
	FlagsAdded   map[string][]string
	FlagsRemoved map[string][]string
}

func (fr FlagsRef) Found() bool {
	return len(fr.FlagsAdded) > 0 || len(fr.FlagsRemoved) > 0
}

func BuildFlagComment(buildComment FlagComments, flagsRef FlagsRef, existingComment *github.IssueComment) string {
	tableHeader := "| Flag name | Key | Aliases |\n| --- | --- | --- |"

	var commentStr []string
	commentStr = append(commentStr, "## LaunchDarkly flag references")
	if len(flagsRef.FlagsAdded) > 0 {
		commentStr = append(commentStr, fmt.Sprintf("### :green_circle: %d flag references added or modified\n", len(flagsRef.FlagsAdded)))
		commentStr = append(commentStr, tableHeader)
		commentStr = append(commentStr, buildComment.CommentsAdded...)
		commentStr = append(commentStr, "\n")
	}
	if len(flagsRef.FlagsRemoved) > 0 {
		commentStr = append(commentStr, fmt.Sprintf("### :red_circle: %d flag references removed\n", len(flagsRef.FlagsRemoved)))
		commentStr = append(commentStr, tableHeader)
		commentStr = append(commentStr, buildComment.CommentsRemoved...)
	}
	postedComments := strings.Join(commentStr, "\n")
	allFlagKeys := mergeKeys(flagsRef.FlagsAdded, flagsRef.FlagsRemoved)
	if len(allFlagKeys) > 0 {
		var flagKeys []string
		for v := range allFlagKeys {
			flagKeys = append(flagKeys, v)
		}
		postedComments = postedComments + fmt.Sprintf("\n <!-- flags:%s -->", strings.Join(flagKeys, ","))
	}

	hash := md5.Sum([]byte(postedComments))
	if existingComment != nil && strings.Contains(*existingComment.Body, hex.EncodeToString(hash[:])) {
		log.Println("comment already exists")
		return ""
	}

	postedComments = postedComments + "\n <!-- comment hash: " + hex.EncodeToString(hash[:]) + " -->"
	return postedComments
}

func ProcessFlags(flagsRef FlagsRef, flags ldapi.FeatureFlags, config *lcr.Config) FlagComments {
	buildComment := FlagComments{}
	addedKeys := make([]string, 0, len(flagsRef.FlagsAdded))
	for key := range flagsRef.FlagsAdded {
		addedKeys = append(addedKeys, key)
	}
	// sort keys so hashing can work for checking if comment already exists
	sort.Strings(addedKeys)
	for _, flagKey := range addedKeys {
		aliases := flagsRef.FlagsAdded[flagKey]

		flagAliases := aliases[:0]
		for _, alias := range aliases {
			if !(len(strings.TrimSpace(alias)) == 0) {
				flagAliases = append(flagAliases, alias)
			}
		}
		idx, _ := find(flags.Items, flagKey)
		createComment, err := githubFlagComment(flags.Items[idx], flagAliases, config)
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
		aliases := flagsRef.FlagsRemoved[flagKey]
		flagAliases := aliases[:0]
		for _, alias := range aliases {
			if !(len(strings.TrimSpace(alias)) == 0) {
				flagAliases = append(flagAliases, alias)
			}
		}
		idx, _ := find(flags.Items, flagKey)
		removedComment, err := githubFlagComment(flags.Items[idx], flagAliases, config)
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

func mergeKeys(a map[string][]string, b map[string][]string) map[string][]string {
	allKeys := a
	for k, v := range b {
		allKeys[k] = v
	}
	return allKeys
}
