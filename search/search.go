package search

import (
	"strings"

	laliases "github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/aliases"
)

func GetMatcher(opts options.Options, flagKeys []string, diffContents laliases.FileContentsMap) (matcher lsearch.Matcher, err error) {
	aliasesByFlagKey, err := aliases.GenerateAliases(opts, flagKeys, diffContents)
	if err != nil {
		return lsearch.Matcher{}, err
	}

	delimiters := strings.Join(lsearch.GetDelimiters(opts), "")
	elements := make([]lsearch.ElementMatcher, 0, 1)
	elements = append(elements, lsearch.NewElementMatcher(opts.ProjKey, opts.Dir, delimiters, flagKeys, aliasesByFlagKey))
	matcher = lsearch.Matcher{
		Elements: elements,
	}

	return matcher, nil
}
