package search

import (
	"strings"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	laliases "github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/aliases"
)

func GetMatcher(opts options.Options, flags []ldapi.FeatureFlag, diffContents laliases.FileContentsMap) (matcher lsearch.Matcher, err error) {
	flagKeys := make([]string, 0, len(flags))
	for _, flag := range flags {
		flagKeys = append(flagKeys, flag.Key)
	}

	aliasesByFlagKey, err := aliases.GenerateAliases(opts, flagKeys, diffContents)
	if err != nil {
		return lsearch.Matcher{}, err
	}

	delimiters := strings.Join(lsearch.GetDelimiters(opts), "")
	elements := make([]lsearch.ElementMatcher, 0, 1)
	elements = append(elements, lsearch.NewElementMatcher(opts.ProjKey, "", delimiters, flagKeys, aliasesByFlagKey))

	matcher = lsearch.Matcher{
		Elements: elements,
	}

	return matcher, nil
}
