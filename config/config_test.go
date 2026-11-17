package config

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureOutput redirects os.Stdout and os.Stderr while fn runs and returns
// everything written to them.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	defer func() {
		os.Stdout, os.Stderr = origStdout, origStderr
	}()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	require.NoError(t, w.Close())
	return <-done
}

const (
	sentinelAccessToken = "api-sentinel-access-token-0123456789"
	sentinelRepoToken   = "ghs_sentinelRepoToken0123456789"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INPUT_ACCESS-TOKEN", sentinelAccessToken)
	t.Setenv("INPUT_REPO-TOKEN", sentinelRepoToken)
	t.Setenv("GITHUB_TOKEN", sentinelRepoToken)
	t.Setenv("INPUT_PROJECT-KEY", "default")
	t.Setenv("INPUT_ENVIRONMENT-KEY", "production")
	t.Setenv("INPUT_BASE-URI", "https://app.launchdarkly.com")
	t.Setenv("INPUT_MAX-FLAGS", "5")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "example")
	t.Setenv("GITHUB_REPOSITORY", "example/repo")
	// GHE host exercises the gha.Log call in getGithubClient.
	t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.test")
}

func TestValidateInputandParse_DoesNotWriteTokensByDefault(t *testing.T) {
	for name, value := range map[string]*string{
		"unset":   nil,
		"false":   strPtr("false"),
		"invalid": strPtr("yes please"),
	} {
		t.Run(name, func(t *testing.T) {
			setRequiredEnv(t)
			if value != nil {
				t.Setenv("INPUT_MASK-TOKENS", *value)
			}

			var cfg *Config
			var parseErr error
			out := captureOutput(t, func() {
				cfg, parseErr = ValidateInputandParse(context.Background())
			})

			require.NoError(t, parseErr)
			assert.False(t, cfg.MaskTokens)
			assert.Equal(t, sentinelAccessToken, cfg.ApiToken)
			assert.NotContains(t, out, sentinelAccessToken)
			assert.NotContains(t, out, sentinelRepoToken)
			assert.False(t, strings.Contains(out, "::add-mask::"), "unexpected ::add-mask:: command in output: %q", out)
		})
	}
}

func TestValidateInputandParse_MaskTokensOptIn(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("INPUT_MASK-TOKENS", "true")

	var cfg *Config
	var parseErr error
	out := captureOutput(t, func() {
		cfg, parseErr = ValidateInputandParse(context.Background())
	})

	require.NoError(t, parseErr)
	assert.True(t, cfg.MaskTokens)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	// Masks must be the first lines written, before any other output.
	assert.Equal(t, "::add-mask::"+sentinelAccessToken, lines[0])
	assert.Equal(t, "::add-mask::"+sentinelRepoToken, lines[1])
}

func strPtr(s string) *string { return &s }
