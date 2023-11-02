package diff

import (
	"fmt"
	"os"
	"strings"

	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	"github.com/launchdarkly/find-code-references-in-pull-request/ignore"
	diff_util "github.com/launchdarkly/find-code-references-in-pull-request/internal/util/diff_util"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
	"github.com/sourcegraph/go-diff/diff"
)

func PreprocessDiffs(dir string, multiFiles []*diff.FileDiff) DiffFileMap {
	diffMap := make(map[string][]byte, len(multiFiles))

	for _, parsedDiff := range multiFiles {
		getPath := CheckDiff(parsedDiff, dir)
		if getPath.Skip {
			continue
		}

		_, ok := diffMap[getPath.FileToParse]
		if !ok {
			diffMap[getPath.FileToParse] = make([]byte, 0)
		}
		for _, hunk := range parsedDiff.Hunks {
			diffMap[getPath.FileToParse] = append(diffMap[getPath.FileToParse], hunk.Body...)
		}
	}

	return diffMap
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

func ProcessDiffs(matcher lsearch.Matcher, contents []byte, builder *lflags.ReferenceBuilder) {
	diffLines := strings.Split(string(contents), "\n")
	for _, line := range diffLines {
		op := diff_util.LineOperation(line)
		if op == diff_util.OperationEqual {
			continue
		}

		// only one for now
		elementMatcher := matcher.Elements[0]
		for _, flagKey := range elementMatcher.FindMatches(line) {
			aliasMatches := elementMatcher.FindAliases(line, flagKey)
			builder.AddReference(flagKey, op, aliasMatches)
		}
		if builder.MaxReferences() {
			break
		}
	}
}
