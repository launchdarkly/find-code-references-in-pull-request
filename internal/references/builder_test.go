package flags

import (
	"testing"

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
		includeExtinctions: true,
	}

	built := ref.Build()

	assert.Len(t, built.FlagsAdded, 2)
	assert.Len(t, built.FlagsRemoved, 2)
	assert.Len(t, built.ExtinctFlags, 1)
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
		includeExtinctions: true,
	}

	keys := ref.RemovedFlagKeys()

	assert.Len(t, keys, 3)
	assert.ElementsMatch(t, keys, []string{"flag2", "flag3", "flag4"})
}
