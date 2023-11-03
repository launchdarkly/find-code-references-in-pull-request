package ldclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/version"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	"github.com/pkg/errors"
)

func GetAllFlags(config *lcr.Config) ([]ldapi.FeatureFlag, error) {
	client := NewLDClient(config.LdInstance, config.ApiToken)

	params := url.Values{}
	params.Add("env", config.LdEnvironment)
	activeFlags, err := client.getFlags(config.LdProject, params)
	if err != nil {
		return nil, err
	}

	flags := make([]ldapi.FeatureFlag, 0, len(activeFlags))
	flags = append(flags, activeFlags...)

	if config.IncludeArchivedFlags {
		params.Add("filter", "state:archived")
		archivedFlags, err := client.getFlags(config.LdProject, params)
		if err != nil {
			return nil, err
		}
		flags = append(flags, archivedFlags...)
	}

	return flags, nil
}

func GetMultiProjectFlags(config *lcr.Config, opts options.Options) (map[string][]ldapi.FeatureFlag, error) {
	client := NewLDClient(config.LdInstance, config.ApiToken)

	flags := make(map[string][]ldapi.FeatureFlag, len(opts.Projects))

	for _, project := range opts.Projects {
		params := url.Values{}
		params.Add("env", config.LdEnvironment) // TODO figure out how to handle for multi-project
		activeFlags, err := client.getFlags(project.Key, params)
		if err != nil {
			return nil, err
		}

		projectFlags := activeFlags

		if config.IncludeArchivedFlags {
			params.Add("filter", "state:archived")
			archivedFlags, err := client.getFlags(config.LdProject, params)
			if err != nil {
				return nil, err
			}
			projectFlags = append(projectFlags, archivedFlags...)
		}
		flags[project.Key] = projectFlags
	}

	return flags, nil
}

func (c LDClient) getFlags(project string, params url.Values) ([]ldapi.FeatureFlag, error) {
	url := fmt.Sprintf("%s/api/v2/flags/%s", c.instance, project)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Add("Authorization", c.apiToken)
	req.Header.Add("LD-API-Version", "20220603")
	req.Header.Add("User-Agent", fmt.Sprintf("find-code-references-pr/%s", version.Version))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var r interface{}
		if err := decoder.Decode(&r); err != nil {
			return []ldapi.FeatureFlag{}, errors.Wrapf(err, "unexpected status code: %d. unable to parse response", resp.StatusCode)
		}
		err := fmt.Errorf("unexpected status code: %d with response: %#v", resp.StatusCode, r)
		return nil, err
	}

	flags := ldapi.FeatureFlags{}
	if err := decoder.Decode(&flags); err != nil {
		return nil, err
	}

	return flags.Items, nil
}
