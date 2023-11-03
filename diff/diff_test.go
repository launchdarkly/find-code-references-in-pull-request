package diff

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go/v13"
	"github.com/launchdarkly/find-code-references-in-pull-request/config"
	lflags "github.com/launchdarkly/find-code-references-in-pull-request/flags"
	lsearch "github.com/launchdarkly/ld-find-code-refs/v2/search"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
)

func ptr[T any](t T) *T { return &t }

func createFlag(key string) ldapi.FeatureFlag {
	environment := ldapi.FeatureFlagConfig{
		Site: ldapi.Link{
			Href: ptr("test"),
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
	Flags   ldapi.FeatureFlags
	Config  config.Config
	Builder *lflags.ReferenceBuilder
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
	flag2 := createFlag("sample-flag")
	flags := ldapi.FeatureFlags{}
	flags.Items = append(flags.Items, flag)
	flags.Items = append(flags.Items, flag2)
	builder := lflags.NewReferenceBuilder(5)
	config := config.Config{
		LdEnvironment: "production",
		LdInstance:    "https://example.com/",
	}
	return &testProcessor{
		Flags:   flags,
		Config:  config,
		Builder: builder,
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
			filePath, ignore := checkDiffFile(&diff, "../testdata")
			assert.Equal(t, tc.fileName, filePath)
			assert.Equal(t, tc.skip, ignore)
		})
	}
}

func TestProcessDiffs_BuildReferences(t *testing.T) {
	cases := []struct {
		name       string
		sampleBody string
		expected   lflags.FlagsRef
		aliases    map[string][]string
		delimiters string
	}{
		{
			name: "add flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": []string{}},
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
				FlagsRemoved: lflags.FlagAliasMap{"example-flag": []string{}},
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
			name: "add and remove flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"sample-flag": []string{}},
				FlagsRemoved: lflags.FlagAliasMap{"example-flag": []string{}},
			},
			aliases: map[string][]string{},
			sampleBody: `
			-Testing data
-this is for testing
-here is a flag
-example-flag
-
+ sample-flag
 this is no changes
 in the hunk`,
		},
		{
			name: "modified flag",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": []string{}},
				FlagsRemoved: lflags.FlagAliasMap{},
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
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": []string{"exampleFlag"}},
				FlagsRemoved: lflags.FlagAliasMap{},
			},
			aliases: map[string][]string{"example-flag": {"exampleFlag"}},
			sampleBody: `
			+Testing data
+this is for testing
+here is a flag
+exampleFlag
+exampleFlag
+`,
		},
		{
			name: "require delimiters - no matches",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{},
				FlagsRemoved: lflags.FlagAliasMap{},
			},
			delimiters: "'\"",
			aliases:    map[string][]string{},
			sampleBody: `
			+Testing data
+this is for testing
+here is a flag
+example-flag
+`,
		},
		{
			name: "require delimiters - match",
			expected: lflags.FlagsRef{
				FlagsAdded:   lflags.FlagAliasMap{"example-flag": []string{}},
				FlagsRemoved: lflags.FlagAliasMap{},
			},
			delimiters: "'\"",
			aliases:    map[string][]string{},
			sampleBody: `
			+Testing data
+this is for testing
+here is a flag
+"example-flag"
+`,
		},
	}

	for _, tc := range cases {
		processor := newProcessFlagAccEnv()
		t.Run(tc.name, func(t *testing.T) {
			elements := []lsearch.ElementMatcher{}
			elements = append(elements, lsearch.NewElementMatcher("default", "", tc.delimiters, processor.flagKeys(), tc.aliases))
			matcher := lsearch.Matcher{
				Elements: elements,
			}
			ProcessDiffs(matcher, []byte(tc.sampleBody), processor.Builder)
			flagsRef := processor.Builder.Build()
			assert.Equal(t, tc.expected, flagsRef)
		})
	}
}
