package comments

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go"
	"github.com/stretchr/testify/assert"
)

func ptr(v interface{}) *interface{} { return &v }

type testEnv struct {
	Flag ldapi.FeatureFlag
}

func newTestAccEnv() *testEnv {

	flag := createFlag("example-flag")

	return &testEnv{
		Flag: flag,
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
func TestGithubFlagComment(t *testing.T) {
	acceptanceTestEnv := newTestAccEnv()
	t.Run("basic flag", acceptanceTestEnv.noAliasesNoTags)
	t.Run("flag with alias", acceptanceTestEnv.Alias)
	t.Run("flag with tag", acceptanceTestEnv.Tag)
	t.Run("flag with aliases and tags", acceptanceTestEnv.AliasesAndTags)
}

func TestGithubNoFlagComment(t *testing.T) {
	comment := GithubNoFlagComment()
	assert.Equal(t, "LaunchDarkly Flag Details:\n **No flag references found in PR**", *comment.Body, "they should be equal")
}

func (e *testEnv) noAliasesNoTags(t *testing.T) {
	comment, err := GithubFlagComment(e.Flag, []string{}, "production", "https://example.com/")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\n\nDefault variation: `true`\nOff variation: `true`\nKind: **boolean**\nTemporary: **false**\n", comment, "they should be equal")
}

func (e *testEnv) Alias(t *testing.T) {
	comment, err := GithubFlagComment(e.Flag, []string{"exampleFlag"}, "production", "https://example.com/")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\n\nDefault variation: `true`\nOff variation: `true`\nKind: **boolean**\nTemporary: **false**\nAliases: `exampleFlag`\n", comment, "they should be equal")
}

func (e *testEnv) Tag(t *testing.T) {
	e.Flag.Tags = []string{"myTag"}
	comment, err := GithubFlagComment(e.Flag, []string{}, "production", "https://example.com/")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nTags: `myTag`\n\nDefault variation: `true`\nOff variation: `true`\nKind: **boolean**\nTemporary: **false**\n", comment, "they should be equal")
}

func (e *testEnv) AliasesAndTags(t *testing.T) {
	e.Flag.Tags = []string{"myTag", "otherTag", "finalTag"}
	comment, err := GithubFlagComment(e.Flag, []string{"exampleFlag", "example_flag", "ExampleFlag"}, "production", "https://example.com/")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\nTags: `myTag`, `otherTag`, `finalTag`\n\nDefault variation: `true`\nOff variation: `true`\nKind: **boolean**\nTemporary: **false**\nAliases: `exampleFlag`, `example_flag`, `ExampleFlag`\n", comment, "they should be equal")
}
