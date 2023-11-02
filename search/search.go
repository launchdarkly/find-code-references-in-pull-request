package search

import (
	"strings"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	laliases "github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"

	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/aliases"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils"
)

func GetMatcher(config *lcr.Config, opts options.Options, flags []ldapi.FeatureFlag, diffContents laliases.FileContentsMap) (matcher lsearch.Matcher, err error) {
	flagKeys := make([]string, 0, len(flags))
	for _, flag := range flags {
		flagKeys = append(flagKeys, flag.Key)
	}

	aliasesByFlagKey, err := aliases.GenerateAliases(config, opts, flagKeys, diffContents)
	if err != nil {
		return lsearch.Matcher{}, err
	}

	delimiters := strings.Join(utils.Dedupe(getDelimiters(opts)), "")
	elements := []lsearch.ElementMatcher{}
	elements = append(elements, lsearch.NewElementMatcher(config.LdProject, "", delimiters, flagKeys, aliasesByFlagKey))
	matcher = lsearch.Matcher{
		Elements: elements,
	}

	return matcher, nil
}

func getDelimiters(opts options.Options) []string {
	delims := []string{`"`, `'`, "`"}
	if opts.Delimiters.DisableDefaults {
		delims = []string{}
	}

	delims = append(delims, opts.Delimiters.Additional...)

	return delims
}
