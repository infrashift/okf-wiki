package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/config"
)

// BundleMeta is the bundle metadata OKF v0.3 section 8.1 lets the bundle-root
// index.md carry in its frontmatter.
type BundleMeta struct {
	OKFVersion  string `yaml:"okf_version"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"` // when to use the bundle; an agent picks bundles by it
	License     string `yaml:"license"`     // an SPDX expression or a path (section 6.2)
}

// BundleMetaKeys are the keys section 8.1 defines for the root index.md. An
// x-* extension key is also permitted there.
var BundleMetaKeys = []string{"okf_version", "title", "description", "license"}

// ReadBundleMeta reads the bundle-root index.md's frontmatter. A missing file
// or one without frontmatter yields the zero value.
func ReadBundleMeta(bundleRoot string) (BundleMeta, error) {
	var meta BundleMeta
	data, err := os.ReadFile(filepath.Join(bundleRoot, "index.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return meta, nil
		}
		return meta, err
	}
	if !hasFrontmatter(data) {
		return meta, nil
	}
	fm, _, _, err := splitFrontmatter(data)
	if err != nil {
		return meta, err
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return meta, fmt.Errorf("index.md frontmatter: %w", err)
	}
	return meta, nil
}

// SetBundleMeta sets keys in the bundle-root index.md's frontmatter, keeping
// every other key, comment and quoting style (section 8.1: a producer MUST
// keep the existing root frontmatter). An empty value removes the key.
// The root index.md is created first when there is none.
func SetBundleMeta(cfg *config.Config, set map[string]string) error {
	for k := range set {
		if !isBundleMetaKey(k) {
			return fmt.Errorf("%q is not bundle metadata: use okf_version, title, description, license or an x-* key (OKF section 8.1)", k)
		}
	}
	p := filepath.Join(cfg.RootPath, "index.md")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if _, err := WriteIndexFiles(cfg, cfg.RootPath); err != nil {
			return err
		}
	}
	content, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if !hasFrontmatter(content) {
		content = append([]byte("---\n---\n\n"), content...)
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out, err := Edit(content, func(m *yaml.Node) error {
		for _, k := range keys {
			v := strings.TrimSpace(set[k])
			if v == "" {
				MapDelete(m, k)
				continue
			}
			if strings.ContainsAny(v, "\r\n") {
				return fmt.Errorf("%s must be one line", k)
			}
			MapSetString(m, k, v)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0644)
}

func isBundleMetaKey(k string) bool {
	for _, b := range BundleMetaKeys {
		if k == b {
			return true
		}
	}
	return strings.HasPrefix(k, "x-")
}

// CheckBundleMeta reports what okfw's strict validation requires of the
// bundle-root index.md beyond the spec: a title and a description. OKF v0.3
// makes both optional; okfw requires them of the bundles it authors because
// the description is what an agent holding several bundles selects one by, and
// a field nobody is made to write stays empty. `validate --okf` does not apply
// this.
func CheckBundleMeta(bundleRoot string) []string {
	meta, err := ReadBundleMeta(bundleRoot)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	if strings.TrimSpace(meta.OKFVersion) == "" {
		problems = append(problems, "the root index.md declares no okf_version (run `okfw migrate`)")
	}
	if strings.TrimSpace(meta.Title) == "" {
		problems = append(problems, "the root index.md has no title (set it with `okfw set-bundle-meta --title`)")
	}
	if strings.TrimSpace(meta.Description) == "" {
		problems = append(problems, "the root index.md has no description (set it with `okfw set-bundle-meta --description`)")
	}
	return problems
}

var uriSchemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// UpgradeBarePathRefs rewrites the path fields of a concept written for OKF
// v0.2 so that they mean under v0.3 what okfw meant by them then. okfw read a
// bare path ("skills/run.md") from the bundle root; v0.3 reads it from the
// concept's directory (section 6.2, a breaking change). A bare value that
// names a file from the root, and none from the concept's directory, gains a
// leading "/". Anything else is left: URLs, "/", "./" and "../" values, scope
// descriptors, and values naming nothing — those are reported, since they
// meant nothing under either reading.
//
// It returns the rewritten content (nil when nothing changed), the fields it
// changed, and warnings.
func UpgradeBarePathRefs(content []byte, bundleRoot, fromDir string) ([]byte, []string, []string, error) {
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(bundleRoot, filepath.FromSlash(rel)))
		return err == nil
	}
	var changed, warnings []string
	upgrade := func(v string) (string, bool, string) {
		value := strings.TrimSpace(v)
		p := value
		if i := strings.IndexByte(p, '#'); i >= 0 {
			p = p[:i]
		}
		switch {
		case p == "",
			strings.HasPrefix(p, "/"), strings.HasPrefix(p, "./"), strings.HasPrefix(p, "../"),
			uriSchemeRe.MatchString(p),
			strings.ContainsAny(p, " \t"): // a scope descriptor, not a path
			return "", false, ""
		}
		local := filepath.ToSlash(filepath.Clean(filepath.Join(fromDir, p)))
		root := filepath.ToSlash(filepath.Clean(p))
		if strings.HasPrefix(root, "../") || root == ".." {
			return "", false, ""
		}
		switch {
		case exists(root) && !exists(local):
			return "/" + value, true, ""
		case !exists(root) && !exists(local) && strings.HasSuffix(p, ".md"):
			return "", false, "names no file from the bundle root or the concept's directory"
		}
		return "", false, ""
	}
	if !hasFrontmatter(content) {
		return nil, nil, nil, nil
	}
	out, err := Edit(content, func(m *yaml.Node) error {
		for _, r := range pathRefNodes(m) {
			nv, ok, warn := upgrade(r.node.Value)
			if warn != "" {
				warnings = append(warnings, fmt.Sprintf("%s %q %s", r.field, r.node.Value, warn))
			}
			if ok {
				r.node.Value = nv
				changed = append(changed, r.field)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if len(changed) == 0 {
		return nil, nil, warnings, nil
	}
	return out, changed, warnings, nil
}

// StampOKFVersion sets the bundle-root index.md's okf_version to the version
// okfw targets, keeping its other frontmatter. Only `okfw migrate` calls it:
// the version decides how a bare path field reads (section 6.2), so bumping it
// on an ordinary write would silently change what a v0.2 bundle's paths mean.
// It reports whether the file changed.
func StampOKFVersion(cfg *config.Config) (bool, error) {
	meta, err := ReadBundleMeta(cfg.RootPath)
	if err != nil {
		return false, err
	}
	if meta.OKFVersion == config.OKFVersion {
		return false, nil
	}
	return true, SetBundleMeta(cfg, map[string]string{"okf_version": config.OKFVersion})
}

// UpgradeScratchDir removes the export scratch directory okfw wrote before
// v0.3, ephemeral/ at the bundle root, and points the bundle's .gitignore at
// its new name. OKF v0.3 section 3.2 leaves only dot-named paths outside the
// bundle, so another consumer read every exported copy as a second concept.
// The old directory is removed only when it carries the .gitignore okfw's
// export writes — anything else is someone's content, and is reported instead.
func UpgradeScratchDir(cfg *config.Config) (removed bool, warning string, err error) {
	gi := filepath.Join(cfg.RootPath, ".gitignore")
	if data, rerr := os.ReadFile(gi); rerr == nil {
		s := string(data)
		if strings.Contains(s, "\nephemeral/\n") || strings.HasPrefix(s, "ephemeral/\n") {
			s = strings.Replace(s, "ephemeral/\n", config.EphemeralDir+"/\n", 1)
			if werr := os.WriteFile(gi, []byte(s), 0644); werr != nil {
				return false, "", werr
			}
		}
	}
	old := filepath.Join(cfg.RootPath, config.LegacyEphemeralDir)
	if _, serr := os.Stat(old); os.IsNotExist(serr) {
		return false, "", nil
	}
	marker, rerr := os.ReadFile(filepath.Join(old, ".gitignore"))
	if rerr != nil || !strings.Contains(string(marker), "Auto-generated") {
		return false, config.LegacyEphemeralDir + "/ is not okfw's export output; left in place", nil
	}
	return true, "", os.RemoveAll(old)
}
