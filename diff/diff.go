package diff

import (
	"fmt"
	"os"
	"strings"

	ldapi "github.com/launchdarkly/api-client-go"
	ghc "github.com/launchdarkly/cr-flags/comments"
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

func ProcessDiffs(raw *diff.Hunk, flagsRef ghc.FlagsRef, flags ldapi.GlobalFlagCollectionRep, aliases map[string][]string, maxFlags int) {
	diffRows := strings.Split(string(raw.Body), "\n")
	for _, row := range diffRows {

		if (len(flagsRef.FlagsAdded) + len(flagsRef.FlagsRemoved)) >= maxFlags {
			break
		}
		if strings.HasPrefix(row, "+") {
			for _, flag := range flags.Items {
				if strings.Contains(row, flag.Key) {
					currentKeys := flagsRef.FlagsAdded[flag.Key]
					currentKeys = append(currentKeys, "")
					flagsRef.FlagsAdded[flag.Key] = currentKeys
				}
				if len(aliases[flag.Key]) > 0 {
				CheckAliasAdded:
					for _, alias := range aliases[flag.Key] {
						if strings.Contains(row, alias) {
							currentKeys := flagsRef.FlagsAdded[flag.Key]
							for i, _ := range currentKeys {
								if alias == currentKeys[i] {
									// If key already exists we do not want to add it
									continue CheckAliasAdded
								}
							}
							currentKeys = append(currentKeys, alias)
							flagsRef.FlagsAdded[flag.Key] = currentKeys
						}
					}
				}
			}
		} else if strings.HasPrefix(row, "-") {
			for _, flag := range flags.Items {
				if strings.Contains(row, flag.Key) {
					currentKeys := flagsRef.FlagsRemoved[flag.Key]
					currentKeys = append(currentKeys, "")
					flagsRef.FlagsRemoved[flag.Key] = currentKeys
				}
				if len(aliases[flag.Key]) > 0 {
				CheckAliasRemoved:
					for _, alias := range aliases[flag.Key] {
						if strings.Contains(row, alias) {
							currentKeys := flagsRef.FlagsRemoved[flag.Key]
							for i, _ := range currentKeys {
								// If key already exists we do not want to add it
								if alias == currentKeys[i] {
									continue CheckAliasRemoved
								}
							}
							currentKeys = append(currentKeys, alias)
							flagsRef.FlagsRemoved[flag.Key] = currentKeys
						}
					}
				}
			}
		}
	}
}
