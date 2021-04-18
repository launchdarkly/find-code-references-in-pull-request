package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	LdProject     string
	LdEnvironment string
	LdInstance    string
	Owner         string
	Repo          []string
	ApiToken      string
	Workspace     string
}

func ValidateInputandParse() *Config {
	var config Config
	config.LdProject = os.Getenv("INPUT_PROJKEY")
	if config.LdProject == "" {
		fmt.Println("`project` is required.")
		os.Exit(1)
	}
	config.LdEnvironment = os.Getenv("INPUT_ENVKEY")
	if config.LdEnvironment == "" {
		fmt.Println("`environment` is required.")
		os.Exit(1)
	}
	config.LdInstance = os.Getenv("INPUT_BASEURI")
	if config.LdInstance == "" {
		fmt.Println("`baseUri` is required.")
		os.Exit(1)
	}
	config.Owner = os.Getenv("GITHUB_REPOSITORY_OWNER")
	config.Repo = strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")

	config.ApiToken = os.Getenv("INPUT_ACCESSTOKEN")
	if config.ApiToken == "" {
		fmt.Println("`accessToken` is required.")
		os.Exit(1)
	}

	config.Workspace = os.Getenv("GITHUB_WORKSPACE")

	return &config
}
