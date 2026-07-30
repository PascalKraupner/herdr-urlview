package main

import (
	"reflect"
	"testing"
)

func TestExtract(t *testing.T) {
	const sample = `
  see https://herdr.dev/docs/plugins/ for details.
  Also (https://example.com/a_(b)) and www.foo.dev/x?q=1&r=2.
  Trailing prose: visit https://news.ycombinator.com/item?id=12932592.
  dupe: https://herdr.dev/docs/plugins/
  ftp://mirror.example.org/pub/file.tar.gz
  not a url: foo/bar, http://
`

	// Newest first, deduped, trailing prose punctuation stripped, the balanced
	// paren in a_(b) preserved, and bare "http://" rejected as too short.
	want := []string{
		"ftp://mirror.example.org/pub/file.tar.gz",
		"https://news.ycombinator.com/item?id=12932592",
		"www.foo.dev/x?q=1&r=2",
		"https://example.com/a_(b)",
		"https://herdr.dev/docs/plugins/",
	}

	if got := extract(sample); !reflect.DeepEqual(got, want) {
		t.Errorf("extract()\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExtractEmpty(t *testing.T) {
	if got := extract("nothing to see here, foo/bar, mailto:a@b.c"); len(got) != 0 {
		t.Errorf("expected no URLs, got %#v", got)
	}
}
