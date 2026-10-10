package graph

import (
	"reflect"
	"testing"
)

func TestHeadings(t *testing.T) {
	body := "# Title\n" +
		"text with a # in it\n" +
		"#hashtag is not a heading\n" +
		"## Closed ##\n" +
		"### Ends in C# \n" +
		"   #### Indented three\n" +
		"    # indented four is code\n" +
		"####### seven is too many\n" +
		"#\n" +
		"```sh\n# a shell comment\n```\n" +
		"~~~\n## inside tildes\n~~~\n" +
		"## After\r\n"
	got := Headings(body)
	want := []Heading{
		{Line: 0, Level: 1, Text: "Title"},
		{Line: 3, Level: 2, Text: "Closed"},
		{Line: 4, Level: 3, Text: "Ends in C#"},
		{Line: 5, Level: 4, Text: "Indented three"},
		{Line: 8, Level: 1, Text: ""},
		{Line: 15, Level: 2, Text: "After"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Headings =\n%+v\nwant\n%+v", got, want)
	}
}

// An unclosed fence runs to the end of the body, so nothing after it is a
// heading.
func TestHeadingsInsideAnUnclosedFence(t *testing.T) {
	if got := Headings("# Before\n```\n# not a heading\n"); len(got) != 1 {
		t.Errorf("Headings = %+v, want only # Before", got)
	}
}
