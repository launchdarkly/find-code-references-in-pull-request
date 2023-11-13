package flags

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
