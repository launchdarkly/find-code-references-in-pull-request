package diff

import (
	"fmt"
	"os"
	"strings"

	i "github.com/launchdarkly/find-code-references-in-pull-request/ignore"
	gha "github.com/launchdarkly/find-code-references-in-pull-request/internal/github_actions"
	refs "github.com/launchdarkly/find-code-references-in-pull-request/internal/references"
	diff_util "github.com/launchdarkly/find-code-references-in-pull-request/internal/utils/diff_util"
	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
	"github.com/sourcegraph/go-diff/diff"
)

func PreprocessDiffs(dir string, multiFiles []*diff.FileDiff) aliases.FileContentsMap {
	diffMap := make(map[string][]byte, len(multiFiles))

	for _, parsedDiff := range multiFiles {
		filePath, ignore := checkDiffFile(parsedDiff, dir)
		if ignore {
			continue
		}

		if _, ok := diffMap[filePath]; !ok {
			diffMap[filePath] = make([]byte, 0)
		}

		for _, hunk := range parsedDiff.Hunks {
			diffMap[filePath] = append(diffMap[filePath], hunk.Body...)
		}
	}

	return diffMap
}

func checkDiffFile(parsedDiff *diff.FileDiff, workspace string) (filePath string, ignore bool) {
	allIgnores := i.NewIgnore(workspace)

	// If file is being renamed we don't want to check it for flags.
	parsedFileA := strings.SplitN(parsedDiff.OrigName, "/", 2)
	parsedFileB := strings.SplitN(parsedDiff.NewName, "/", 2)
	fullPathToA := workspace + "/" + parsedFileA[1]
	fullPathToB := workspace + "/" + parsedFileB[1]
	info, err := os.Stat(fullPathToB)
	if err != nil {
		fmt.Println(err)
	}
	var isDir bool
	// If there is no 'b' parse 'a', means file is deleted.
	if info == nil {
		isDir = false
		filePath = fullPathToA
	} else {
		isDir = info.IsDir()
		filePath = fullPathToB
	}
	// Similar to ld-find-code-refs do not match dotfiles, and read in ignore files.
	if strings.HasPrefix(parsedFileB[1], ".") && strings.HasPrefix(parsedFileA[1], ".") || allIgnores.Match(filePath, isDir) {
		return filePath, true
	}
	// We don't want to run on renaming of files.
	if (parsedFileA[1] != parsedFileB[1]) && (!strings.Contains(parsedFileB[1], "dev/null") && !strings.Contains(parsedFileA[1], "dev/null")) {
		return filePath, true
	}

	return filePath, false
}

func ProcessDiffs(matcher lsearch.Matcher, contents []byte, builder *refs.ReferenceSummaryBuilder) {
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
			gha.LogDebug("Found (%s) reference to flag %s with aliases %v", op, flagKey, aliasMatches)
			builder.AddReference(flagKey, op, aliasMatches)
		}
		if builder.MaxReferences() {
			break
		}
	}
}
