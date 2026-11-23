package config

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// version is stamped by `make` from the git tag (-ldflags -X
// github.com/infrashift/okf-wiki/internal/config.version=...). Empty in a
// plain `go build`.
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
// bundle-root index.md, which OKF v0.3 section 8.1 makes the only place an
// okf_version key is permitted.
const OKFVersion = "0.3"

// DeclaredOKFVersion returns the okf_version a bundle-root index.md's content
// declares, or "".
func DeclaredOKFVersion(content []byte) string {
	fm, err := ExtractFrontmatter(content)
	if err != nil {
		return ""
	}
	var v struct {
		OKFVersion string `yaml:"okf_version"`
	}
	_ = yaml.Unmarshal(fm, &v)
	return strings.TrimSpace(v.OKFVersion)
}

// ReadDeclaredOKFVersion reads the okf_version declared by the index.md at a
// bundle root, or "" when there is none.
func ReadDeclaredOKFVersion(bundleRoot string) string {
	data, err := os.ReadFile(filepath.Join(bundleRoot, "index.md"))
	if err != nil {
		return ""
	}
	return DeclaredOKFVersion(data)
}

// LegacyPathFallback reports whether a bundle declaring this okf_version gets
// the v0.2 compatibility reading of path fields (OKF v0.3 sections 6.2 and
// 12.1): a bare value that does not resolve from its file's directory MAY be
// tried from the bundle root. That applies to a bundle that declares 0.2 or
// earlier, or no version at all — and every bundle okfw wrote before v0.3
// declared 0.2 and spelled such paths from the root.
func LegacyPathFallback(declared string) bool {
	if declared == "" {
		return true
	}
	major, minor, ok := parseMajorMinor(declared)
	if !ok {
		return false // a version this reader cannot read is treated as newer
	}
	return major == 0 && minor <= 2
}

func parseMajorMinor(v string) (int, int, bool) {
	a, b, ok := strings.Cut(strings.TrimSpace(v), ".")
	if !ok {
		return 0, 0, false
	}
	if i := strings.IndexByte(b, '.'); i >= 0 {
		b = b[:i]
	}
	major, err1 := strconv.Atoi(a)
	minor, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}
