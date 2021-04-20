package comments

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go"
	"github.com/launchdarkly/cr-flags/config"
	"github.com/stretchr/testify/assert"
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
	environment := ldapi.FeatureFlagConfig{
		Site: &ldapi.Site{
			Href: "test",
		},
		Fallthrough_: &ldapi.ModelFallthrough{
			Variation: 0,
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
		Name:         "Sample Flag",
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

func TestGithubFlagComment(t *testing.T) {
	acceptanceTestEnv := newTestAccEnv()
	t.Run("Basic flag", acceptanceTestEnv.noAliasesNoTags)
	t.Run("Flag with alias", acceptanceTestEnv.Alias)
	t.Run("Flag with tag", acceptanceTestEnv.Tag)
	t.Run("Flag with aliases and tags", acceptanceTestEnv.AliasesAndTags)
	t.Run("Flag Rollout", acceptanceTestEnv.RolloutFlag)
}

func TestProcessFlags(t *testing.T) {
	processor := newProcessFlagAccEnv()
	t.Run("Basic Test", processor.Basic)
}
func TestGithubNoFlagComment(t *testing.T) {
	comment := GithubNoFlagComment()
	assert.Equal(t, "LaunchDarkly Flag Details:\n **No flag references found in PR**", *comment.Body, "they should be equal")
}

func TestBuildFlagComment(t *testing.T) {
	addedAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Added comments only", addedAcceptanceTestEnv.AddedOnly)

	removedAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Removed comments only", removedAcceptanceTestEnv.RemovedOnly)
	bothAcceptanceTestEnv := newCommentBuilderAccEnv()
	t.Run("Add and Remove comments", bothAcceptanceTestEnv.AddedAndRemoved)
}

func (e *testFlagEnv) noAliasesNoTags(t *testing.T) {

	comment, err := githubFlagComment(e.Flag, []string{}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nKind: **boolean**\nTemporary: **false**\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | `true`| |\n| Off | `true` | |\n\n", comment, "they should be equal")
}

func (e *testFlagEnv) Alias(t *testing.T) {
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag"}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nKind: **boolean**\nTemporary: **false**\nAliases: `exampleFlag`\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | `true`| |\n| Off | `true` | |\n\n", comment, "they should be equal")
}

func (e *testFlagEnv) Tag(t *testing.T) {
	e.Flag.Tags = []string{"myTag"}
	comment, err := githubFlagComment(e.Flag, []string{}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nTags: `myTag`\n\nKind: **boolean**\nTemporary: **false**\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | `true`| |\n| Off | `true` | |\n\n", comment, "they should be equal")
}

func (e *testFlagEnv) AliasesAndTags(t *testing.T) {
	e.Flag.Tags = []string{"myTag", "otherTag", "finalTag"}
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag", "example_flag", "ExampleFlag"}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nTags: `myTag`, `otherTag`, `finalTag`\n\nKind: **boolean**\nTemporary: **false**\nAliases: `exampleFlag`, `example_flag`, `ExampleFlag`\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | `true`| |\n| Off | `true` | |\n\n", comment, "they should be equal")
}

func (e *testFlagEnv) RolloutFlag(t *testing.T) {
	trueRollout := ldapi.WeightedVariation{
		Variation: 0,
		Weight:    12345,
	}
	falseRollout := ldapi.WeightedVariation{
		Variation: 1,
		Weight:    87655,
	}
	rollout := ldapi.Rollout{
		Variations: []ldapi.WeightedVariation{trueRollout, falseRollout},
	}
	environment := ldapi.FeatureFlagConfig{
		Site: &ldapi.Site{
			Href: "test",
		},
		Fallthrough_: &ldapi.ModelFallthrough{
			Rollout: &rollout,
		},
	}
	e.Flag.Environments["production"] = environment
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag", "example_flag", "ExampleFlag"}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nTags: `myTag`, `otherTag`, `finalTag`\n\nKind: **boolean**\nTemporary: **false**\nAliases: `exampleFlag`, `example_flag`, `ExampleFlag`\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | Rollout | |\n| |`true` | `12.345%`|\n| |`false` | `87.655%`|\n| Off | `true` | |\n\n", comment, "they should be equal")
}

func (e *testCommentBuilder) AddedOnly(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n** **Added/Modified** **\ncomment1\ncomment2\n comment hash: f66709e4eb57c204ca233601b6620203", comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n** **Removed** **\ncomment1\ncomment2\n comment hash: 293c9cd1d0c3b75c193fa614c0ac6bff", comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n** **Added/Modified** **\ncomment1\ncomment2\n---\n** **Removed** **\ncomment1\ncomment2\n comment hash: 2ab0148ecf63637c38a87d2f89eb2276", comment)
}

func (e *testProcessor) Basic(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{""}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{"\n**[Sample Flag](https://example.com/test)** `example-flag`\nKind: **boolean**\nTemporary: **false**\n\n\nEnvironment: **production**\n| Type | Variation | Weight(if Rollout) |\n| --- | --- | --- |\n| Default | `true`| |\n| Off | `true` | |\n\n"},
	}
	assert.Equal(t, expected, processor)
}
