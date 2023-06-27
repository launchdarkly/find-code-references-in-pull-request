package client

import (
	"context"
	"errors"
	"fmt"

	ldapi "github.com/launchdarkly/api-client-go/v7"
)

type Client struct {
	ApiKey  string
	ApiHost string
	Ld      *ldapi.APIClient
	Ctx     context.Context
}

const (
	APIVersion = "20191212"
)

func NewClient(token string, apiHost string, oauth bool) (*Client, error) {
	if token == "" {
		return nil, errors.New("token cannot be empty")
	}

	basePath := fmt.Sprintf("%s/api/v2", apiHost)

	cfg := &ldapi.Configuration{
		Host:          basePath,
		DefaultHeader: make(map[string]string),
		UserAgent:     "launchdarkly-pr-flags/0.1.0",
	}

	cfg.AddDefaultHeader("LD-API-Version", APIVersion)
	ctx := context.WithValue(context.Background(), ldapi.ContextAPIKey, ldapi.APIKey{
		Key: token,
	})
	if oauth {
		ctx = context.WithValue(context.Background(), ldapi.ContextAccessToken, token)
	}

	return &Client{
		ApiKey:  token,
		ApiHost: apiHost,
		Ld:      ldapi.NewAPIClient(cfg),
		Ctx:     ctx,
	}, nil
}
