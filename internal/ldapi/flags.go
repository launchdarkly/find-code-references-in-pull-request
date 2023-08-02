package ldapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	lcr "github.com/launchdarkly/cr-flags/config"
)

func GetFlags(config *lcr.Config) ([]ldapi.FeatureFlag, error) {
	url := fmt.Sprintf("%s/api/v2/flags/%s?env=%s", config.LdInstance, config.LdProject, config.LdEnvironment)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}
	req.Header.Add("Authorization", config.ApiToken)
	req.Header.Add("LD-API-Version", "20220603")

	resp, err := client.Do(req)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}
	defer resp.Body.Close()

	flags := ldapi.FeatureFlags{}
	err = json.NewDecoder(resp.Body).Decode(&flags)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}

	return flags.Items, nil
}
