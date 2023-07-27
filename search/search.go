package search

import (
	"strings"

	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"

	lcr "github.com/launchdarkly/cr-flags/config"
)

func GetMatcher(config *lcr.Config, opts options.Options, flagKeys []string) (matcher lsearch.Matcher, err error) {
	elements := []lsearch.ElementMatcher{}

	aliasesByFlagKey, err := aliases.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)
	if err != nil {
		return lsearch.Matcher{}, err
	}

	delimiters := strings.Join(dedupe(getDelimiters(opts)), "")
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

func dedupe(s []string) []string {
	if len(s) <= 1 {
		return s
	}
	keys := make(map[string]struct{}, len(s))
	ret := make([]string, 0, len(s))
	for _, entry := range s {
		if _, value := keys[entry]; !value {
			keys[entry] = struct{}{}
			ret = append(ret, entry)
		}
	}
	return ret
}
