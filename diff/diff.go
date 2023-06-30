package diff

import (
	"fmt"
	"os"
	"strings"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	lflags "github.com/launchdarkly/cr-flags/flags"
	"github.com/launchdarkly/cr-flags/ignore"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
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

func ProcessDiffs(matcher lsearch.Matcher, hunk *diff.Hunk, flagsRef lflags.FlagsRef, flags ldapi.FeatureFlags, maxFlags int) {
	diffLines := strings.Split(string(hunk.Body), "\n")
	for _, line := range diffLines {
		if flagsRef.Count() >= maxFlags {
			break
		}
		op := operation(line)

		// only one for now
		elementMatcher := matcher.Elements[0]

		for _, flagKey := range elementMatcher.FindMatches(line) {
			if op == Add {
				if _, ok := flagsRef.FlagsAdded[flagKey]; !ok {
					flagsRef.FlagsAdded[flagKey] = lflags.AliasSet{}
				}
			} else if op == Delete {
				if _, ok := flagsRef.FlagsRemoved[flagKey]; !ok {
					flagsRef.FlagsRemoved[flagKey] = lflags.AliasSet{}
				}
			}
			if aliasMatches := matcher.FindAliases(line, flagKey); len(aliasMatches) > 0 {
				if op == Add {
					if _, ok := flagsRef.FlagsAdded[flagKey]; !ok {
						flagsRef.FlagsAdded[flagKey] = lflags.AliasSet{}
					}
					for _, alias := range aliasMatches {
						flagsRef.FlagsAdded[flagKey][alias] = true
					}
				} else if op == Delete {
					if _, ok := flagsRef.FlagsRemoved[flagKey]; !ok {
						flagsRef.FlagsRemoved[flagKey] = lflags.AliasSet{}
					}
					for _, alias := range aliasMatches {
						flagsRef.FlagsRemoved[flagKey][alias] = true
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
