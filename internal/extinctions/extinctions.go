package extinctions

import (
	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, builder *lflags.ReferenceBuilder) error {
	flags := make([]string, 0, len(builder.RemovedFlagKeys()))

	matcher, err := search.GetMatcher(opts, flags, nil)
	if err != nil {
		return err
	}

	references, err := lsearch.SearchForRefs(opts.Dir, matcher)
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
