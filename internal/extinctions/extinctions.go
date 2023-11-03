package extinctions

import (
	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, removedFlags lflags.FlagAliasMap) (map[string]struct{}, error) {
	flags := make([]string, 0, len(removedFlags))

	for flagKey := range removedFlags {
		flags = append(flags, flagKey)
	}

	matcher, err := search.GetMatcher(opts, flags, nil)
	if err != nil {
		return nil, err
	}

	referenceHunks, err := lsearch.SearchForRefs(opts.Dir, matcher)
	if err != nil {
		return nil, err
	}

	foundFlags := make(map[string]struct{}, len(removedFlags))
	for _, reference := range referenceHunks {
		for _, hunk := range reference.Hunks {
			foundFlags[hunk.FlagKey] = struct{}{}
		}
	}

	extinctFlags := make(map[string]struct{}, len(removedFlags))
	for flagKey := range removedFlags {
		if _, ok := foundFlags[flagKey]; !ok {
			extinctFlags[flagKey] = struct{}{}
		}
	}

	return extinctFlags, nil
}
