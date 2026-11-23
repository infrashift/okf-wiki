// The vocabulary of the okf-wiki docs: technical terms of okfw, OKF and the
// tools okfw works with, which the extended rules STE-100-06 and STE-100-07
// do not report. `make docs-style` loads this file over the built-in ste-100
// ruleset:
//
//	lexiscope scan docs/src/content/docs --ruleset ste-100 --ruleset vocabulary.cue
//
// Add a term only when it names a concept of okfw, OKF, a tool or a standard.
// Rewrite other prose.
vocabulary: {
	// OKF: the format and its metadata families.
	conformance:  true // OKF conformance (section 11), okfw validate --okf
	provenance:   true // OKF provenance metadata (section 5.1): sources[]
	verification: true // an OKF verification event (section 5.3): verified[]
	computation:  true // OKF Attested Computation (section 10)
	divergence:   true // a deliberate divergence of okfw from OKF
	migration:    true // okfw migrate, and the OKF v0.3 migration guide
	dangling:     true // a dangling link: a link to a concept that does not exist
	violation:    true // a conformance violation that okfw validate --okf reports
	breaking:     true // a breaking change of the spec (OKF section 13.1) or of okfw

	// okfw: search, the CLI and its templates.
	ranking:    true // the order of search hits (okfw search)
	tracking:   true // task tracking, which okfw does not have
	meeting:    true // the meeting template
	rating:     true // the rating section of the book-review template
	acceptance: true // the acceptance criteria of the user-story template
	citation:   true // a [^id] footnote that cites a source; okfw validate checks them

	// Paths, files and the shell.
	leading:    true // a leading slash in a path: /dir/file.md
	working:    true // the working directory, the working tree
	quoting:    true // the quoting style of a YAML scalar
	completion: true // command-line completion in the shell and in NeoVim

	// Releases.
	signing: true // release signing with cosign

	// NeoVim.
	floating: true // a NeoVim floating window

	// The wiki of this repository: ADRs and issues.
	decision:   true // an architecture decision record, and its Decision section
	resolution: true // the Resolution section of an issue
	validation: true // strict validation, okfw validate --all

	// Standards and other tools.
	implementation: true // the OKF reference implementation
	notarization:   true // Apple notarization of a macOS binary

	// Daily notes.
	evening: true // the evening review of a daily note, like the exception morning
}
