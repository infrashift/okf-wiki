// Package sample is a golden-test fixture.
package sample

import (
	"fmt"
	"strings"
)

// Greeter greets people.
type Greeter struct {
	prefix string
}

// Mood is how a greeting feels.
type Mood interface {
	Feel() string
}

// Alias is a plain type alias target.
type Alias = string

// NewGreeter builds a Greeter.
func NewGreeter(prefix string) *Greeter {
	return &Greeter{prefix: strings.TrimSpace(prefix)}
}

// Greet says hello.
func (g *Greeter) Greet(name string) string {
	return format(g.prefix, name)
}

func format(prefix, name string) string {
	return fmt.Sprintf("%s, %s!", prefix, name)
}
