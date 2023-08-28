package comments

import (
	"strings"
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	"github.com/launchdarkly/find-code-references-in-pull-request/config"
	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](t T) *T { return &t }

type testFlagEnv struct {
	Flag         ldapi.FeatureFlag
	ArchivedFlag ldapi.FeatureFlag
	Config       config.Config
}

func newTestAccEnv() *testFlagEnv {
	flag := createFlag("example-flag")
	config := config.Config{
		LdEnvironment: "production",
		LdInstance:    "https://example.com/",
	}

	archivedFlag := createFlag("archived-flag")
	archivedFlag.Archived = true
	archivedFlag.ArchivedDate = ptr(int64(1691072480000))
	return &testFlagEnv{
		Flag:         flag,
		ArchivedFlag: archivedFlag,
		Config:       config,
	}
}

func createFlag(key string) ldapi.FeatureFlag {
	environment := ldapi.FeatureFlagConfig{
		EnvironmentName: "Production",
		Site: ldapi.Link{
			Href: ptr("test"),
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
	t.Run("Archived flag added", acceptanceTestEnv.ArchivedAdded)
	t.Run("Archived flag removed", acceptanceTestEnv.ArchivedRemoved)
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
	comment, err := githubFlagComment(e.Flag, []string{}, true, &e.Config)
	require.NoError(t, err)

	expected := "| [example flag](https://example.com/test) | `example-flag` | |"
	assert.Equal(t, expected, comment)
}

func (e *testFlagEnv) Alias(t *testing.T) {
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag", "ExampleFlag"}, true, &e.Config)
	require.NoError(t, err)

	expected := "| [example flag](https://example.com/test) | `example-flag` | `exampleFlag`, `ExampleFlag` |"
	assert.Equal(t, expected, comment)
}

func (e *testFlagEnv) ArchivedAdded(t *testing.T) {
	comment, err := githubFlagComment(e.ArchivedFlag, []string{}, true, &e.Config)
	require.NoError(t, err)

	expected := "| :warning: [archived flag](https://example.com/test) (archived on 2023-08-03) | `archived-flag` | |"
	assert.Equal(t, expected, comment)
}

func (e *testFlagEnv) ArchivedRemoved(t *testing.T) {
	comment, err := githubFlagComment(e.ArchivedFlag, []string{}, false, &e.Config)
	require.NoError(t, err)

	expected := "| [archived flag](https://example.com/test) (archived on 2023-08-03) | `archived-flag` | |"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) AddedOnly(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :mag: 1 flag added or modified\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n <!-- flags:example-flag -->\n <!-- comment hash: c449ce18623b2038f1ae2f02c46869cd -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = lflags.AliasSet{}
	e.FlagsRef.FlagsRemoved["sample-flag"] = lflags.AliasSet{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :x: 2 flags removed\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag,sample-flag -->\n <!-- comment hash: 9ab2cb5c0fcfcce9002cf0935f5f4ad5 -->"
	assert.Equal(t, expected, comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = lflags.AliasSet{}
	e.FlagsRef.FlagsRemoved["example-flag"] = lflags.AliasSet{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)

	expected := "## LaunchDarkly flag references\n### :mag: 1 flag added or modified\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n\n\n### :x: 1 flag removed\n\n| Name | Key | Aliases found |\n| --- | --- | --- |\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: 7e6314b3b27a97ed6f8979304650af54 -->"

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
