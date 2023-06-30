package flags

type FlagAliasMap = map[string]AliasSet

type AliasSet = map[string]bool

type FlagsRef struct {
	FlagsAdded   FlagAliasMap
	FlagsRemoved FlagAliasMap
}

func (fr FlagsRef) Found() bool {
	return fr.Count() > 0
}

func (fr FlagsRef) Count() int {
	return len(fr.FlagsAdded) + len(fr.FlagsRemoved)
}
