package flags

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils/diff_util"
)

type refCounts struct {
	adds    int
	deletes int
}

type ReferenceSummaryBuilder struct {
	max                int  // maximum number of flags to find
	includeExtinctions bool // include extinctions in summary
	flagsAdded         map[string][]string
	flagsRemoved       map[string][]string
	flagsFoundAtHead   map[string]struct{}
	foundFlags         map[string]struct{}
	counts             map[string]refCounts
}

func NewReferenceSummaryBuilder(max int, includeExtinctions bool) *ReferenceSummaryBuilder {
	return &ReferenceSummaryBuilder{
		flagsAdded:         make(map[string][]string),
		flagsRemoved:       make(map[string][]string),
		foundFlags:         make(map[string]struct{}),
		flagsFoundAtHead:   make(map[string]struct{}),
		counts:             make(map[string]refCounts),
		max:                max,
		includeExtinctions: includeExtinctions,
	}
}

func (b *ReferenceSummaryBuilder) MaxReferences() bool {
	return len(b.foundFlags) >= b.max
}

// Add a found flag in diff by operation
func (b *ReferenceSummaryBuilder) AddReference(flagKey string, op diff_util.Operation, aliases []string) error {
	switch op {
	case diff_util.OperationAdd:
		b.addedFlag(flagKey, aliases)
	case diff_util.OperationDelete:
		b.removedFlag(flagKey, aliases)
	default:
		return fmt.Errorf("invalid operation=%s", op.String())
	}

	return nil
}

// Flag found in HEAD ref
func (b *ReferenceSummaryBuilder) AddHeadFlag(flagKey string) {
	if _, ok := b.flagsFoundAtHead[flagKey]; !ok {
		b.flagsFoundAtHead[flagKey] = struct{}{}
	}
}

func (b *ReferenceSummaryBuilder) foundFlag(flagKey string) {
	if _, ok := b.foundFlags[flagKey]; !ok {
		b.foundFlags[flagKey] = struct{}{}
	}
}

// Flag and aliases found in added diff
func (b *ReferenceSummaryBuilder) addedFlag(flagKey string, aliases []string) {
	b.foundFlag(flagKey)
	counts := b.counts[flagKey]
	counts.adds++
	b.counts[flagKey] = counts
	if _, ok := b.flagsAdded[flagKey]; !ok {
		b.flagsAdded[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsAdded[flagKey] = append(b.flagsAdded[flagKey], aliases...)
}

// Flag and aliases found in removed diff
func (b *ReferenceSummaryBuilder) removedFlag(flagKey string, aliases []string) {
	b.foundFlag(flagKey)
	counts := b.counts[flagKey]
	counts.deletes++
	b.counts[flagKey] = counts
	if _, ok := b.flagsRemoved[flagKey]; !ok {
		b.flagsRemoved[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsRemoved[flagKey] = append(b.flagsRemoved[flagKey], aliases...)
}

// Returns a list of removed flag keys
func (b *ReferenceSummaryBuilder) RemovedFlagKeys() []string {
	keys := make([]string, 0, len(b.flagsRemoved))
	for k, counts := range b.counts {
		if counts.deletes > 0 && counts.adds == 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

func (b *ReferenceSummaryBuilder) Build() ReferenceSummary {
	added := make(map[string][]string, len(b.flagsAdded))
	removed := make(map[string][]string, len(b.flagsRemoved))
	extinctions := make(map[string]struct{}, len(b.flagsRemoved))

	for flagKey := range b.foundFlags {
		counts := b.counts[flagKey]
		switch {
		case counts.adds > 0:
			aliases := append(b.flagsAdded[flagKey], b.flagsRemoved[flagKey]...)
			aliases = uniqueStrs(aliases)
			sort.Strings(aliases)
			added[flagKey] = aliases
		case counts.deletes > 0:
			aliases := uniqueStrs(b.flagsRemoved[flagKey])
			sort.Strings(aliases)
			removed[flagKey] = aliases
			if _, ok := b.flagsFoundAtHead[flagKey]; !ok {
				extinctions[flagKey] = struct{}{}
			}
		}
	}

	summary := ReferenceSummary{
		FlagsAdded:   added,
		FlagsRemoved: removed,
	}

	if b.includeExtinctions {
		summary.ExtinctFlags = extinctions
	}

	return summary
}

// get slice with unique, non-empty strings
func uniqueStrs(s []string) []string {
	if len(s) <= 1 {
		return s
	}
	keys := make(map[string]struct{}, len(s))
	ret := make([]string, 0, len(s))
	for _, elem := range s {
		trimmed := strings.TrimSpace(elem)
		if len(trimmed) == 0 {
			continue
		}
		if _, ok := keys[trimmed]; !ok {
			keys[trimmed] = struct{}{}
			ret = append(ret, trimmed)
		}
	}
	return ret
}
