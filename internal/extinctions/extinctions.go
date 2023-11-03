package extinctions

import (
	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, removedFlags lflags.FlagAliasMap) (map[string]struct{}, error) {
	flags := make([]string, 0, len(removedFlags))
	flagMap := make(map[string]struct{}, len(removedFlags))

	for flagKey := range removedFlags {
		flags = append(flags, flagKey)
		flagMap[flagKey] = struct{}{}
	}

	matcher, err := search.GetMatcher(opts, flags, nil)
	if err != nil {
		return nil, err
	}

	references, err := lsearch.SearchForRefs(opts.Dir, matcher)
	if err != nil {
		return nil, err
	}

check:
	for _, ref := range references {
		for _, hunk := range ref.Hunks {
			if _, ok := flagMap[hunk.FlagKey]; ok {
				delete(flagMap, hunk.FlagKey)
			}
			if len(flagMap) == 0 {
				break check
			}
		}
	}

	// remaining flags are extinct
	return flagMap, nil
}
