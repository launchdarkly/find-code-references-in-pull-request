package flags

import (
	"sort"
	"strings"
)

type ReferenceBuilder struct {
	max          int // maximum number of flags to find
	flagsAdded   map[string][]string
	flagsRemoved map[string][]string
	foundFlags   map[string]struct{}
}

func NewReferenceBuilder(max int) *ReferenceBuilder {
	return &ReferenceBuilder{
		flagsAdded:   make(map[string][]string),
		flagsRemoved: make(map[string][]string),
		foundFlags:   make(map[string]struct{}),
	}
}

func (b *ReferenceBuilder) MaxReferences() bool {
	return len(b.foundFlags) >= b.max
}

func (b *ReferenceBuilder) AddReference(flagKey string, op string, aliases []string) {
	if op == "+" {
		b.AddedFlag(flagKey, aliases)
	} else if op == "-" {
		b.RemovedFlag(flagKey, aliases)
	}
	// ignore
}

func (b *ReferenceBuilder) AddedFlag(flagKey string, aliases []string) {
	b.foundFlags[flagKey] = struct{}{}
	if _, ok := b.flagsAdded[flagKey]; !ok {
		b.flagsAdded[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsAdded[flagKey] = append(b.flagsAdded[flagKey], aliases...)
}

func (b *ReferenceBuilder) RemovedFlag(flagKey string, aliases []string) {
	b.foundFlags[flagKey] = struct{}{}
	if _, ok := b.flagsRemoved[flagKey]; !ok {
		b.flagsRemoved[flagKey] = make([]string, 0, len(aliases))
	}
	b.flagsRemoved[flagKey] = append(b.flagsRemoved[flagKey], aliases...)
}

func (b *ReferenceBuilder) Build() FlagsRef {
	added := make(map[string][]string, len(b.flagsAdded))
	removed := make(map[string][]string, len(b.flagsRemoved))

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
		}
	}

	return FlagsRef{
		FlagsAdded:   added,
		FlagsRemoved: removed,
	}
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
