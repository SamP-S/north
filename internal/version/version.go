// Package version holds the single version string for the north binary.
package version

import "runtime/debug"

// Version is the north binary version. It defaults to "dev" for local builds
// and is overwritten at link time via -ldflags for tagged releases. When
// installed via go install @<tag>, ReadBuildInfo populates it automatically;
// for local VCS builds the commit revision is used so that skill installs are
// pinned to a real build (`north skill check` compares this stamp).
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = v
		return
	}
	if v := vcsVersion(info); v != "" {
		Version = v
	}
}

// vcsVersion derives "dev-<short-rev>[+dirty]" from the build's VCS metadata,
// or "" when none is recorded (e.g. `go build` outside a checkout).
func vcsVersion(info *debug.BuildInfo) string {
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	v := "dev-" + rev
	if dirty {
		v += "+dirty"
	}
	return v
}
