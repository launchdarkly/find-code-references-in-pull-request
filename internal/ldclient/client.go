package ldclient

import "net/http"

type LDClient struct {
	client   *http.Client
	instance string
	apiToken string
}

func NewLDClient(instance, apiToken string) LDClient {
	return LDClient{
		client:   new(http.Client),
		instance: instance,
		apiToken: apiToken,
	}
}
