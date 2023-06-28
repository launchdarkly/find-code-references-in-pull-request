package comments

import (
	"strings"
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	"github.com/launchdarkly/cr-flags/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(v interface{}) *interface{} { return &v }

type testFlagEnv struct {
	Flag   ldapi.FeatureFlag
	Config config.Config
}

func newTestAccEnv() *testFlagEnv {

	flag := createFlag("example-flag")
	config := config.Config{
		LdEnvironment: []string{"production"},
		LdInstance:    "https://example.com/",
	}
	return &testFlagEnv{
		Flag:   flag,
		Config: config,
	}
}

func createFlag(key string) ldapi.FeatureFlag {
	href := "test"
	environment := ldapi.FeatureFlagConfig{
		EnvironmentName: "Production",
		Site: ldapi.Link{
			Href: &href,
		},
	}
	variationTrue := ldapi.Variation{
		Value: ptr(true),
	}
	variationFalse := ldapi.Variation{
		Value: ptr(false),
	}
	flag := ldapi.FeatureFlag{
		Key:          key,
		Name:         strings.ReplaceAll(key, "-", " "),
		Kind:         "boolean",
		Environments: map[string]ldapi.FeatureFlagConfig{"production": environment},
		Variations:   []ldapi.Variation{variationTrue, variationFalse},
	}
	return flag
}

type testCommentBuilder struct {
	Comments FlagComments
	FlagsRef FlagsRef
}

func newCommentBuilderAccEnv() *testCommentBuilder {
	flagComments := FlagComments{
		CommentsAdded:   []string{},
		CommentsRemoved: []string{},
	}
	flagsAdded := make(map[string][]string)
	flagsRemoved := make(map[string][]string)
	flagsRef := FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	return &testCommentBuilder{
		Comments: flagComments,
		FlagsRef: flagsRef,
	}
}

type testProcessor struct {
	Flags    ldapi.FeatureFlags
	FlagsRef FlagsRef
	Config   config.Config
}

func newProcessFlagAccEnv() *testProcessor {
	flag := createFlag("example-flag")
	flags := ldapi.FeatureFlags{}
	flags.Items = append(flags.Items, flag)
	flagsAdded := make(map[string][]string)
	flagsRemoved := make(map[string][]string)
	flagsRef := FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	config := config.Config{
		LdEnvironment: []string{"production"},
		LdInstance:    "https://example.com/",
	}
	return &testProcessor{
		Flags:    flags,
		FlagsRef: flagsRef,
		Config:   config,
	}
}

func newProcessMultipleFlagsFlagAccEnv() *testProcessor {
	flag := createFlag("example-flag")
	flag2 := createFlag("second-flag")
	flags := ldapi.FeatureFlags{Items: []ldapi.FeatureFlag{flag, flag2}}
	flagsAdded := make(map[string][]string)
	flagsRemoved := make(map[string][]string)
	flagsRef := FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	config := config.Config{
		LdEnvironment: []string{"production"},
		LdInstance:    "https://example.com/",
	}
	return &testProcessor{
		Flags:    flags,
		FlagsRef: flagsRef,
		Config:   config,
	}
}

func TestGithubFlagComment(t *testing.T) {
	acceptanceTestEnv := newTestAccEnv()
	t.Run("Basic flag", acceptanceTestEnv.NoAliases)
	t.Run("Flag with alias", acceptanceTestEnv.Alias)
}

func TestProcessFlags(t *testing.T) {
	processor := newProcessFlagAccEnv()
	t.Run("Basic Test", processor.Basic)

	multiFlagProcessor := newProcessMultipleFlagsFlagAccEnv()
	t.Run("Multiple flags test", multiFlagProcessor.Multi)
}

func TestGithubNoFlagComment(t *testing.T) {
	comment := GithubNoFlagComment()
	assert.Equal(t, "# LaunchDarkly flag references\n\n **No flag references found in PR**", *comment.Body, "they should be equal")
}

func TestBuildFlagComment(t *testing.T) {
	addedAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Added comments only", addedAcceptanceTestEnv.AddedOnly)

	removedAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Removed comments only", removedAcceptanceTestEnv.RemovedOnly)

	bothAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Add and Remove comments", bothAcceptanceTestEnv.AddedAndRemoved)
}

func (e *testFlagEnv) NoAliases(t *testing.T) {
	comment, err := githubFlagComment(e.Flag, []string{}, &e.Config)
	require.NoError(t, err)

	expected := "| [example flag](https://example.com/test) | `example-flag` | |"
	assert.Equal(t, expected, comment)
}

func (e *testFlagEnv) Alias(t *testing.T) {
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag", "ExampleFlag"}, &e.Config)
	require.NoError(t, err)

	expected := "| [example flag](https://example.com/test) | `example-flag` | `exampleFlag`, `ExampleFlag` |"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) AddedOnly(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "# LaunchDarkly flag references\n## :green_circle: 1 flag references added or modified\n\n| Flag name | Key | Aliases |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n <!-- flags:example-flag -->\n <!-- comment hash: 5e4ed16eb53ca4128b2cac3a47f785d3 -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "# LaunchDarkly flag references\n## :red_circle: 1 flag references removed\n\n| Flag name | Key | Aliases |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: cffaa8620ebdd00e8ca2954b61a92efd -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "# LaunchDarkly flag references\n## :green_circle: 1 flag references added or modified\n\n| Flag name | Key | Aliases |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n## :red_circle: 1 flag references removed\n\n| Flag name | Key | Aliases |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: f1a16a3880cd9ff51d62935c4c5bd8c8 -->"

	assert.Equal(t, expected, comment)

}

func (e *testProcessor) Basic(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{""}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{"| [example flag](https://example.com/test) | `example-flag` | |"},
	}
	assert.Equal(t, expected, processor)
}

func (e *testProcessor) Multi(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{""}
	e.FlagsRef.FlagsAdded["second-flag"] = []string{""}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{
			"| [example flag](https://example.com/test) | `example-flag` | |",
			"| [second flag](https://example.com/test) | `second-flag` | |",
		},
	}
	assert.Equal(t, expected, processor)
}
