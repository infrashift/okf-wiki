// Concept schema — OKF v0.3 plus okfw's declared extensions.
//
// DELIBERATE DIVERGENCE FROM OKF v0.3. Two choices here knowingly break the
// spec's consumer rules, in exchange for a hard guarantee about the bundles okfw
// authors. Both are intentional; do not "fix" them toward the spec:
//
//   1. Required fields. okfw requires id/title/type/tags/created/timestamp.
//      OKF v0.3 section 4.1 requires only a non-empty `type` (title and tags
//      are optional), and section 11.2 says consumers MUST NOT reject a
//      concept for missing optional fields.
//   2. Closed schema. #Concept is a CUE definition, so undeclared keys are
//      rejected. OKF v0.3 section 4.1 says consumers MUST NOT reject documents
//      with unknown keys, at any level.
//
// Two escape hatches keep (2) liveable. Every mapping accepts `x-*` keys, the
// namespace v0.3 section 4.1 promises never to claim (producers SHOULD name
// theirs x-<producer>-<name>); and the open `x:` block from okfw's v0.2 days
// still passes anything through. okfw itself writes neither.
//
// `okfw validate --okf` implements the permissive spec check, so a bundle okfw
// did not author can still be assessed on the spec's own terms.
//
// OKF v0.3 field families, all optional:
//   - Core (section 4.1): type (required), title, description, resource, tags.
//   - Provenance (section 5.1): sources, usage_window.
//   - Trust (section 5.2): generated, verified. A bare `verified` mapping is a
//     one-element list — normalized in Go, since CUE cannot rewrite shape.
//   - Lifecycle (sections 5.4-5.5): status, superseded_by, stale_after.
//     Absent status => stable.
//
// okfw extensions: id, kind, project, parent, created, timestamp, x (open).
//
// okfw has no task-tracking fields. It used to carry status/due/completed/
// priority for a `todo` kind, where `status` collided with OKF's lifecycle
// field; the whole feature was removed rather than namespaced, to keep okfw a
// knowledge format instead of a task tracker. `okfw migrate` strips those
// fields from concepts written before the removal.

// OKF section 7, for generated.by and verified[].by: "<scheme>:<id>" with a
// lowercase scheme (human:, team:, process:, or another), or
// "<producer>/<version>". The trust tier keys on the exact prefix "human:", so
// Human:alice must not pass as some other scheme. Keep in step with actorRe in
// actor.go; the near-miss guard (humna:) is Go-only, see ValidateActor.
#Actor: =~"^[a-z][a-z0-9+.-]*:\\S(.*\\S)?$|^[^\\s/:]+/\\S(.*\\S)?$"

// okfw's own created/timestamp: an ISO 8601 datetime with an explicit UTC
// offset. Keep in step with dateTimeRe in timestamps.go.
#DateTime: =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$"

// OKF v0.3 section 5: an OKF timestamp is a date-time with an offset, or a
// full-date meaning the start of that day in UTC. Keep in step with
// ParseTimestamp in timestamps.go.
#Timestamp: #DateTime | =~"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"

// OKF v0.3 section 4.1: extension keys may appear at any level, and future
// versions will not define keys starting with x-. Every definition embeds this.
#Ext: {[=~"^x-"]: _}

#ConceptID: =~"^([0-9]{14}-)?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"

// Trust (OKF section 5.2)
#Generated: {
	#Ext
	by:  #Actor
	at?: #Timestamp // the last meaningful change; not okfw's timestamp
}

// v0.3 section 5.2: by and at are both REQUIRED in an event.
#Verification: {
	#Ext
	by: #Actor
	at: #Timestamp
}

// Provenance (OKF section 5.1)
#UsageWindow: {
	#Ext
	from?: #Timestamp
	to?:   #Timestamp
}

#Source: {
	#Ext
	resource:       string & !="" // a path/URL, or a scope descriptor
	id?:            string & !="" // join key for [^id] body footnotes
	title?:         string & !=""
	author?:        string & !="" // an actor (section 7); validate warns, not fails
	usage_count?:   int & >=0
	last_modified?: #Timestamp
	usage_window?:  #UsageWindow // overrides the concept-level window
}

// Attested Computation (OKF section 10.2)
#Parameter: {
	#Ext
	name:      string & !=""
	type:      string & !=""
	required?: bool
}

#Executor: {
	#Ext
	resource?: string & !=""
	receipt?: [...(string & !="")]
}

#Attester: {
	#Ext
	resource?: string & !=""
}

#Concept: {
	#Ext

	// --- OKF core ---
	type:         string & !="" // OKF required free-form concept kind
	title:        string & !=""
	description?: string
	resource?:    string
	tags: [...(string & !="")]

	// --- OKF lifecycle (section 5.4-5.5) ---
	// Absent means "stable". superseded_by is a path (section 6.2), meaningful
	// only under deprecated; validate warns otherwise.
	status?:        "draft" | "stable" | "deprecated"
	superseded_by?: string & !=""
	stale_after?:   #Timestamp

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
	timestamp: #DateTime // okfw's last-write time; NOT generated.at (see TouchTimestamp)
	category?: string    // legacy (untethered/tethered), tolerated but ignored

	// The one open struct in an otherwise closed schema: user-defined metadata
	// passes through untouched. This is what makes a closed #Concept liveable —
	// without it there is no way to attach a custom field at all.
	x?: {
		...
	}
}
