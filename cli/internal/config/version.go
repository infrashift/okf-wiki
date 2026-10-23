package config

import "runtime/debug"

// version is stamped by `make` from the git tag (-ldflags -X
// okfw/cli/internal/config.version=...). Empty in a plain `go build`.
var version string

// Version is okfw's own version, printed by `okfw --version` and reported by
// the MCP server's initialize response. Releases are CalVer tags, vYYYY.M.N
// (no leading zeros, so they are also valid semver). Unstamped, it falls back
// to what Go recorded from VCS — a pseudo-version naming the commit — so a
// binary built without make still says where it came from.
func Version() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "devel"
}

// OKFVersion is the Open Knowledge Format revision okfw targets. Declared in the
// bundle-root index.md, which OKF v0.2 section 12 makes the only place an
// okf_version key is permitted.
const OKFVersion = "0.2"
