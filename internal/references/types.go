package flags

import "sort"

type FlagAliasMap = map[string][]string

type ReferenceSummary struct {
	FlagsAdded   FlagAliasMap
	FlagsRemoved FlagAliasMap
	ExtinctFlags map[string]struct{}
}

func (fr ReferenceSummary) Found() bool {
	return fr.Count() > 0
}

func (fr ReferenceSummary) Count() int {
	return len(fr.FlagsAdded) + len(fr.FlagsRemoved)
}

// returns a sorted list of all added flag keys
func (fr ReferenceSummary) AddedKeys() []string {
	return fr.sortedKeys(fr.FlagsAdded)
}

// returns a sorted list of all removed flag keys
func (fr ReferenceSummary) RemovedKeys() []string {
	return fr.sortedKeys(fr.FlagsRemoved)
}

func (fr ReferenceSummary) sortedKeys(keys map[string][]string) []string {
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	// sort keys so hashing can work for checking if comment already exists
	sort.Strings(sortedKeys)
	return sortedKeys
}
