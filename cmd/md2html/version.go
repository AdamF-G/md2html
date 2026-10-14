package main

import (
	"runtime/debug"

	"github.com/AdamF-G/md2html"
)

// versionString is what --version prints. The constant alone cannot tell
// two builds apart: every commit between two tags carries the same one,
// and a dirty tree carries it too. So a build that is not exactly the
// tagged release adds what the Go toolchain recorded about its source,
// with the constant still first, so it matches the provenance marker.
func versionString() string {
	info, _ := debug.ReadBuildInfo()
	return describeBuild(md2html.Version, info)
}

// describeBuild is versionString with the build info passed in, for tests.
//
// A clean build at the tag needs nothing more: go install @v0.9.0, or a
// clean checkout of the tag, records the module version as the tag itself.
// Otherwise a build from a checkout records its commit and whether the
// tree had changes, and a go install @commit records a pseudo-version,
// which names the commit itself.
func describeBuild(version string, info *debug.BuildInfo) string {
	if info == nil || info.Main.Version == version {
		return version
	}
	var rev string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev != "" {
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if modified {
			return version + " (" + rev + ", modified)"
		}
		return version + " (" + rev + ")"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return version + " (" + v + ")"
	}
	return version
}
