package config

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type Config struct {
	LdProject         string
	LdEnvironment     []string
	LdInstance        string
	Owner             string
	Repo              []string
	ApiToken          string
	Workspace         string
	GHClient          *github.Client
	ReferencePRonFlag bool
	MaxFlags          int
}

func ValidateInputandParse(ctx context.Context) (*Config, error) {
	var config Config
	config.LdProject = os.Getenv("INPUT_PROJKEY")
	if config.LdProject == "" {
		return nil, errors.New("`project` is required.")

	}
	config.LdEnvironment = strings.Split(os.Getenv("INPUT_ENVKEY"), ",")
	if len(config.LdEnvironment) == 0 {
		return nil, errors.New("`environment` is required.")
	}
	config.LdInstance = os.Getenv("INPUT_BASEURI")
	if config.LdInstance == "" {
		return nil, errors.New("`baseUri` is required.")
	}
	config.Owner = os.Getenv("GITHUB_REPOSITORY_OWNER")
	config.Repo = strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")

	config.ApiToken = os.Getenv("INPUT_ACCESSTOKEN")
	if config.ApiToken == "" {
		return nil, errors.New("`accessToken` is required.")
	}

	config.Workspace = os.Getenv("GITHUB_WORKSPACE")

	ReferencePRonFlag, err := strconv.ParseBool(os.Getenv("INPUT_REFERENCEPRONFLAG"))
	if err != nil {
		return nil, err
	}
	config.ReferencePRonFlag = ReferencePRonFlag

	MaxFlags, err := strconv.ParseInt(os.Getenv("INPUT_MAXFLAGS"), 10, 32)
	if err != nil {
		return nil, err
	}
	config.MaxFlags = int(MaxFlags)
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
