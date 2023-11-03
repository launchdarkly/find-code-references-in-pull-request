package flags

type FlagAliasMap = map[string][]string

type FlagsRef struct {
	FlagsAdded   FlagAliasMap
	FlagsRemoved FlagAliasMap
	ExtinctFlags map[string]struct{}
}

func (fr FlagsRef) Found() bool {
	return fr.Count() > 0
}

func (fr FlagsRef) Count() int {
	return len(fr.FlagsAdded) + len(fr.FlagsRemoved)
}
