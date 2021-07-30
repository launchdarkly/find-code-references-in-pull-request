package diff

import (
	"testing"

	ldapi "github.com/launchdarkly/api-client-go"
	"github.com/launchdarkly/cr-flags/comments"
	"github.com/launchdarkly/cr-flags/config"
	"github.com/sourcegraph/go-diff/diff"
	"github.com/stretchr/testify/assert"
)

func ptr(v interface{}) *interface{} { return &v }

func createFlag(key string) ldapi.GlobalFlagRep {
	testVal := "test"
	testVar := int32(0)
	environment := ldapi.FlagConfigurationRep{
		Site: ldapi.CoreLink{
			Href: &testVal,
		},
		Fallthrough: ldapi.VariationOrRolloutRep{
			Variation: &testVar,
		},
	}
	variationTrue := ldapi.VariateRep{
		Value: ptr(true),
	}
	variationFalse := ldapi.VariateRep{
		Value: ptr(false),
	}
	flag := ldapi.GlobalFlagRep{
		Key:          key,
		Name:         "Sample Flag",
		Kind:         "boolean",
		Environments: map[string]ldapi.FlagConfigurationRep{"production": environment},
		Variations:   []ldapi.VariateRep{variationTrue, variationFalse},
	}
	return flag
}

type testProcessor struct {
	Flags    ldapi.GlobalFlagCollectionRep
	FlagsRef comments.FlagsRef
	Config   config.Config
}

func newProcessFlagAccEnv() *testProcessor {
	flag := createFlag("example-flag")
	flags := ldapi.GlobalFlagCollectionRep{}
	flags.Items = append(flags.Items, flag)
	flagsAdded := make(map[string][]string)
	flagsRemoved := make(map[string][]string)
	flagsRef := comments.FlagsRef{
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
		expected   comments.FlagsRef
		aliases    map[string][]string
	}{
		{
			name: "add flag",
			expected: comments.FlagsRef{
				FlagsAdded:   map[string][]string{"example-flag": []string{""}},
				FlagsRemoved: map[string][]string{},
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
			expected: comments.FlagsRef{
				FlagsRemoved: map[string][]string{"example-flag": []string{""}},
				FlagsAdded:   map[string][]string{},
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
			expected: comments.FlagsRef{
				FlagsAdded:   map[string][]string{"example-flag": []string{""}},
				FlagsRemoved: map[string][]string{"example-flag": []string{""}},
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
			expected: comments.FlagsRef{
				FlagsAdded:   map[string][]string{"example-flag": []string{"exampleFlag"}},
				FlagsRemoved: map[string][]string{},
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
			ProcessDiffs(hunk, processor.FlagsRef, processor.Flags, tc.aliases, 5)
			assert.Equal(t, tc.expected, processor.FlagsRef, "")
		})
	}
}
