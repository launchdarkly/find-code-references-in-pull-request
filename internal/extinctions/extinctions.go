package extinctions

import (
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	refs "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	ld_search "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, builder *refs.ReferenceSummaryBuilder) error {
	flagKeys := builder.RemovedFlagKeys()
	if len(flagKeys) == 0 {
		return nil
	}
	gha.StartLogGroup("Checking for extinctions...")
	defer gha.EndLogGroup()

	matcher, err := search.GetMatcher(opts, flagKeys, nil)
	if err != nil {
		return err
	}

	gha.Debug("Searching for any remaining references to %d removed flags...", len(flagKeys))
	references, err := ld_search.SearchForRefs(opts.Dir, matcher)
	if err != nil {
		return err
	}
	gha.Debug("Found %d references to removed flags", len(references))

	for _, ref := range references {
		for _, hunk := range ref.Hunks {
			gha.Debug("Flag '%s' is not extinct", hunk.FlagKey)
			builder.AddHeadFlag(hunk.FlagKey)
		}
	}
	return nil
}
