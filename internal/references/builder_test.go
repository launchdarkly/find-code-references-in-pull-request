package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuilder(t *testing.T) {
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
