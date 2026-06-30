// Package version holds the single version string for the north binary.
package version

import "runtime/debug"

// Version is the north binary version. It defaults to "dev" for local builds
// and is overwritten at link time via -ldflags for tagged releases. When
// installed via go install @<tag>, ReadBuildInfo populates it automatically.
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
	}
}
