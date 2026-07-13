// Package version reports the build's version.
package version

import "runtime/debug"

// Version is set at release time via -ldflags.
var Version = ""

// String returns the version.
//
// When the binary was not built by the release pipeline — someone ran
// `go install ...@latest`, or built from a checkout — there is no ldflags value
// to read. Go records the module version in the build info in that case, so use
// it rather than reporting an unhelpful "dev" for a binary that came from a real
// tagged release.
func String() string {
	if Version != "" {
		return Version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}
