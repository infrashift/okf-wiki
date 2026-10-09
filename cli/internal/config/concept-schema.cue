// Concept schema — OKF v0.2 plus okfw's declared extensions.
//
// DELIBERATE DIVERGENCE FROM OKF v0.2. Two choices here knowingly break the
// spec's consumer rules, in exchange for a hard guarantee about the bundles okfw
// authors. Both are intentional; do not "fix" them toward the spec:
//
//   1. Required fields. okfw requires id/title/type/tags/created/timestamp.
//      OKF v0.2 section 11 requires only a non-empty `type`, and says consumers
//      MUST NOT reject a concept for missing optional fields.
//   2. Closed schema. #Concept is a CUE definition, so undeclared top-level keys
//      are rejected. OKF v0.2 section 4.1 says consumers MUST NOT reject
//      documents with unrecognized fields and SHOULD round-trip them.
//
// The `x:` block is the escape hatch that keeps (2) liveable: it is the one open
// struct in the schema, so users can attach arbitrary metadata without the
// closed schema refusing the file.
//
// `okfw validate --okf` implements the permissive spec check, so a bundle okfw
// did not author can still be assessed on the spec's own terms.
//
// OKF v0.2 field families, all optional:
//   - Core (section 4.1): type (required), title, description, resource, tags.
//   - Provenance (section 5.1): sources, usage_window.
//   - Trust (section 5.2): generated, verified. A bare `verified` mapping is a
//     one-element list — normalized in Go, since CUE cannot rewrite shape.
//   - Lifecycle (sections 5.4-5.5): status, stale_after. Absent status => stable.
//
// okfw extensions: id, kind, project, parent, x (open).
//
// okfw has no task-tracking fields. It used to carry status/due/completed/
// priority for a `todo` kind, where `status` collided with OKF's lifecycle
// field; the whole feature was removed rather than namespaced, to keep okfw a
// knowledge format instead of a task tracker. `okfw migrate` strips those
// fields from concepts written before the removal.

#Actor: string & !="" // "<producer>/<version>", "human:<id>", or "process:<id>"

// OKF section 5: every timestamp is an ISO 8601 datetime with an explicit UTC
// offset. Keep in step with dateTimeRe in timestamps.go.
#DateTime: =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$"

#ConceptID: =~"^([0-9]{14}-)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

// Trust (OKF section 5.2)
#Generated: {
	by:  #Actor
	at?: #DateTime
}

#Verification: {
	by:  #Actor
	at?: #DateTime
}

// Provenance (OKF section 5.1)
#UsageWindow: {
	from?: #DateTime
	to?:   #DateTime
}

#Source: {
	resource:       string & !="" // a path/URL, or a scope descriptor
	id?:            string & !="" // join key for [^id] body footnotes
	title?:         string & !=""
	author?:        #Actor
	usage_count?:   int & >=0
	last_modified?: #DateTime
	usage_window?:  #UsageWindow // overrides the concept-level window
}

// Attested Computation (OKF section 10.2)
#Parameter: {
	name:      string & !=""
	type:      string & !=""
	required?: bool
}

#Executor: {
	resource?: string & !=""
	receipt?: [...(string & !="")]
}

#Attester: {
	resource?: string & !=""
}

#Concept: {
	// --- OKF core ---
	type:         string & !="" // OKF required free-form concept kind
	title:        string & !=""
	description?: string
	resource?:    string
	tags: [...(string & !="")]

	// --- OKF lifecycle (section 5.4-5.5) ---
	// Absent means "stable".
	status?:      "draft" | "stable" | "deprecated"
	stale_after?: #DateTime

	// --- OKF trust (section 5.2) ---
	generated?: #Generated
	verified?: #Verification | [...#Verification]

	// --- OKF provenance (section 5.1) ---
	sources?: [...#Source]
	usage_window?: #UsageWindow

	// --- OKF Attested Computation (section 10) ---
	// okfw does not execute or attest anything; these are declared so a bundle
	// can hold an Attested Computation concept at all. They are spec-defined
	// top-level keys, so the closed schema rejected them, and the x: escape
	// hatch could not help.
	runtime?: string & !=""
	parameters?: [...#Parameter]
	computation?: string & !="" // a path (section 6.2); absent means the body fence
	executor?:    #Executor
	attester?:    #Attester // runtime is required for this type: see ValidateBusinessRules

	// --- okfw extensions ---
	id:        #ConceptID // okfw-required; OKF derives concept identity from the file path
	kind:      *"note" | "daily-note" | "issue"
	project?:  string
	parent?:   #ConceptID
	created:   #DateTime
	timestamp: #DateTime // mirrors generated.at; see TouchTimestamp
	category?: string    // legacy (untethered/tethered), tolerated but ignored

	// The one open struct in an otherwise closed schema: user-defined metadata
	// passes through untouched. This is what makes a closed #Concept liveable —
	// without it there is no way to attach a custom field at all.
	x?: {
		...
	}
}
