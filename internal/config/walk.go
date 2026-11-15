package config

import (
	"path"
	"strings"
)

// The rules for which files of a bundle are concepts. Every walk applies
// them: the CLI's walks over the disk, through bundle.SkipWalkDir, and the
// fs.FS walk of the public pkg/bundle, so a consumer of that package sees the
// concepts the CLI sees.

// EphemeralDir is okfw's export scratch directory, directly under the bundle
// root. It is the only directory okfw generates, and walks skip it.
const EphemeralDir = "ephemeral"

// IsReservedBase reports whether name is an OKF reserved filename (index.md or
// log.md). Neither carries concept frontmatter.
func IsReservedBase(name string) bool {
	return name == "index.md" || name == "log.md"
}

// IsConceptFile reports whether a file of this base name can hold a concept: a
// .md file that is not reserved.
func IsConceptFile(name string) bool {
	return strings.HasSuffix(name, ".md") && !IsReservedBase(name)
}

// SkipBundleDir reports whether a bundle walk skips the directory at rel, a
// slash-separated path relative to the bundle root ("." is the root). It skips
// hidden directories and the ephemeral export scratch area, but never the root
// itself, which is the dot-directory .okfw.
//
// ephemeral/ is okfw's own directory at the bundle root only. An "ephemeral"
// directory deeper down is an ordinary part of the bundle, and skipping it at
// any depth hid its concepts from every command, validate --okf included.
func SkipBundleDir(rel string) bool {
	rel = path.Clean(rel)
	if rel == "." {
		return false
	}
	return strings.HasPrefix(path.Base(rel), ".") || rel == EphemeralDir
}
