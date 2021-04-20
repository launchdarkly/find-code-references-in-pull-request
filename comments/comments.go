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

	"github.com/Masterminds/sprig/v3"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go"
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
	defer func() { recover() }()
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
	tmplSetup := `
**[{{.Flag.Name}}]({{.LDInstance}}{{.Primary.Site.Href}})** ` + "`" + `{{.Flag.Key}}` + "`" + `
{{- if .Flag.Description}}
*{{trim .Flag.Description}}*
{{- end}}
{{- if .Flag.Tags}}
Tags: {{ range $i, $e := .Flag.Tags }}` + "{{if $i}}, {{end}}`" + `{{$e}}` + "`" + `{{end}}
{{ end}}
Kind: **{{ .Flag.Kind }}**
Temporary: **{{ .Flag.Temporary }}**
{{- if .Aliases }}
{{- if ne (len .Aliases) 0}}
Aliases: {{range $i, $e := .Aliases }}` + "{{if $i}}, {{end}}`" + `{{$e}}` + "`" + `{{end}}
{{- end}}
{{- end}}
{{ "\n" }}
{{- range $key, $env := .Environments }}
Environment: **{{ $key }}**
| Type | Variation | Weight(if Rollout) |
| --- | --- | --- |
{{- if not (isNil .Fallthrough_.Rollout) }}
{{- if not (isNil .Fallthrough_.Rollout.Variations)}}
| Default | Rollout | |
{{- range .Fallthrough_.Rollout.Variations }}
| |` + "`" + `{{  (index $.Flag.Variations .Variation).Value }}` + "` | `" + `{{  divf .Weight 1000 }}%` + "`|" + `
{{- end }}
{{- end }}
{{- else }}
| Default | ` + "`" + `{{ trunc 20 (toRawJson (index $.Flag.Variations .Fallthrough_.Variation).Value) }}` + "`| |" + `
{{- end }}
{{- if kindIs "int32" .OffVariation }}
| Off | ` + "`" + `{{(index $.Flag.Variations .OffVariation).Value}}` + "` | |" + `
{{- else }}
Off variation: No off variation set.
{{- end }}
{{ end }}
`
	tmpl := template.Must(template.New("comment").Funcs(template.FuncMap{"trim": strings.TrimSpace, "isNil": isNil}).Funcs(sprig.FuncMap()).Parse(tmplSetup))
	err := tmpl.Execute(&commentBody, commentTemplate)
	if err != nil {
		return "", err
	}
	return html.UnescapeString(commentBody.String()), nil
}

func GithubNoFlagComment() *github.IssueComment {
	commentStr := `LaunchDarkly Flag Details:
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

func BuildFlagComment(buildComment FlagComments, flagsRef FlagsRef, existingComment *github.IssueComment) string {
	var commentStr []string
	commentStr = append(commentStr, "LaunchDarkly Flag Details:")
	if len(flagsRef.FlagsAdded) > 0 {
		commentStr = append(commentStr, "** **Added/Modified** **")
		commentStr = append(commentStr, buildComment.CommentsAdded...)
	}
	if len(flagsRef.FlagsRemoved) > 0 {
		// Add in divider if there are both removed flags and already added/modified flags
		if len(buildComment.CommentsAdded) > 0 {
			commentStr = append(commentStr, "---")
		}
		commentStr = append(commentStr, "** **Removed** **")
		commentStr = append(commentStr, buildComment.CommentsRemoved...)
	}
	postedComments := strings.Join(commentStr, "\n")

	hash := md5.Sum([]byte(postedComments))
	if existingComment != nil && strings.Contains(*existingComment.Body, hex.EncodeToString(hash[:])) {
		fmt.Println("comment already exists")
		return ""
	}
	postedComments = postedComments + "\n comment hash: " + hex.EncodeToString(hash[:])

	return postedComments
}

func ProcessFlags(flagsRef FlagsRef, flags ldapi.FeatureFlags, config *lcr.Config) FlagComments {
	addedKeys := make([]string, 0, len(flagsRef.FlagsAdded))
	for key := range flagsRef.FlagsAdded {
		addedKeys = append(addedKeys, key)
	}
	// sort keys so hashing can work for checking if comment already exists
	sort.Strings(addedKeys)
	buildComment := FlagComments{}

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
		createComment, err := githubFlagComment(flags.Items[idx], flagAliases, config)
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
		removedComment, err := githubFlagComment(flags.Items[idx], flagAliases, config)
		buildComment.CommentsRemoved = append(buildComment.CommentsRemoved, removedComment)
		if err != nil {
			fmt.Println(err)
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
