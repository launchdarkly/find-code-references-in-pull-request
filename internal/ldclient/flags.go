package ldapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	ldapi "github.com/launchdarkly/api-client-go/v15"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"
	"github.com/pkg/errors"
)

func GetAllFlags(config *lcr.Config) ([]ldapi.FeatureFlag, error) {
	gha.Debug("Fetching all flags for project")
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

	gha.Debug("Fetched %d flags", len(flags))
	return flags, nil
}

func getFlags(config *lcr.Config, params url.Values) ([]ldapi.FeatureFlag, error) {
	pageParams := make(url.Values, len(params)+2)
	for key, values := range params {
		pageParams[key] = append([]string(nil), values...)
	}
	const pageSize = 100
	pageParams.Set("limit", strconv.Itoa(pageSize))

	client := &http.Client{}
	flags := []ldapi.FeatureFlag{}
	for offset := 0; ; offset += pageSize {
		pageParams.Set("offset", strconv.Itoa(offset))
		page, err := getFlagPage(client, config, pageParams)
		if err != nil {
			return []ldapi.FeatureFlag{}, err
		}
		// The migration guide uses an empty page to signal completion.
		if len(page) == 0 {
			return flags, nil
		}
		flags = append(flags, page...)
	}
}

func getFlagPage(client *http.Client, config *lcr.Config, params url.Values) ([]ldapi.FeatureFlag, error) {
	url := fmt.Sprintf("%s/api/v2/flags/%s", config.LdInstance, config.LdProject)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return []ldapi.FeatureFlag{}, err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Add("Authorization", config.ApiToken)
	req.Header.Add("LD-API-Version", "20240415")
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
