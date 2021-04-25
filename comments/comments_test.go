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
		EnvironmentName: "Production",
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
	assert.Equal(t, []string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tKind: **boolean**\n\tTemporary: **false**\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"}, comment, "they should be equal")
}

func (e *testFlagEnv) Alias(t *testing.T) {
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag"}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, []string([]string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tKind: **boolean**\n\tTemporary: **false**\n\tAliases: `exampleFlag`\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"}), comment, "they should be equal")
}

func (e *testFlagEnv) Tag(t *testing.T) {
	e.Flag.Tags = []string{"myTag"}
	comment, err := githubFlagComment(e.Flag, []string{}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, []string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tTags: `myTag`\n\t\n\tKind: **boolean**\n\tTemporary: **false**\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"}, comment, "they should be equal")
}

func (e *testFlagEnv) AliasesAndTags(t *testing.T) {
	e.Flag.Tags = []string{"myTag", "otherTag", "finalTag"}
	comment, err := githubFlagComment(e.Flag, []string{"exampleFlag", "example_flag", "ExampleFlag"}, &e.Config)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, []string([]string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tTags: `myTag`, `otherTag`, `finalTag`\n\t\n\tKind: **boolean**\n\tTemporary: **false**\n\tAliases: `exampleFlag`, `example_flag`, `ExampleFlag`\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"}), comment, "they should be equal")
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
	assert.Equal(t, []string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tTags: `myTag`, `otherTag`, `finalTag`\n\t\n\tKind: **boolean**\n\tTemporary: **false**\n\tAliases: `exampleFlag`, `example_flag`, `ExampleFlag`\n\t\n\n\tEnvironment: `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | Rollout | |\n\t| |`true` | `12.345%`|\n\t| |`false` | `87.655%`|\n\t| Off | `true` | |\n\t\n", "\t</details>"}, comment, "they should be equal")
}

func (e *testCommentBuilder) AddedOnly(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n<details><summary>Flags: Added/Modified (1)</summary>\ncomment1\ncomment2\n</details>\n <!-- flags:example-flag -->\n <!-- comment hash: 2cdf50a05fa94a4098331f402de44fea -->", comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n<details><summary>Flags: Removed (0)</summary>\ncomment1\ncomment2\n</details>\n <!-- flags:example-flag -->\n <!-- comment hash: 4e6fe593e0cc7e779246fe3be6ae1183 -->", comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details:\n<details><summary>Flags: Added/Modified (1)</summary>\ncomment1\ncomment2\n</details>\n---\n<details><summary>Flags: Removed (1)</summary>\ncomment1\ncomment2\n</details>\n <!-- flags:example-flag -->\n <!-- comment hash: d10bec70a2e34a96172e75b31a46c728 -->", comment)
}

func (e *testProcessor) Basic(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{""}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tKind: **boolean**\n\tTemporary: **false**\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"},
	}
	assert.Equal(t, expected, processor)
}
