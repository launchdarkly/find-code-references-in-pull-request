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

	return &Client{
		Ld: ldapi.NewAPIClient(ldapi.NewConfiguration()),
	}, nil
}

func (w *Client) WrapContext(ctx context.Context) context.Context {
	fmt.Println(w.ApiKey)
	auth := map[string]ldapi.APIKey{
		"ApiKey": {Key: w.ApiKey},
	}
	return context.WithValue(ctx, ldapi.ContextAPIKeys, auth)

}
