package comments

import (
	"strings"
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	"github.com/launchdarkly/cr-flags/config"
	lflags "github.com/launchdarkly/cr-flags/flags"
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
		LdEnvironment: "production",
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
	FlagsRef lflags.FlagsRef
}

func newCommentBuilderAccEnv() *testCommentBuilder {
	flagComments := FlagComments{
		CommentsAdded:   []string{},
		CommentsRemoved: []string{},
	}
	flagsAdded := make(lflags.FlagAliasMap)
	flagsRemoved := make(lflags.FlagAliasMap)
	flagsRef := lflags.FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	return &testCommentBuilder{
		Comments: flagComments,
		FlagsRef: flagsRef,
	}
}

type testProcessor struct {
	Flags    []ldapi.FeatureFlag
	FlagsRef lflags.FlagsRef
	Config   config.Config
}

func newProcessFlagAccEnv() *testProcessor {
	flag := createFlag("example-flag")
	flags := []ldapi.FeatureFlag{flag}
	flagsAdded := make(lflags.FlagAliasMap)
	flagsRemoved := make(lflags.FlagAliasMap)
	flagsRef := lflags.FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	config := config.Config{
		LdEnvironment: "production",
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
	flags := []ldapi.FeatureFlag{flag, flag2}
	flagsAdded := make(lflags.FlagAliasMap)
	flagsRemoved := make(lflags.FlagAliasMap)
	flagsRef := lflags.FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	config := config.Config{
		LdEnvironment: "production",
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
	assert.Equal(t, "## LaunchDarkly flag references\n\n **No flag references found in PR**", *comment.Body, "they should be equal")
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
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :green_circle: 1 flag reference added or modified\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n <!-- flags:example-flag -->\n <!-- comment hash: 77f715c4d3584af227af5d5e661aad5e -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = lflags.AliasSet{}
	e.FlagsRef.FlagsRemoved["sample-flag"] = lflags.AliasSet{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :red_circle: 2 flag references removed\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag,sample-flag -->\n <!-- comment hash: 10e2cf9d20d151dd33a99dfde7f8ec5b -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{}
	e.FlagsRef.FlagsRemoved["example-flag"] = lflags.AliasSet{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :green_circle: 1 flag reference added or modified\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n### :red_circle: 1 flag reference removed\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: ef0ca0c51fa26dc61b10100ef1bf55eb -->"

	assert.Equal(t, expected, comment)

}

func (e *testProcessor) Basic(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{"": true}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{"| [example flag](https://example.com/test) | `example-flag` | |"},
	}
	assert.Equal(t, expected, processor)
}

func (e *testProcessor) Multi(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{"": true}
	e.FlagsRef.FlagsAdded["second-flag"] = lflags.AliasSet{"": true}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{
			"| [example flag](https://example.com/test) | `example-flag` | |",
			"| [second flag](https://example.com/test) | `second-flag` | |",
		},
	}
	assert.Equal(t, expected, processor)
}
