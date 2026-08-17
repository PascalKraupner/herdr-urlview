// Command herdr-urlview picks a URL out of a herdr pane and opens or copies it.
//
// A herdr equivalent of tmux-urlview. Reads the invoking pane's scrollback
// through the herdr socket API, extracts URLs, and offers them in an fzf picker.
//
//	↑ ↓     move, also Ctrl-K and Ctrl-J, wrapping around at both ends
//	Enter   open in the default browser
//	Ctrl-Y  copy to the clipboard instead
//	Esc     cancel
//
// HERDR_URLVIEW_LINES sets how much scrollback to scan (default 5000).
//
// Go rather than a scripting language so the published plugin ships as a static
// binary and needs no runtime on the installing machine.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const defaultLines = 5000

// Punctuation far more likely to be prose than part of the URL.
const trailing = ".,;:!?'\")]}>"

// Deliberately conservative: terminals wrap and decorate output, so anything
// clever produces more false positives than it recovers real links. Built by
// concatenation because a raw string literal cannot contain a backtick.
const notURL = `[^\s<>"'` + "`" + `\[\]{}|\\^]+`

var urlRe = regexp.MustCompile(`(?i)(?:https?|ftp|file)://` + notURL + `|www\.` + notURL)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "herdr-urlview: %v\n", err)
		pause()
		os.Exit(1)
	}
}

func run() error {
	lines := defaultLines
	if raw := os.Getenv("HERDR_URLVIEW_LINES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			lines = n
		}
	}

	pane, err := sourcePaneID()
	if err != nil {
		return err
	}

	text, err := paneText(pane, lines)
	if err != nil {
		return err
	}

	urls := extract(text)
	if len(urls) == 0 {
		// No pane id and no program name. This is the one thing the reader sees
		// in a popup that is about to ask them to press enter, and neither tells
		// them anything they can act on.
		//
		// Deliberately no Nerd Font glyph, unlike the picker chrome: this is the
		// path that runs when there is nothing to show, so it should stay legible
		// on a terminal without the font rather than opening with a tofu box.
		fmt.Printf("\n  No links in the last %d lines of output.\n", lines)
		pause()
		return nil
	}

	key, url, err := pick(urls)
	if err != nil {
		return err
	}
	if url == "" {
		return nil // cancelled
	}

	if key == "ctrl-y" {
		return copyToClipboard(url)
	}
	if strings.HasPrefix(url, "www.") {
		url = "https://" + url
	}
	return open(url)
}

func herdrBin() string {
	if bin := os.Getenv("HERDR_BIN_PATH"); bin != "" {
		return bin
	}
	return "herdr"
}

func herdr(args ...string) (string, error) {
	cmd := exec.Command(herdrBin(), args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("herdr %s failed: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// sourcePaneID resolves the pane whose output should be scanned. herdr passes
// the invoking pane in the environment; when it does not (run by hand, or from
// a popup carrying no pane context), fall back to the focused pane.
func sourcePaneID() (string, error) {
	if pane := os.Getenv("HERDR_PANE_ID"); pane != "" {
		return pane, nil
	}

	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		var ctx struct {
			PaneID string `json:"pane_id"`
			Pane   struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
			// What herdr 0.7.5 actually sends for a plugin pane, and the only one
			// of the three that is populated in practice. It is the pane that was
			// focused when the entrypoint was invoked, so it is still the pane the
			// user was looking at rather than the popup about to open. Without it
			// this function fell through to the pane list below, by which point
			// the popup itself is focused, and the picker scanned its own empty
			// scrollback and reported no links.
			FocusedPaneID string `json:"focused_pane_id"`
		}
		if json.Unmarshal([]byte(raw), &ctx) == nil {
			if ctx.PaneID != "" {
				return ctx.PaneID, nil
			}
			if ctx.Pane.PaneID != "" {
				return ctx.Pane.PaneID, nil
			}
			if ctx.FocusedPaneID != "" {
				return ctx.FocusedPaneID, nil
			}
		}
	}

	out, err := herdr("pane", "list")
	if err != nil {
		return "", err
	}
	var payload struct {
		Result struct {
			Panes []struct {
				PaneID  string `json:"pane_id"`
				Focused bool   `json:"focused"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return "", fmt.Errorf("could not parse pane list: %w", err)
	}
	panes := payload.Result.Panes
	for _, p := range panes {
		if p.Focused {
			return p.PaneID, nil
		}
	}
	if len(panes) > 0 {
		return panes[0].PaneID, nil
	}
	return "", fmt.Errorf("no panes reported by herdr")
}

func paneText(paneID string, lines int) (string, error) {
	// recent-unwrapped matters for shell panes: it makes herdr rejoin URLs that
	// the terminal split across wrapped lines, which is the one thing
	// extract_url was genuinely better at than a naive regex.
	//
	// For agent panes it is the bug. Agents like Claude Code run on the
	// alternate screen, which has no scrollback, so herdr can only satisfy a
	// deep recent-unwrapped read by "harvesting": it injects real wheel-scroll
	// events into the pane, reads what appears, and scrolls back down
	// (alt_screen_read.rs, herdr issue 2669). On herdr 0.8.0 that takes up to
	// 12 seconds, during which the agent's UI visibly scrolls up under the
	// popup, exactly as if the pane had jumped. It only happens when the agent
	// is idle, because herdr refuses to harvest a busy pane, which is why the
	// scroll seemed to spare panes that were streaming.
	//
	// The visible screen reads instantly and never touches the pane, at the
	// cost of only seeing what is on screen. For a picker that is the right
	// trade: links you cannot see are rarely the ones you want to open.
	source := sourceFor(paneAgent(paneID))
	return herdr("pane", "read", paneID,
		"--source", source,
		"--lines", strconv.Itoa(lines),
		"--format", "text",
	)
}

// sourceFor picks the pane read source: full unwrapped history for normal
// panes, the visible screen for alternate-screen agents, where a history read
// would wheel-scroll the agent's UI in front of the user.
func sourceFor(agent string) string {
	if agent != "" {
		return "visible"
	}
	return "recent-unwrapped"
}

// paneAgent returns herdr's detected agent for the pane, "" for a plain pane
// or when the lookup fails. Failure must never break the picker, so it just
// means the pane is treated as a normal one.
func paneAgent(paneID string) string {
	out, err := herdr("pane", "get", paneID)
	if err != nil {
		return ""
	}
	var payload struct {
		Result struct {
			Pane struct {
				Agent string `json:"agent"`
			} `json:"pane"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(out), &payload) != nil {
		return ""
	}
	return payload.Result.Pane.Agent
}

func extract(text string) []string {
	matches := urlRe.FindAllString(text, -1)

	seen := make(map[string]bool, len(matches))
	ordered := make([]string, 0, len(matches))

	for _, raw := range matches {
		url := strings.TrimRight(raw, trailing)

		// Keep a closing paren that belongs to the URL, e.g. wiki_(disambiguation)
		if strings.Count(url, "(") > strings.Count(url, ")") && strings.HasSuffix(raw, ")") {
			url += ")"
		}

		if len(url) < 8 || seen[url] {
			continue
		}
		seen[url] = true
		ordered = append(ordered, url)
	}

	// Most recent output is at the bottom, and that is usually what you want.
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	return ordered
}

func pick(urls []string) (key, url string, err error) {
	fzf, err := exec.LookPath("fzf")
	if err != nil {
		return "", "", fmt.Errorf("fzf is required but was not found on PATH")
	}

	cmd := exec.Command(fzf,
		// Nerd Font glyphs: the config assumes Iosevka Nerd Font, which every
		// terminal in this flake uses.
		"--prompt= search ",
		"--pointer=",
		"--height=100%",
		"--reverse",
		"--no-multi",
		"--expect=ctrl-y",
		// No border and no label on purpose. This runs as a herdr plugin pane,
		// which herdr already frames and titles "URLs" from the manifest, so
		// fzf drawing its own bordered box inside that nests two frames and shows
		// the same title twice.
		"--info=inline",
		// Wrap around at both ends, so down on the last entry lands on the
		// first and up on the first lands on the last.
		"--cycle",
		// The movement keys are fzf's own defaults (arrows, plus ctrl-j down and
		// ctrl-k up); they are only spelled out here so the picker says so.
		fmt.Sprintf("--header=↑↓ ctrl-j/k  move   ⏎ open   ctrl-y copy   (%d found)", len(urls)),
	)
	cmd.Stdin = strings.NewReader(strings.Join(urls, "\n"))
	cmd.Stderr = os.Stderr // fzf draws on /dev/tty, but let its errors through

	out, err := cmd.Output()
	if err != nil {
		// 1 is "no match", 130 is "user pressed esc or Ctrl-C". Neither is a failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("fzf failed: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	if scanner.Scan() {
		key = strings.TrimSpace(scanner.Text())
	}
	if scanner.Scan() {
		url = strings.TrimSpace(scanner.Text())
	}
	return key, url, nil
}

func open(url string) error {
	if runtime.GOOS == "darwin" {
		return spawn("open", url)
	}
	if path, err := exec.LookPath("xdg-open"); err == nil {
		return spawn(path, url)
	}
	if path, err := exec.LookPath("gio"); err == nil {
		return spawn(path, "open", url)
	}
	return fmt.Errorf("no xdg-open or gio found to open the URL")
}

// spawn starts a detached process so the popup can close immediately.
//
// Setsid is what actually detaches it. Release only drops Go's handle on the
// child; it leaves it in this process group, so when herdr tears the popup down
// and signals the group the handler dies with us. That is why Enter appeared to
// do nothing while Ctrl-Y worked: copyToClipboard waits, and wl-copy forks a
// daemon of its own, but xdg-open was killed before it reached the browser.
func spawn(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func copyToClipboard(url string) error {
	for _, candidate := range [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"pbcopy"},
	} {
		path, err := exec.LookPath(candidate[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, candidate[1:]...)
		cmd.Stdin = strings.NewReader(url)
		return cmd.Run()
	}
	return fmt.Errorf("no wl-copy, xclip or pbcopy found to copy the URL")
}

// pause keeps the popup readable before herdr tears it down.
func pause() {
	if !isPopup() {
		return
	}
	fmt.Print("press enter to close ")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// isPopup is a best-effort check for "herdr will close this window the moment
// we exit", so running the binary from a normal shell does not block.
func isPopup() bool {
	return os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_SESSION") != ""
}
