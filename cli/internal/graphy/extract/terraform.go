package extract

import (
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func init() {
	register("terraform", func() (Extractor, error) { return &terraformExtractor{}, nil })
}

// terraformExtractor parses HCL in pure Go via hashicorp/hcl/v2 — the
// official parser gives block structure AND expression traversals, so
// resources, modules, variables, locals, and outputs become nodes with their
// Terraform address as the label (aws_instance.web, var.region, module.vpc),
// and every var./local./module./resource reference inside expressions becomes
// a resolvable edge. Module source paths resolve like imports: an internal
// directory with extracted files becomes a module node.
type terraformExtractor struct{}

func (t *terraformExtractor) Language() string { return "terraform" }
func (t *terraformExtractor) Close()           {}

// tfSkipRoots are traversal roots that never name a graph entity.
var tfSkipRoots = map[string]bool{
	"count": true, "each": true, "path": true, "terraform": true, "self": true,
}

func (t *terraformExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	fn := fileNode("terraform", file)
	res.Nodes = append(res.Nodes, fn)

	f, diags := hclsyntax.ParseConfig(src, file.Path, hcl.Pos{Line: 1, Column: 1})
	if f == nil {
		return res, nil
	}
	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		return res, nil
	}
	_ = diags // partial ASTs are still useful; malformed parts are skipped

	addNode := func(kind model.Kind, addr string, line int) string {
		n := model.Node{
			ID:       model.NodeID("terraform", kind, file.Path, addr),
			Label:    addr,
			Kind:     kind,
			Language: "terraform",
			File:     file.Path,
			Span:     model.Span{StartLine: line},
		}
		res.Nodes = append(res.Nodes, n)
		res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, line))
		return n.ID
	}
	addRefs := func(fromID string, body *hclsyntax.Body) {
		seen := map[string]bool{}
		var walk func(b *hclsyntax.Body)
		walk = func(b *hclsyntax.Body) {
			// Attributes are a map: iterate sorted for deterministic output.
			for _, name := range sortedAttrNames(b.Attributes) {
				attr := b.Attributes[name]
				for _, trav := range attr.Expr.Variables() {
					addr := tfAddress(trav)
					if addr == "" || seen[addr] {
						continue
					}
					seen[addr] = true
					res.Refs = append(res.Refs, model.Ref{
						FromID:   fromID,
						Name:     addr,
						Relation: model.RelReferences,
						Line:     attr.SrcRange.Start.Line,
					})
				}
			}
			for _, nested := range b.Blocks {
				walk(nested.Body)
			}
		}
		walk(body)
	}

	// .tfvars files are plain attribute assignments: each one references the
	// variable it sets, which links environments to their declarations.
	if strings.HasSuffix(file.Path, ".tfvars") {
		for _, name := range sortedAttrNames(body.Attributes) {
			res.Refs = append(res.Refs, model.Ref{
				FromID:   fn.ID,
				Name:     "var." + name,
				Relation: model.RelReferences,
				Line:     body.Attributes[name].SrcRange.Start.Line,
			})
		}
		return res, nil
	}

	for _, block := range body.Blocks {
		line := block.DefRange().Start.Line
		switch block.Type {
		case "resource":
			if len(block.Labels) == 2 {
				id := addNode(model.KindResource, block.Labels[0]+"."+block.Labels[1], line)
				addRefs(id, block.Body)
			}
		case "data":
			if len(block.Labels) == 2 {
				id := addNode(model.KindResource, "data."+block.Labels[0]+"."+block.Labels[1], line)
				addRefs(id, block.Body)
			}
		case "module":
			if len(block.Labels) == 1 {
				id := addNode(model.KindModule, "module."+block.Labels[0], line)
				addRefs(id, block.Body)
				// The module's source path resolves like an import: internal
				// directories become module nodes, registry sources external.
				if src, ok := block.Body.Attributes["source"]; ok {
					if lit := tfStringLiteral(src.Expr); lit != "" {
						res.Refs = append(res.Refs, model.Ref{
							FromID:   id,
							Name:     lit,
							Relation: model.RelImports,
							Line:     src.SrcRange.Start.Line,
						})
					}
				}
			}
		case "variable":
			if len(block.Labels) == 1 {
				addNode(model.KindVariable, "var."+block.Labels[0], line)
			}
		case "output":
			if len(block.Labels) == 1 {
				id := addNode(model.KindVariable, "output."+block.Labels[0], line)
				addRefs(id, block.Body)
			}
		case "locals":
			for _, name := range sortedAttrNames(block.Body.Attributes) {
				attr := block.Body.Attributes[name]
				id := addNode(model.KindVariable, "local."+name, attr.SrcRange.Start.Line)
				localBody := &hclsyntax.Body{Attributes: hclsyntax.Attributes{name: attr}}
				addRefs(id, localBody)
			}
		case "provider":
			if len(block.Labels) == 1 {
				addNode(model.KindConfig, "provider."+block.Labels[0], line)
			}
		}
	}
	return res, nil
}

func sortedAttrNames(attrs hclsyntax.Attributes) []string {
	names := make([]string, 0, len(attrs))
	for name := range attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// tfAddress renders a traversal as the graph address of what it references:
// var.x / local.x / module.x stop after two parts; data.t.n after three;
// bare resource references (aws_instance.web.id) after type.name.
func tfAddress(trav hcl.Traversal) string {
	root := trav.RootName()
	if tfSkipRoots[root] {
		return ""
	}
	var parts []string
	for _, step := range trav {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			parts = append(parts, s.Name)
		case hcl.TraverseAttr:
			parts = append(parts, s.Name)
		}
	}
	want := 2
	if root == "data" {
		want = 3
	}
	if len(parts) < want {
		return ""
	}
	return strings.Join(parts[:want], ".")
}

// tfStringLiteral extracts a plain string literal from an expression
// ("./modules/vpc"); expressions with interpolation return "".
func tfStringLiteral(expr hclsyntax.Expression) string {
	tmpl, ok := expr.(*hclsyntax.TemplateExpr)
	if !ok || len(tmpl.Parts) != 1 {
		return ""
	}
	lit, ok := tmpl.Parts[0].(*hclsyntax.LiteralValueExpr)
	if !ok || lit.Val.Type().FriendlyName() != "string" {
		return ""
	}
	return lit.Val.AsString()
}
