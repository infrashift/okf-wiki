// Package gitctx locates the git repository root so `okfw init` can place the
// wiki at the project root. It requires nothing from the user beyond having
// run the command somewhere inside a repo; outside git, callers fall back to
// the current directory.
package gitctx

import (
	"os/exec"
	"strings"
)

// GetGitRoot returns the absolute path of the git repository containing cwd,
// or empty when not inside a git repo.
func GetGitRoot(cwd string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
