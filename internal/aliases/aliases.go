package aliases

import (
	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"

	lcr "github.com/launchdarkly/find-code-references-in-pull-request/config"
	ldiff "github.com/launchdarkly/find-code-references-in-pull-request/diff"
	"github.com/launchdarkly/find-code-references-in-pull-request/internal/utils"
)

// diff contents is the removed contents from files that are in alias configuration
func GenerateAliases(config *lcr.Config, opts options.Options, flagKeys []string, diffContents ldiff.DiffFileMap) (map[string][]string, error) {
	aliasesByFlagKey, err := aliases.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)
	if err != nil {
		return nil, err
	}

	for _, flag := range flagKeys {
		for _, alias := range getFilepatternAliases(opts.Aliases) {
			aliases, err := aliases.GenerateAliasesFromFilePattern(alias, flag, config.Workspace, diffContents)
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
