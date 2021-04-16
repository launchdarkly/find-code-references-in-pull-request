package diff

import (
	"testing"

	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
)

func TestCheckDiff(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		origName string
		newName  string
		skip     bool
	}{
		{
			name:     "basic",
			fileName: "test",
			origName: "a/test",
			newName:  "b/test",
			skip:     false,
		},
		{
			name:     "skip true for dotfiles",
			fileName: ".testignore",
			origName: "a/.testignore",
			newName:  "b/.testignore",
			skip:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hunk := &diff.Hunk{
				NewLines:      1,
				NewStartLine:  1,
				OrigLines:     0,
				OrigStartLine: 0,
				StartPosition: 1,
			}
			diff := diff.FileDiff{
				OrigName: tc.origName,
				NewName:  tc.newName,
				Hunks:    []*diff.Hunk{hunk},
			}
			results := CheckDiff(&diff, "../testdata")
			assert.Equal(t, &DiffPaths{FileToParse: "../testdata/" + tc.fileName, Skip: tc.skip}, results, "")
		})
	}
}
