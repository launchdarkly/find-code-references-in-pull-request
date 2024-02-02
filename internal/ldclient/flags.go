package ldapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"
	"github.com/pkg/errors"
)

func GetAllFlags(config *lcr.Config) ([]ldapi.FeatureFlag, error) {
	gha.LogDebug("Fetching all flags for project")
	params := url.Values{}
	params.Add("env", config.LdEnvironment)
	activeFlags, err := getFlags(config, params)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}

	flags := make([]ldapi.FeatureFlag, 0, len(activeFlags))
	flags = append(flags, activeFlags...)

	if config.IncludeArchivedFlags {
		params.Add("filter", "state:archived")
		archivedFlags, err := getFlags(config, params)
		if err != nil {
			return []ldapi.FeatureFlag{}, err
		}
		flags = append(flags, archivedFlags...)
	}

	return flags, nil
}

func getFlags(config *lcr.Config, params url.Values) ([]ldapi.FeatureFlag, error) {
	url := fmt.Sprintf("%s/api/v2/flags/%s", config.LdInstance, config.LdProject)
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Add("Authorization", config.ApiToken)
	req.Header.Add("LD-API-Version", "20220603")
	req.Header.Add("User-Agent", fmt.Sprintf("find-code-references-pr/%s", version.Version))

	resp, err := client.Do(req)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var r interface{}
		if err := decoder.Decode(&r); err != nil {
			return []ldapi.FeatureFlag{}, errors.Wrapf(err, "unexpected status code: %d. unable to parse response", resp.StatusCode)
		}
		err := fmt.Errorf("unexpected status code: %d with response: %#v", resp.StatusCode, r)
		return []ldapi.FeatureFlag{}, err
	}

	flags := ldapi.FeatureFlags{}
	err = decoder.Decode(&flags)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}

	return flags.Items, nil
}
