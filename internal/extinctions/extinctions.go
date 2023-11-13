package extinctions

import (
	"github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	ld_search "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, builder *flags.ReferenceBuilder) error {
	flagKeys := make([]string, 0, len(builder.RemovedFlagKeys()))

	matcher, err := search.GetMatcher(opts, flagKeys, nil)
	if err != nil {
		return err
	}

	references, err := ld_search.SearchForRefs(opts.Dir, matcher)
	if err != nil {
		return err
	}

	for _, ref := range references {
		for _, hunk := range ref.Hunks {
			builder.ExistingFlag(hunk.FlagKey)
		}
	}

	return nil
}
