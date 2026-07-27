package config

// Version is okfw's own version. It appears in the OKF actor convention as
// "okfw/<version>" (see GeneratorActor) and is reported by the MCP server, so
// there is one place to change it rather than three.
const Version = "0.2.0"

// OKFVersion is the Open Knowledge Format revision okfw targets. Declared in the
// bundle-root index.md, which OKF v0.2 section 12 makes the only place an
// okf_version key is permitted.
const OKFVersion = "0.2"

// GeneratorActor returns the OKF actor string identifying okfw as the producer
// of a concept, for the `generated.by` field (OKF section 7: agents and tools
// use "<producer>/<version>").
func GeneratorActor() string { return "okfw/" + Version }
