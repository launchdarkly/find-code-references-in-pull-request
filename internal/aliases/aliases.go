package aliases

import (
	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"

	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils"
)

// Generate aliases, making sure to identify aliases in the removed diff contents
func GenerateAliases(opts options.Options, flagKeys []string, diffContents aliases.FileContentsMap) (map[string][]string, error) {
	aliasesByFlagKey, err := aliases.GenerateAliases(flagKeys, opts.Aliases, opts.Dir)
	if err != nil {
		return nil, err
	}

	filePatternAliases := getFilepatternAliases(opts.Aliases)
	for _, flag := range flagKeys {
		for _, alias := range filePatternAliases {
			aliases, err := aliases.GenerateAliasesFromFilePattern(alias, flag, opts.Dir, diffContents)
			if err != nil {
				// skip aliases that fail to generate
				continue
			}
			aliasesByFlagKey[flag] = append(aliasesByFlagKey[flag], aliases...)
		}
		aliasesByFlagKey[flag] = utils.Dedupe(aliasesByFlagKey[flag])
	}

	return aliasesByFlagKey, nil
}

func getFilepatternAliases(aliases []options.Alias) []options.Alias {
	filePatternAliases := make([]options.Alias, 0, len(aliases))
	for _, alias := range aliases {
		if alias.Type.Canonical() == options.FilePattern {
			filePatternAliases = append(filePatternAliases, alias)
		}
	}

	return filePatternAliases
}
