package comments

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"html/template"
	"strings"

	"github.com/google/go-github/github"
	ldapi "github.com/launchdarkly/api-client-go"
)

type Comment struct {
	Flag        ldapi.FeatureFlag
	Aliases     []string
	ChangeType  string
	Environment ldapi.FeatureFlagConfig
	LDInstance  string
}

func GithubFlagComment(flag ldapi.FeatureFlag, aliases []string, environment string, instance string) (string, error) {

	commentTemplate := Comment{
		Flag:        flag,
		Aliases:     aliases,
		Environment: flag.Environments[environment],
		LDInstance:  instance,
	}
	var commentBody bytes.Buffer
	tmplSetup := `
**[{{.Flag.Name}}]({{.LDInstance}}{{.Environment.Site.Href}})** ` + "`" + `{{.Flag.Key}}` + "`" + `
{{- if .Flag.Description}}
*{{trim .Flag.Description}}*
{{- end}}
{{- if .Flag.Tags}}
Tags: {{ range $i, $e := .Flag.Tags }}` + "{{if $i}}, {{end}}`" + `{{$e}}` + "`" + `{{end}}
{{- end}}

Default variation: ` + "`" + `{{(index .Flag.Variations .Environment.Fallthrough_.Variation).Value}}` + "`" + `
Off variation: ` + "`" + `{{(index .Flag.Variations .Environment.OffVariation).Value}}` + "`" + `
Kind: **{{ .Flag.Kind }}**
Temporary: **{{ .Flag.Temporary }}**
{{- if .Aliases }}
{{- if ne (len .Aliases) 0}}
Aliases: {{range $i, $e := .Aliases }}` + "{{if $i}}, {{end}}`" + `{{$e}}` + "`" + `{{end}}
{{- end}}
{{- end}}
`
	tmpl := template.Must(template.New("comment").Funcs(template.FuncMap{"trim": strings.TrimSpace}).Parse(tmplSetup))
	err := tmpl.Execute(&commentBody, commentTemplate)
	if err != nil {
		return "", err
	}
	return commentBody.String(), nil
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

func BuildFlagComment(buildComment FlagComments, flagsRef FlagsRef, existingCommentBody string) string {
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
	if strings.Contains(existingCommentBody, hex.EncodeToString(hash[:])) {
		fmt.Println("comment already exists")
		return ""
	}
	postedComments = postedComments + "\n comment hash: " + hex.EncodeToString(hash[:])

	return postedComments
}
