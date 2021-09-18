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
	testVal := "test"
	varVal := int32(0)
	environment := ldapi.FeatureFlagConfig{
		EnvironmentName: "Production",
		Site: ldapi.Link{
			Href: &testVal,
		},
		Fallthrough: ldapi.VariationOrRolloutRep{
			Variation: &varVal,
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
	testVal := "test"
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
		Site: ldapi.Link{
			Href: &testVal,
		},
		Fallthrough: ldapi.VariationOrRolloutRep{
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
	assert.Equal(t, "LaunchDarkly Flag Details, references to flags have been found in the diff:\n\n\nFlag references: Added/Modified (1)\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: b47fc43b30d97bf647a43d48ce1b85fd -->", comment)
}

func (e *testCommentBuilder) RemovedOnly(t *testing.T) {
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details, references to flags have been found in the diff:\n\n\nFlag references: Removed (1)\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: e39d16108df3e5369f43176ab1a12323 -->", comment)
}

func (e *testCommentBuilder) AddedAndRemoved(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{}
	e.FlagsRef.FlagsRemoved["example-flag"] = []string{}
	e.Comments.CommentsAdded = []string{"comment1", "comment2"}
	e.Comments.CommentsRemoved = []string{"comment1", "comment2"}
	comment := BuildFlagComment(e.Comments, e.FlagsRef, nil)
	assert.Equal(t, "LaunchDarkly Flag Details, references to flags have been found in the diff:\n\n\nFlag references: Added/Modified (1)\ncomment1\ncomment2\nFlag references: Removed (1)\ncomment1\ncomment2\n <!-- flags:example-flag -->\n <!-- comment hash: e9b00faf6005ae2ed28911472366fcc0 -->", comment)
}

func (e *testProcessor) Basic(t *testing.T) {
	e.FlagsRef.FlagsAdded["example-flag"] = []string{""}
	processor := ProcessFlags(e.FlagsRef, e.Flags, &e.Config)
	expected := FlagComments{
		CommentsAdded: []string{"\n\n", "- <details><summary> Sample Flag</summary>", "\n\t**[Sample Flag](https://example.com/test)** `example-flag`\n\tKind: **boolean**\n\tTemporary: **false**\n\t\n\n\tEnvironment: **Production** `production`\n\t| Type | Variation | Weight(if Rollout) |\n\t| --- | --- | --- |\n\t| Default | `true`| |\n\t| Off | `true` | |\n\t\n", "\t</details>"},
	}
	assert.Equal(t, expected, processor)
}
