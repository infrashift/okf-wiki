// Package site builds and serves a static HTML site of an OKF bundle with
// Astro and Starlight, run by bun.
//
// okfw itself stays a pure-Go binary. bun is needed by `okfw site` and by
// nothing else, so it is checked here, before anything is written, and every
// call to it lives in this package (TestOnlySiteRunsBun).
package site

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MinBunVersion is the oldest bun that can install the embedded template:
// its bun.lock is the text lockfile, which bun 1.2 introduced.
var MinBunVersion = Version{1, 2, 0}

// BunEnv names the environment variable that points okfw at a bun binary.
const BunEnv = "OKFW_BUN"

// versionTimeout bounds `bun --version`, so a hung shim fails instead of
// blocking the command forever.
var versionTimeout = 5 * time.Second

// InstallHint is how a person gets bun. An agent cannot fix a missing bun on
// its own, so every preflight error ends with it.
const InstallHint = "Install bun 1.2 or later (https://bun.sh), or point okfw at one with --bun <path> or " + BunEnv + "=<path>."

// Version is a bun release.
type Version struct{ Major, Minor, Patch int }

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Less reports whether v is older than w.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

var versionRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.+-]*)?$`)

// ParseVersion reads `bun --version` output strictly: one semver line.
func ParseVersion(out string) (Version, error) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return Version{}, fmt.Errorf("unrecognised version output %q", strings.TrimSpace(out))
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	return v, nil
}

// Bun is a bun binary that passed preflight.
type Bun struct {
	Path    string
	Version Version
	Source  string // "--bun", OKFW_BUN or PATH: where the path came from
}

// PreflightError is a failed preflight. Each kind says what was found, what
// is required, and how to fix it.
type PreflightError struct {
	Kind   string // not-found, not-executable, no-run, timeout, bad-output, too-old
	Path   string
	Detail string
}

func (e *PreflightError) Error() string {
	where := ""
	if e.Path != "" {
		where = " (" + e.Path + ")"
	}
	return fmt.Sprintf("okfw site needs bun: %s%s. %s", e.Detail, where, InstallHint)
}

// Preflight finds a usable bun before okfw site does anything. flagPath is the
// --bun flag; it wins over OKFW_BUN, which wins over PATH. An explicit path
// that does not work is an error: it never quietly falls back to PATH, which
// would build with a bun the user did not choose.
func Preflight(flagPath string) (*Bun, error) {
	path, source := flagPath, "--bun"
	if path == "" {
		path, source = os.Getenv(BunEnv), BunEnv
	}
	if path == "" {
		found, err := exec.LookPath("bun")
		if err != nil {
			return nil, &PreflightError{Kind: "not-found", Detail: "no bun on PATH"}
		}
		path, source = found, "PATH"
	} else {
		fi, err := os.Stat(path)
		switch {
		case err != nil:
			return nil, &PreflightError{Kind: "not-found", Path: path, Detail: source + " names no file"}
		case !fi.Mode().IsRegular():
			return nil, &PreflightError{Kind: "not-executable", Path: path, Detail: source + " is not a regular file"}
		case fi.Mode().Perm()&0o111 == 0:
			return nil, &PreflightError{Kind: "not-executable", Path: path, Detail: source + " is not executable"}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	// A shim that forks a child holding the output pipe would otherwise keep
	// Output waiting after the timeout killed the shim itself.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, &PreflightError{Kind: "timeout", Path: path,
			Detail: fmt.Sprintf("`bun --version` did not answer within %s", versionTimeout)}
	}
	if err != nil {
		detail := "`bun --version` failed: " + err.Error()
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			detail += ": " + strings.TrimSpace(string(ee.Stderr))
		}
		return nil, &PreflightError{Kind: "no-run", Path: path, Detail: detail}
	}
	v, err := ParseVersion(string(out))
	if err != nil {
		return nil, &PreflightError{Kind: "bad-output", Path: path, Detail: "`bun --version` gave " + err.Error()}
	}
	if v.Less(MinBunVersion) {
		return nil, &PreflightError{Kind: "too-old", Path: path,
			Detail: fmt.Sprintf("found bun %s, need %s or later", v, MinBunVersion)}
	}
	return &Bun{Path: path, Version: v, Source: source}, nil
}
