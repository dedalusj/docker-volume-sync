package syncer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/stretchr/testify/require"
)

func TestBuildFilterRules(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		include []string
		want    []string
		wantErr bool
	}{
		{
			name: "NoPatternsProducesNoRules",
			want: []string{},
		},
		{
			name:    "ExcludesOnlyHaveNoCatchAll",
			exclude: []string{"*.log", "cache/**"},
			want:    []string{"- *.log", "- cache/**"},
		},
		{
			name:    "IncludesOnlyAppendCatchAll",
			include: []string{"data/**"},
			want:    []string{"+ data/**", "- /**"},
		},
		{
			name:    "ExcludesComeBeforeIncludes",
			exclude: []string{"*.log"},
			include: []string{"data/**"},
			want:    []string{"- *.log", "+ data/**", "- /**"},
		},
		{
			name:    "BraceAlternationIsAValidPattern",
			exclude: []string{"*.{jpg,png}"},
			want:    []string{"- *.{jpg,png}"},
		},
		{
			name:    "MalformedExcludeIsRejected",
			exclude: []string{"*.{jpg"},
			wantErr: true,
		},
		{
			name:    "MalformedIncludeIsRejected",
			include: []string{"data/{a"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildFilterRules(tt.exclude, tt.include)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)

			// Whatever we produce must be accepted by rclone itself.
			f := filter.Opt
			f.FilterRule = got
			_, err = filter.NewFilter(&f)
			require.NoError(t, err, "rclone should accept the generated rules")
		})
	}
}

func TestBuildFilterRules_ErrorNamesThePattern(t *testing.T) {
	_, err := BuildFilterRules([]string{"*.log", "bad{"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad{")
}

// syncWithPatterns syncs a populated source tree through the given filters and
// returns the relative paths that reached the destination.
func syncWithPatterns(t *testing.T, tree []string, exclude, include []string) []string {
	t.Helper()

	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	require.NoError(t, os.MkdirAll(dstDir, 0755))

	for _, path := range tree {
		full := filepath.Join(srcDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte("x"), 0644))
	}

	rules, err := BuildFilterRules(exclude, include)
	require.NoError(t, err)

	f := filter.Opt
	f.MinAge = fs.DurationOff
	f.MaxAge = fs.DurationOff
	f.FilterRule = append([]string{"- .volumesync_done"}, rules...)

	s, err := New(context.Background(), WithFilterOpt(f))
	require.NoError(t, err)
	require.NoError(t, s.Sync(context.Background(), srcDir, dstDir))

	var got []string
	require.NoError(t, filepath.Walk(dstDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dstDir, p)
		if err != nil {
			return err
		}
		got = append(got, rel)
		return nil
	}))
	sort.Strings(got)
	return got
}

func TestSync_Filters(t *testing.T) {
	tree := []string{
		"app.db",
		"app.log",
		"data/app.db",
		"data/app.log",
		"data/deep/nested.db",
		"cache/blob",
		"cache/deep/blob",
		".volumesync_done",
	}

	tests := []struct {
		name    string
		exclude []string
		include []string
		want    []string
	}{
		{
			name: "NoFiltersCopiesEverythingButTheSentinel",
			want: []string{
				"app.db", "app.log",
				"cache/blob", "cache/deep/blob",
				"data/app.db", "data/app.log", "data/deep/nested.db",
			},
		},
		{
			name:    "ExcludePatternsAndFolders",
			exclude: []string{"*.log", "cache/**"},
			want:    []string{"app.db", "data/app.db", "data/deep/nested.db"},
		},
		{
			name:    "IncludeRestrictsToASubtree",
			include: []string{"data/**"},
			want:    []string{"data/app.db", "data/app.log", "data/deep/nested.db"},
		},
		{
			// The precedence decision: an exclude must beat an include.
			name:    "ExcludeBeatsInclude",
			exclude: []string{"*.log"},
			include: []string{"data/**"},
			want:    []string{"data/app.db", "data/deep/nested.db"},
		},
		{
			// A bare file glob must still recurse: rclone's Filter.Add adds
			// directory globs for include rules so the catch-all doesn't cut
			// traversal off at the root.
			name:    "BareIncludeGlobStillRecurses",
			include: []string{"*.db"},
			want:    []string{"app.db", "data/app.db", "data/deep/nested.db"},
		},
		{
			// The sentinel must stay excluded even when an include would match it.
			name:    "SentinelStaysExcluded",
			include: []string{"**"},
			want: []string{
				"app.db", "app.log",
				"cache/blob", "cache/deep/blob",
				"data/app.db", "data/app.log", "data/deep/nested.db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syncWithPatterns(t, tree, tt.exclude, tt.include)
			require.Equal(t, tt.want, got)
		})
	}
}
