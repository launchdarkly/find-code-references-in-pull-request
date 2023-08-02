package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type Config struct {
	LdProject          string
	LdEnvironment      string
	LdInstance         string
	Owner              string
	Repo               string
	ApiToken           string
	Workspace          string
	GHClient           *github.Client
	MaxFlags           int
	PlaceholderComment bool
}

func ValidateInputandParse(ctx context.Context) (*Config, error) {
	// mask tokens
	if accessToken := os.Getenv("INPUT_ACCESS-TOKEN"); len(accessToken) > 0 {
		fmt.Printf("::add-mask::%s\n", accessToken)
	}
	if repoToken := os.Getenv("INPUT_REPO-TOKEN"); len(repoToken) > 0 {
		fmt.Printf("::add-mask::%s\n", repoToken)
	}

	// set config
	var config Config
	config.LdProject = os.Getenv("INPUT_PROJECT-KEY")
	if config.LdProject == "" {
		return nil, errors.New("`project-key` is required")
	}
	config.LdEnvironment = os.Getenv("INPUT_ENVIRONMENT-KEY")
	if len(config.LdEnvironment) == 0 {
		return nil, errors.New("`environment-key` is required")
	}
	config.LdInstance = os.Getenv("INPUT_BASE-URI")
	if config.LdInstance == "" {
		return nil, errors.New("`base-uri` is required.")
	}
	config.Owner = os.Getenv("GITHUB_REPOSITORY_OWNER")
	config.Repo = strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")[1]

	config.ApiToken = os.Getenv("INPUT_ACCESS-TOKEN")
	if config.ApiToken == "" {
		return nil, errors.New("`access-token` is required")
	}

	config.Workspace = os.Getenv("GITHUB_WORKSPACE")

	maxFlags, err := strconv.ParseInt(os.Getenv("INPUT_MAX-FLAGS"), 10, 32)
	if err != nil {
		return nil, err
	}
	config.MaxFlags = int(maxFlags)

	if placholderComment, err := strconv.ParseBool(os.Getenv("INPUT_PLACEHOLDER-COMMENT")); err == nil {
		// ignore error
		config.PlaceholderComment = placholderComment
	}

	config.GHClient = getGithubClient(ctx)
	return &config, nil
}

func getGithubClient(ctx context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}
