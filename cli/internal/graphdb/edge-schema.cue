// Edge schema for graph relationship validation.
//
// Predicates:
//   - links_to: body [[id]] link between concepts
//   - parent_of: hierarchical parent relationship
//   - child_of: hierarchical child relationship
//   - tagged: concept-to-tag association

#Edge: {
	from:      =~"^([0-9]{14}-)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
	predicate: "links_to" | "parent_of" | "child_of" | "tagged"
	to:        string & !=""
}
