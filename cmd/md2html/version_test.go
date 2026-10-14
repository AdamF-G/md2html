package main

import (
	"runtime/debug"
	"testing"
)

func TestDescribeBuild(t *testing.T) {
	vcs := func(rev, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.modified", Value: modified},
		}
	}
	const rev = "db78845788095cc708266952ecbc717f7e1c4440"
	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"no build info", nil, "v0.9.0"},
		{"go install at the tag",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.0"}},
			"v0.9.0"},
		{"clean checkout of the tag",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.0"}, Settings: vcs(rev, "false")},
			"v0.9.0"},
		{"checkout past the tag",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.1-0.20261014050324-db7884578809"}, Settings: vcs(rev, "false")},
			"v0.9.0 (db7884578809)"},
		{"dirty checkout of the tag",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.0+dirty"}, Settings: vcs(rev, "true")},
			"v0.9.0 (db7884578809, modified)"},
		{"go install at a commit",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.1-0.20261014050324-db7884578809"}},
			"v0.9.0 (v0.9.1-0.20261014050324-db7884578809)"},
		{"devel build with no vcs",
			&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			"v0.9.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeBuild("v0.9.0", tc.info); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
