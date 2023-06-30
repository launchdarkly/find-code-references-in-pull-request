package diff

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v7"
	"github.com/launchdarkly/cr-flags/config"
	lflags "github.com/launchdarkly/cr-flags/flags"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
)

func ptr(v interface{}) *interface{} { return &v }

func createFlag(key string) ldapi.FeatureFlag {
	variation := int32(0)
	href := "test"
	environment := ldapi.FeatureFlagConfig{
		Site: ldapi.Link{
			Href: &href,
		},
		Fallthrough: ldapi.VariationOrRolloutRep{
			Variation: &variation,
		},
	}
	variationTrue := ldapi.Variation{
		Value: ptr(true),
	}
	variationFalse := ldapi.Variation{
		Value: ptr(false),
	}
	flag := ldapi.FeatureFlag{
		Key:          key,
		Name:         "Sample Flag",
		Kind:         "boolean",
		Environments: map[string]ldapi.FeatureFlagConfig{"production": environment},
		Variations:   []ldapi.Variation{variationTrue, variationFalse},
	}
	return flag
}

type testProcessor struct {
	Flags    ldapi.FeatureFlags
	FlagsRef lflags.FlagsRef
	Config   config.Config
}

func (t testProcessor) flagKeys() []string {
	keys := make([]string, 0, len(t.Flags.Items))
	for _, f := range t.Flags.Items {
		keys = append(keys, f.Key)
	}
	return keys
}

func newProcessFlagAccEnv() *testProcessor {
	flag := createFlag("example-flag")
	flags := ldapi.FeatureFlags{}
	flags.Items = append(flags.Items, flag)
	flagsAdded := make(lflags.FlagAliasMap)
	flagsRemoved := make(lflags.FlagAliasMap)
	flagsRef := lflags.FlagsRef{
		FlagsAdded:   flagsAdded,
		FlagsRemoved: flagsRemoved,
	}

	config := config.Config{
		LdEnvironment: []string{"production"},
		LdInstance:    "https://example.com/",
	}
	return &testProcessor{
		Flags:    flags,
		FlagsRef: flagsRef,
		Config:   config,
	}
}

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

func TestProcessDiffs(t *testing.T) {
	cases := []struct {
		name       string
		sampleBody string
		expected   lflags.FlagsRef
		aliases    map[string][]string
	}{
		{
			name: "add flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": lflags.AliasSet{}},
				FlagsRemoved: lflags.FlagAliasMap{},
			},
			aliases: map[string][]string{},
			sampleBody: `
			+Testing data
+this is for testing
+here is a flag
+example-flag
+
 this is no changes
 in the hunk`,
		},
		{
			name: "remove flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{},
				FlagsRemoved: lflags.FlagAliasMap{"example-flag": lflags.AliasSet{}},
			},
			aliases: map[string][]string{},
			sampleBody: `
			-Testing data
-this is for testing
-here is a flag
-example-flag
-
 this is no changes
 in the hunk`,
		},
		{
			name: "modified flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": lflags.AliasSet{}},
				FlagsRemoved: lflags.FlagAliasMap{"example-flag": lflags.AliasSet{}},
			},
			aliases: map[string][]string{},
			sampleBody: `
			-Testing data
-this is for testing
-here is a flag
-example-flag
-
 this is no changes
 in the hunk
+adding other lines
+example-flag`,
		},
		{
			name: "alias flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": lflags.AliasSet{"exampleFlag": true}},
				FlagsRemoved: lflags.FlagAliasMap{},
			},
			aliases: map[string][]string{"example-flag": []string{"exampleFlag"}},
			sampleBody: `
			+Testing data
+this is for testing
+here is a flag
+exampleFlag
+exampleFlag
+`,
		},
	}

	for _, tc := range cases {
		processor := newProcessFlagAccEnv()
		t.Run(tc.name, func(t *testing.T) {
			hunk := &diff.Hunk{
				NewLines:      1,
				NewStartLine:  1,
				OrigLines:     0,
				OrigStartLine: 0,
				StartPosition: 1,
				Body:          []byte(tc.sampleBody),
			}
			elements := []lsearch.ElementMatcher{}
			elements = append(elements, lsearch.NewElementMatcher("default", "", "", processor.flagKeys(), tc.aliases))
			matcher := lsearch.Matcher{
				Elements: elements,
			}
			ProcessDiffs(matcher, hunk, processor.FlagsRef, processor.Flags, 5)
			assert.Equal(t, tc.expected, processor.FlagsRef)
		})
	}
}
