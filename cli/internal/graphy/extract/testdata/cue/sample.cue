// Package schema is a golden-test fixture.
package schema

import (
	"strings"
)

// #Base is the shared shape.
#Base: {
	id:    string & !=""
	title: string
}

// #Concept extends #Base with metadata.
#Concept: {
	#Base
	description?: string
	tags: [...string]
	slug: strings.ToLower(title)
}

// defaults are plain values, not a definition.
defaults: {
	kind: "note"
}
