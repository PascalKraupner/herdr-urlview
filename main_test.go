package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// The keys herdr actually uses are not obvious and getting one wrong fails
// silently: json.Unmarshal is happy, every field stays empty, and sourcePaneID
// falls through to the focused pane, which for a plugin pane is the popup
// itself. The picker then scans its own empty scrollback and reports no links.
func TestSourcePaneIDFromPluginContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want string
	}{
		// Ids that cannot match a live pane on purpose. With a plausible one the
		// test passes either way, because the buggy fallback asks herdr for the
		// focused pane and would hand back the very id being asserted.
		//
		// What herdr 0.7.5 sends for a plugin pane entrypoint.
		{"focused_pane_id", `{"workspace_id":"w9","focused_pane_id":"w9:pFAKE"}`, "w9:pFAKE"},
		{"pane_id", `{"pane_id":"w9:pFAKEA"}`, "w9:pFAKEA"},
		{"nested pane", `{"pane":{"pane_id":"w9:pFAKEB"}}`, "w9:pFAKEB"},
		// An explicit pane_id is preferred over the merely-focused one.
		{"both", `{"pane_id":"w9:pFAKEA","focused_pane_id":"w9:pFAKE"}`, "w9:pFAKEA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HERDR_PANE_ID", "")
			t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", tc.json)

			got, err := sourcePaneID()
			if err != nil {
				t.Fatalf("sourcePaneID() error: %v", err)
			}
			if got != tc.want {
				t.Errorf("sourcePaneID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// HERDR_PANE_ID wins outright, so a plain popup keybinding keeps working.
func TestSourcePaneIDPrefersEnv(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "w1:pZ")
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"focused_pane_id":"w1:pF"}`)

	got, err := sourcePaneID()
	if err != nil {
		t.Fatalf("sourcePaneID() error: %v", err)
	}
	if got != "w1:pZ" {
		t.Errorf("sourcePaneID() = %q, want %q", got, "w1:pZ")
	}
}

func TestPaneTextUsesNonScrollingUnwrappedRead(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	// Model herdr's unwrapped snapshot: styling may interrupt a long URL,
	// but a real newline before prose must remain a URL boundary.
	script := `#!/bin/sh
test "$*" = 'pane read w1:p1 --source recent-unwrapped --lines 1000 --format ansi' || exit 1
printf '\033[34mhttps://example.com/a/very/long/\033[0mpath?token=123456789&next=page\nprose\n'
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", bin)
	text, err := paneText("w1:p1", 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://example.com/a/very/long/path?token=123456789&next=page"}
	if got := extract(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripHyperlinkStyling(t *testing.T) {
	for _, terminator := range []string{"\a", "\x1b\\"} {
		text := "\x1b]8;;https://hidden.example" + terminator + "https://example.com/\x1b[1mpath\x1b[0m\x1b]8;;" + terminator
		got := ansiRe.ReplaceAllString(text, "")
		if got != "https://example.com/path" || strings.Contains(got, "\x1b") {
			t.Fatalf("unexpected stripped text: %q", got)
		}
	}
}
