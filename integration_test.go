package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt in with HERDR_TEST_BIN=/absolute/path/to/herdr go test -run TestLive.
// A private home, config, and socket keep this away from the user's session.
func TestLiveWrappedURL(t *testing.T) {
	bin := os.Getenv("HERDR_TEST_BIN")
	if bin == "" {
		t.Skip("set HERDR_TEST_BIN to run against a real herdr server")
	}
	dir := t.TempDir()
	for key, value := range map[string]string{
		"HOME": dir, "XDG_CONFIG_HOME": dir, "XDG_STATE_HOME": dir,
		"HERDR_CONFIG_PATH": filepath.Join(dir, "config.toml"),
		"HERDR_SOCKET_PATH": filepath.Join(dir, "herdr.sock"),
		"HERDR_BIN_PATH":    bin, "HERDR_SESSION": "", "HERDR_PANE_ID": "",
		"HERDR_CLIENT_SOCKET_PATH": filepath.Join(dir, "client.sock"),
	} {
		t.Setenv(key, value)
	}
	config := "[terminal]\ndefault_shell = \"/bin/sh\"\n[server]\nheadless_cols = 40\nheadless_rows = 24\n[update]\nversion_check = false\nmanifest_check = false\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	server := exec.Command(bin, "server")
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "herdr.sock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not create its socket")
		}
		time.Sleep(50 * time.Millisecond)
	}
	out, err := herdr("workspace", "create", "--cwd", dir, "--no-focus")
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	pane := created.Result.RootPane.PaneID
	url := "https://example.com/" + strings.Repeat("long-path/", 15) + "?token=abcdef&next=page"
	for _, screen := range []string{"normal", "alternate"} {
		t.Run(screen, func(t *testing.T) {
			prefix := ""
			if screen == "alternate" {
				prefix = `printf '\033[?1049h'; `
			}
			command := prefix + `printf '\033[34m%s\033[0m\n' '` + url + `'`
			if _, err := herdr("pane", "run", pane, command); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				text, err := paneText(pane, 1000)
				if err != nil {
					t.Fatal(err)
				}
				for _, got := range extract(text) {
					if got == url {
						return
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("wrapped URL not recovered from %s screen: %q", screen, text)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}
