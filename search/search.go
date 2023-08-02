package search

import (
	"log"
	"strings"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	"github.com/launchdarkly/ld-find-code-refs/v2/aliases"
	"github.com/launchdarkly/ld-find-code-refs/v2/options"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"

	lcr "github.com/launchdarkly/cr-flags/config"
	"github.com/spf13/viper"
)

func GetMatcher(config *lcr.Config, opts options.Options, flags []ldapi.FeatureFlag) (matcher lsearch.Matcher, err error) {
	flagKeys := make([]string, 0, len(flags))
	for _, flag := range flags {
		flagKeys = append(flagKeys, flag.Key)
	}

	aliasesByFlagKey, err := aliases.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)
	if err != nil {
		return lsearch.Matcher{}, err
	}

	delimiters := strings.Join(Dedupe(getDelimiters(opts)), "")
	elements := []lsearch.ElementMatcher{}
	elements = append(elements, lsearch.NewElementMatcher(config.LdProject, "", delimiters, flagKeys, aliasesByFlagKey))
	matcher = lsearch.Matcher{
		Elements: elements,
	}

	return matcher, nil
}

func getAliases(config *lcr.Config, flagKeys []string) (map[string][]string, error) {
	// Needed for ld-find-code-refs to work as a library
	viper.Set("dir", config.Workspace)
	viper.Set("accessToken", config.ApiToken)

	err := options.InitYAML()
	if err != nil {
		log.Println(err)
	}
	opts, err := options.GetOptions()
	if err != nil {
		log.Println(err)
	}

	return aliases.GenerateAliases(flagKeys, opts.Aliases, config.Workspace)
}

func getDelimiters(opts options.Options) []string {
	delims := []string{`"`, `'`, "`"}
	if opts.Delimiters.DisableDefaults {
		delims = []string{}
	}

	delims = append(delims, opts.Delimiters.Additional...)

	return delims
}

func Dedupe(s []string) []string {
	if len(s) <= 1 {
		return s
	}
	keys := make(map[string]struct{}, len(s))
	ret := make([]string, 0, len(s))
	for _, entry := range s {
		if _, value := keys[entry]; !value {
			keys[entry] = struct{}{}
			ret = append(ret, entry)
		}
	}
	return ret
}
