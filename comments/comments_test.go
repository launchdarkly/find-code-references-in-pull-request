package comments

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGithubNoFlagComment(t *testing.T) {
	comment := GithubNoFlagComment()
	assert.Equal(t, "LaunchDarkly Flag Details:\n **No flag references found in PR**", *comment.Body, "they should be equal")
}
