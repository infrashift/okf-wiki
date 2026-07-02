// Concept schema with kind-specific validation
//
// OKF alignment:
//   - type: REQUIRED non-empty free-form concept kind (OKF). Defaults to the
//     `kind` value at generation time, but may be any descriptive string.
//   - description / resource: optional OKF recommended fields.
//   - timestamp: last meaningful change (OKF recommended); written on create
//     and updated on every mutation.
//
// Kinds (structured, extension):
//   - note (default): standard concept
//   - todo: actionable task with status tracking
//   - daily-note: daily capture note
//   - issue: issue tracking (bug, enhancement, question)
//
// Todo fields (when kind=="todo"):
//   - status: required (open, in_progress, closed)
//   - due: optional target date
//   - completed: set automatically when closed
//   - priority: optional (high, medium, low)

#Concept: {
	id:           =~"^([0-9]{14}-)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$" // UUIDv7 or legacy YYYYMMDDHHmmss-UUID
	title:        string & !=""
	type:         string & !=""                             // OKF required free-form concept kind
	kind:         *"note" | "todo" | "daily-note" | "issue" // structured kind, defaults to "note"
	description?: string
	resource?:    string
	project?:     string
	category?:    string // legacy field (untethered/tethered), tolerated but ignored
	tags: [...(string & !="")]
	created:   =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}.*" // ISO 8601 required
	timestamp: =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}.*" // ISO 8601, last meaningful change
	parent?:   =~"^([0-9]{14}-)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"     // optional link to parent concept

	status?:    "open" | "in_progress" | "closed"
	due?:       =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
	completed?: =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"
	priority?:  "high" | "medium" | "low"
}
