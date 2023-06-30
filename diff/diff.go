package diff

import (
	"fmt"
	"os"
	"strings"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	lflags "github.com/launchdarkly/cr-flags/flags"
	"github.com/launchdarkly/cr-flags/ignore"
	"github.com/sourcegraph/go-diff/diff"
)

type DiffPaths struct {
	FileToParse string
	Skip        bool
}

func CheckDiff(parsedDiff *diff.FileDiff, workspace string) *DiffPaths {
	diffPaths := DiffPaths{}
	allIgnores := ignore.NewIgnore(workspace)

	// If file is being renamed we don't want to check it for flags.
	parsedFileA := strings.SplitN(parsedDiff.OrigName, "/", 2)
	parsedFileB := strings.SplitN(parsedDiff.NewName, "/", 2)
	fullPathToA := workspace + "/" + parsedFileA[1]
	fullPathToB := workspace + "/" + parsedFileB[1]
	info, err := os.Stat(fullPathToB)
	var isDir bool
	// If there is no 'b' parse 'a', means file is deleted.
	if info == nil {
		isDir = false
		diffPaths.FileToParse = fullPathToA
	} else {
		isDir = info.IsDir()
		diffPaths.FileToParse = fullPathToB
	}
	if err != nil {
		fmt.Println(err)
	}
	// Similar to ld-find-code-refs do not match dotfiles, and read in ignore files.
	if strings.HasPrefix(parsedFileB[1], ".") && strings.HasPrefix(parsedFileA[1], ".") || allIgnores.Match(diffPaths.FileToParse, isDir) {
		diffPaths.Skip = true
	}
	// We don't want to run on renaming of files.
	if (parsedFileA[1] != parsedFileB[1]) && (!strings.Contains(parsedFileB[1], "dev/null") && !strings.Contains(parsedFileA[1], "dev/null")) {
		diffPaths.Skip = true
	}

	return &diffPaths
}

func ProcessDiffs(hunk *diff.Hunk, flagsRef lflags.FlagsRef, flags ldapi.FeatureFlags, aliases map[string][]string, maxFlags int) {
	diffRows := strings.Split(string(hunk.Body), "\n")
	for _, row := range diffRows {
		if flagsRef.Count() >= maxFlags {
			break
		}
		op := operation(row)
		for _, flag := range flags.Items {
			if strings.Contains(row, flag.Key) {
				if op == Add {
					if _, ok := flagsRef.FlagsAdded[flag.Key]; !ok {
						flagsRef.FlagsAdded[flag.Key] = lflags.AliasSet{}
					}
				} else if op == Delete {
					if _, ok := flagsRef.FlagsRemoved[flag.Key]; !ok {
						flagsRef.FlagsRemoved[flag.Key] = lflags.AliasSet{}
					}
				}
			}
			if len(aliases[flag.Key]) > 0 {
				for _, alias := range aliases[flag.Key] {
					if strings.Contains(row, alias) {
						if op == Add {
							if _, ok := flagsRef.FlagsAdded[flag.Key]; !ok {
								flagsRef.FlagsAdded[flag.Key] = lflags.AliasSet{}
							}
							flagsRef.FlagsAdded[flag.Key][alias] = true
						} else if op == Delete {
							if _, ok := flagsRef.FlagsRemoved[flag.Key]; !ok {
								flagsRef.FlagsRemoved[flag.Key] = lflags.AliasSet{}
							}
							flagsRef.FlagsRemoved[flag.Key][alias] = true
						}
					}
				}
			}
		}
	}
}

// Operation defines the operation of a diff item.
type Operation int

const (
	// Equal item represents an equals diff.
	Equal Operation = iota
	// Add item represents an insert diff.
	Add
	// Delete item represents a delete diff.
	Delete
)

func operation(row string) Operation {
	if strings.HasPrefix(row, "+") {
		return Add
	}
	if strings.HasPrefix(row, "-") {
		return Delete
	}

	return Equal
}
