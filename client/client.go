package client

import (
	"context"
	"errors"
	"fmt"

	ldapi "github.com/launchdarkly/api-client-go"
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

	auth := make(map[string]ldapi.APIKey)
	auth["ApiKey"] = ldapi.APIKey{
		Key: token,
	}
	fmt.Println(auth)
	cfg := &ldapi.Configuration{
		Host:      apiHost,
		UserAgent: fmt.Sprintf("launchdarkly-pr-flags/0.1.0"),
	}

	//cfg.AddDefaultHeader("LD-API-Version", APIVersion)
	ctx := context.WithValue(context.Background(), ldapi.ContextAPIKeys, auth)

	return &Client{
		Ld:  ldapi.NewAPIClient(cfg),
		Ctx: ctx,
	}, nil
}

func (w *Client) WrapContext(ctx context.Context) context.Context {
	auth := make(map[string]ldapi.APIKey)
	auth["ApiKey"] = ldapi.APIKey{
		Key: w.ApiKey,
	}
	return context.WithValue(ctx, ldapi.ContextAPIKeys, auth)

}
