package comments

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go"
	"github.com/stretchr/testify/assert"
)

func ptr(v interface{}) *interface{} { return &v }

func TestGithubFlagComment(t *testing.T) {
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
	basicFlag := ldapi.FeatureFlag{
		Key:          "example-flag",
		Name:         "Sample Flag",
		Kind:         "boolean",
		Environments: map[string]ldapi.FeatureFlagConfig{"production": environment},
		Variations:   []ldapi.Variation{variationTrue, variationFalse},
	}
	comment, err := GithubFlagComment(basicFlag, []string{}, "production", "https://example.com/")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	assert.Equal(t, "\n**[Sample Flag](https://example.com/test)** `example-flag`\n\nDefault variation: `true`\nOff variation: `true`\nKind: **boolean**\nTemporary: **false**\n", comment, "they should be equal")
}

func TestGithubNoFlagComment(t *testing.T) {
	comment := GithubNoFlagComment()
	assert.Equal(t, "LaunchDarkly Flag Details:\n **No flag references found in PR**", *comment.Body, "they should be equal")
}
