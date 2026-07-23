package flags

import (
	"testing"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils/diff_util"
	"github.com/stretchr/testify/assert"
)

func TestBuilder_Build(t *testing.T) {
	ref := ReferenceSummaryBuilder{
		flagsAdded: map[string][]string{
			"flag1": {"alias1"},
			"flag2": {"alias2"},
		},
		flagsRemoved: map[string][]string{
			"flag2": {},
			"flag3": {"alias3"},
			"flag4": {"alias4"},
		},
		flagsFoundAtHead: map[string]struct{}{
			"flag3": {},
		},
		foundFlags: map[string]struct{}{
			"flag1": {},
			"flag2": {},
			"flag3": {},
			"flag4": {},
		},
		counts: map[string]refCounts{
			"flag1": {adds: 1},
			"flag2": {adds: 1, deletes: 1},
			"flag3": {deletes: 1},
			"flag4": {deletes: 1},
		},
		includeExtinctions: true,
	}

	built := ref.Build()

	assert.Len(t, built.FlagsAdded, 2)
	assert.Len(t, built.FlagsRemoved, 2)
	assert.Len(t, built.ExtinctFlags, 1)
	assert.NotContains(t, built.FlagsRemoved, "flag2")
}

func TestBuilder_RemovedFlagKeys(t *testing.T) {
	ref := ReferenceSummaryBuilder{
		flagsAdded: map[string][]string{
			"flag1": {"alias1"},
			"flag2": {"alias2"},
		},
		flagsRemoved: map[string][]string{
			"flag2": {},
			"flag3": {"alias3"},
			"flag4": {"alias4"},
		},
		flagsFoundAtHead: map[string]struct{}{
			"flag3": {},
		},
		foundFlags: map[string]struct{}{
			"flag1": {},
			"flag2": {},
			"flag3": {},
			"flag4": {},
		},
		counts: map[string]refCounts{
			"flag1": {adds: 1},
			"flag2": {adds: 1, deletes: 1},
			"flag3": {deletes: 1},
			"flag4": {deletes: 1},
		},
		includeExtinctions: true,
	}

	keys := ref.RemovedFlagKeys()

	assert.Len(t, keys, 2)
	assert.ElementsMatch(t, keys, []string{"flag3", "flag4"})
}

func TestBuilder_Build_netZeroChurnIsModified(t *testing.T) {
	builder := NewReferenceSummaryBuilder(5, false)
	assert.NoError(t, builder.AddReference("my-flag", diff_util.OperationDelete, nil))
	assert.NoError(t, builder.AddReference("my-flag", diff_util.OperationAdd, nil))

	built := builder.Build()

	assert.Contains(t, built.FlagsAdded, "my-flag")
	assert.NotContains(t, built.FlagsRemoved, "my-flag")
	assert.Empty(t, builder.RemovedFlagKeys())
}
