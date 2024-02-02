package extinctions

import (
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	refs "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
	"github.com/launchdarkly/find-code-references-in-pull-request/search"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	ld_search "github.com/launchdarkly/ld-find-code-refs/v2/search"
)

func CheckExtinctions(opts options.Options, builder *refs.ReferenceSummaryBuilder) error {
	gha.StartLogGroup("Checking for extinctions...")
	defer gha.EndLogGroup()
	flagKeys := make([]string, 0, len(builder.RemovedFlagKeys()))

	matcher, err := search.GetMatcher(opts, flagKeys, nil)
	if err != nil {
		return err
	}

	gha.Debug("Searching for any remaining references to removed flags...")
	references, err := ld_search.SearchForRefs(opts.Dir, matcher)
	if err != nil {
		return err
	}

	for _, ref := range references {
		for _, hunk := range ref.Hunks {
			gha.Debug("Found reference to removed flag %s in %s", hunk.FlagKey, ref.Path)
			builder.AddHeadFlag(hunk.FlagKey)
		}
	}
	return nil
}
