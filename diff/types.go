package diff

type DiffPaths struct {
	FileToParse string
	Skip        bool
}

// Maps file path to diff content
type DiffFileMap map[string][]byte
