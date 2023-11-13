package flags

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils/diff_util"
)

type ReferenceSummaryBuilder struct {
	max                int  // maximum number of flags to find
	includeExtinctions bool // include extinctions in summary
	flagsAdded         map[string][]string
	flagsRemoved       map[string][]string
	flagsFoundAtHead   map[string]struct{}
	foundFlags         map[string]struct{}
}

func NewReferenceSummaryBuilder(max int, includeExtinctions bool) *ReferenceSummaryBuilder {
	return &ReferenceSummaryBuilder{
		flagsAdded:         make(map[string][]string),
		flagsRemoved:       make(map[string][]string),
		foundFlags:         make(map[string]struct{}),
		flagsFoundAtHead:   make(map[string]struct{}),
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
	if _, ok := b.flagsAdded[flagKey]; !ok {
		b.flagsAdded[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsAdded[flagKey] = append(b.flagsAdded[flagKey], aliases...)
}

// Flag and aliases found in removed diff
func (b *ReferenceSummaryBuilder) removedFlag(flagKey string, aliases []string) {
	b.foundFlag(flagKey)
	if _, ok := b.flagsRemoved[flagKey]; !ok {
		b.flagsRemoved[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsRemoved[flagKey] = append(b.flagsRemoved[flagKey], aliases...)
}

// Returns a list of removed flag keys
func (b *ReferenceSummaryBuilder) RemovedFlagKeys() []string {
	keys := make([]string, 0, len(b.flagsRemoved))
	for k := range b.flagsRemoved {
		keys = append(keys, k)
	}
	return keys
}

func (b *ReferenceSummaryBuilder) Build() ReferenceSummary {
	added := make(map[string][]string, len(b.flagsAdded))
	removed := make(map[string][]string, len(b.flagsRemoved))
	extinctions := make(map[string]struct{}, len(b.flagsRemoved))

	for flagKey := range b.foundFlags {
		if aliases, ok := b.flagsAdded[flagKey]; ok {
			// if there are any removed aliases, we should add them
			aliases := append(aliases, b.flagsRemoved[flagKey]...)
			aliases = uniqueStrs(aliases)
			sort.Strings(aliases)
			added[flagKey] = aliases
		} else if aliases, ok := b.flagsRemoved[flagKey]; ok {
			// only add to removed if it wasn't also added
			aliases := uniqueStrs(aliases)
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
