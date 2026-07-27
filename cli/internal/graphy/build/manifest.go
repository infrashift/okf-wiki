package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/model"
)

// Package-manifest dependencies. Every call into third-party or standard-library
// code is dropped by resolution — there is nothing in the repo to resolve it to —
// so before this a graph could not answer "what does this subsystem rely on that
// we do not own", which is the first question asked about a dependency
// advisory, a licence audit, or an upgrade.
//
// A manifest states the answer directly, so it is read rather than inferred, and
// the dependency nodes are the same external nodes import resolution
// synthesizes. That convergence is the point: a declared dependency nothing
// imports has a `depends_on` edge and no `imports` edges, which is exactly the
// shape of an unused dependency.

// maxManifestDeps caps dependencies per manifest. A lockfile-sized dependency
// list would otherwise dominate the graph with nodes no one asked about.
const maxManifestDeps = 300

// manifestDeps adds the repository's declared dependencies as external nodes
// with `depends_on` edges from a node representing the manifest's module.
func manifestDeps(root string, synthetic map[string]model.Node, addEdge func(model.Edge)) {
	addAll := func(lang, manifest, owner string, deps map[string]string) {
		if owner == "" || len(deps) == 0 {
			return
		}
		ownerID := model.NodeID(lang, model.KindModule, manifest, owner)
		if _, ok := synthetic[ownerID]; !ok {
			synthetic[ownerID] = model.Node{
				ID: ownerID, Label: owner, Kind: model.KindModule, Language: lang,
				File: manifest, Span: model.Span{StartLine: 1},
				Doc: "Declared dependencies of " + manifest + ".",
			}
		}
		names := make([]string, 0, len(deps))
		for name := range deps {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > maxManifestDeps {
			names = names[:maxManifestDeps]
		}
		for _, name := range names {
			id := model.NodeID(lang, model.KindExternal, "", name)
			// Import resolution may have created this node already — a
			// dependency imported by its module path lands on the same id. Reuse
			// it and add the declared version, which only the manifest knows;
			// creating a rival node would split the dependency in two.
			if n, ok := synthetic[id]; ok {
				if n.Doc == "" {
					n.Doc = deps[name]
					synthetic[id] = n
				}
			} else {
				synthetic[id] = model.Node{
					ID: id, Label: name, Kind: model.KindExternal,
					Language: lang, External: true, Doc: deps[name],
				}
			}
			addEdge(model.Edge{
				Source: ownerID, Target: id,
				Relation: model.RelDependsOn, Confidence: model.ConfExtracted,
				File: manifest, Line: 1,
			})
			// Go imports name packages, manifests name modules, and the two
			// spellings differ for anything with subpackages: the graph holds
			// an external node for ".../tree-sitter-go/bindings/go" while go.mod
			// declares ".../tree-sitter-go". Without joining them the module
			// looks unused and the packages look undeclared. Ownership is the
			// honest relation — a module contains its packages.
			for _, sub := range subPackages(synthetic, lang, name) {
				addEdge(model.Edge{
					Source: id, Target: sub,
					Relation: model.RelContains, Confidence: model.ConfInferred,
					File: manifest, Line: 1,
				})
			}
		}
	}

	mod, deps := goModDeps(filepath.Join(root, "go.mod"))
	addAll("go", "go.mod", mod, deps)

	name, npm := npmDeps(filepath.Join(root, "package.json"))
	addAll("javascript", "package.json", name, npm)
}

// subPackages returns the ids of already-synthesized external nodes that sit
// underneath a declared module path.
func subPackages(synthetic map[string]model.Node, lang, module string) []string {
	prefix := module + "/"
	var out []string
	for id, n := range synthetic {
		if n.External && n.Language == lang && strings.HasPrefix(n.Label, prefix) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// goModDeps reads a go.mod's module path and its direct requirements.
//
// Parsed by hand rather than through golang.org/x/mod: graphy needs the declared
// names, not a resolved build list, and a require line is two fields. Both forms
// are accepted — a `require (` block and a bare `require path v1.2.3` — since
// the block is only a grouping.
func goModDeps(path string) (string, map[string]string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	mod := ""
	if m := goModuleRe.FindSubmatch(content); m != nil {
		mod = string(m[1])
	}
	deps := map[string]string{}
	inRequire := false
	for _, line := range strings.Split(string(content), "\n") {
		// "// indirect" requirements are dependencies of the build but not of
		// this repo's code, and listing them buries the ones that are.
		if i := strings.Index(line, "//"); i >= 0 {
			if strings.Contains(line[i:], "indirect") {
				continue
			}
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		bare := strings.HasPrefix(line, "require ")
		if !inRequire && !bare {
			continue // module/go/toolchain/replace/exclude lines
		}
		fields := strings.Fields(strings.TrimPrefix(line, "require "))
		if len(fields) == 2 && strings.HasPrefix(fields[1], "v") {
			deps[fields[0]] = fields[1]
		}
	}
	return mod, deps
}

// npmDeps reads a package.json's name and its runtime + dev dependencies.
func npmDeps(path string) (string, map[string]string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	var pkg struct {
		Name    string            `json:"name"`
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return "", nil
	}
	deps := map[string]string{}
	for name, v := range pkg.Deps {
		deps[name] = v
	}
	for name, v := range pkg.DevDeps {
		if _, ok := deps[name]; !ok {
			deps[name] = v + " (dev)"
		}
	}
	return pkg.Name, deps
}
