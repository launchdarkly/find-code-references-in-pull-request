package flags

import "sort"

type FlagAliasMap = map[string][]string

type ReferenceSummary struct {
	FlagsAdded   FlagAliasMap
	FlagsRemoved FlagAliasMap
	ExtinctFlags map[string]struct{}
}

func (fr ReferenceSummary) AnyFound() bool {
	return len(fr.FlagsAdded)+len(fr.FlagsRemoved) > 0
}

// returns a sorted list of all added flag keys
func (fr ReferenceSummary) AddedKeys() []string {
	return fr.sortedKeys(fr.FlagsAdded)
}

// returns a sorted list of all removed flag keys
func (fr ReferenceSummary) RemovedKeys() []string {
	return fr.sortedKeys(fr.FlagsRemoved)
}

// returns a sorted list of all extinct flag keys
func (fr ReferenceSummary) ExtinctKeys() []string {
	if fr.ExtinctFlags == nil {
		return nil
	}
	keys := make([]string, 0, len(fr.ExtinctFlags))
	for k := range fr.ExtinctFlags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (fr ReferenceSummary) IsExtinct(key string) bool {
	_, ok := fr.ExtinctFlags[key]
	return ok
}

func (fr ReferenceSummary) sortedKeys(keys map[string][]string) []string {
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	return sortedKeys
}
