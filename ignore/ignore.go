package ignore

import (
	"path/filepath"

	"github.com/monochromegane/go-gitignore"
)

type Ignore struct {
	path    string
	ignores []gitignore.IgnoreMatcher
}

func NewIgnore(path string) Ignore {
	ignoreFiles := []string{".gitignore", ".ignore", ".ldignore"}
	ignores := make([]gitignore.IgnoreMatcher, 0, len(ignoreFiles))
	for _, ignoreFile := range ignoreFiles {
		i, err := gitignore.NewGitIgnore(filepath.Join(path, ignoreFile))
		if err != nil {
			continue
		}
		ignores = append(ignores, i)
	}
	return Ignore{path: path, ignores: ignores}
}

func (m Ignore) Match(path string, isDir bool) bool {
	for _, i := range m.ignores {
		if i.Match(path, isDir) {
			return true
		}
	}

	return false
}
