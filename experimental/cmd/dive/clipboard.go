package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deepnoodle-ai/wonton/clipboard"
)

// The managed screen takes the terminal's own selection away, so copying has to
// work everywhere the CLI runs — including over SSH, where nothing on this
// machine can reach the user's clipboard and only the terminal itself can.
//
// Three rungs, tried in order:
//
//  1. A native tool: pbcopy, wl-copy, xclip, xsel, clip.exe. Verifiable — the
//     process either exits 0 or it does not. On X11 and Wayland the text also
//     goes to PRIMARY, so middle-click paste works too.
//  2. tmux load-buffer -w -, when $TMUX says we are inside tmux. tmux owns the
//     terminal there, and -w has it forward the text outward as well.
//  3. OSC 52 to the terminal. The only rung that crosses an SSH connection, and
//     the only one that cannot be confirmed: the terminal sends no reply, and
//     plenty of terminals ignore the sequence or need it switched on first.

// clipboardReport is what to tell the user afterwards. verified separates a
// copy we watched succeed from one we merely asked for.
type clipboardReport struct {
	lines    int
	via      string
	verified bool
}

// notice is the user-facing line. An OSC 52 write has to be described as a
// request rather than a result, because that is all it is.
func (r clipboardReport) notice() string {
	if r.verified {
		return fmt.Sprintf("Copied %d line%s (%s)", r.lines, pluralSuffix(r.lines), r.via)
	}
	return fmt.Sprintf("Sent %d line%s to the terminal clipboard (%s)", r.lines, pluralSuffix(r.lines), r.via)
}

// clipboardCopier puts text on the clipboard and says how it went. A field on
// App rather than a package function, so no test forks a process or writes to
// the developer's real clipboard.
type clipboardCopier func(text string) (clipboardReport, error)

// newClipboardCopier returns the real ladder, writing OSC 52 to osc52 (the
// terminal) when it gets that far.
func newClipboardCopier(osc52 io.Writer) clipboardCopier {
	return func(text string) (clipboardReport, error) {
		lines := countLines(text)

		// The escape hatch for a terminal whose native tool is present but
		// wrong — the common case being a multiplexer or a remote shell where
		// $DISPLAY points at a machine the user is not sitting in front of.
		if strings.EqualFold(os.Getenv("DIVE_CLIPBOARD"), "osc52") {
			return writeOSC52(osc52, text, lines)
		}

		var failures []error

		if name, args, ok := nativeClipboardTool(); ok {
			err := runClipboardTool(name, args, text)
			if err == nil {
				copyToPrimary(text)
				return clipboardReport{lines: lines, via: filepath.Base(name), verified: true}, nil
			}
			failures = append(failures, fmt.Errorf("%s: %w", filepath.Base(name), err))
		}

		if os.Getenv("TMUX") != "" {
			err := runClipboardTool("tmux", []string{"load-buffer", "-w", "-"}, text)
			if err == nil {
				return clipboardReport{lines: lines, via: "tmux", verified: true}, nil
			}
			failures = append(failures, fmt.Errorf("tmux: %w", err))
		}

		report, err := writeOSC52(osc52, text, lines)
		if err != nil {
			return report, errors.Join(append(failures, err)...)
		}
		return report, nil
	}
}

// writeOSC52 asks the terminal to set the clipboard itself. Nothing comes back,
// so the report is deliberately unverified.
func writeOSC52(w io.Writer, text string, lines int) (clipboardReport, error) {
	if w == nil {
		return clipboardReport{}, errors.New("no terminal to send the clipboard sequence to")
	}
	if err := clipboard.WriteOSC52(w, text); err != nil {
		return clipboardReport{}, err
	}
	return clipboardReport{lines: lines, via: "OSC 52", verified: false}, nil
}

// nativeClipboardTool picks the clipboard command for this machine, if there is
// one that can actually work.
//
// On X11 and Wayland, being on $PATH is not enough: those tools write to a
// display server, and over SSH with no forwarded display they will happily run
// and put the text somewhere the user cannot reach. Requiring the display
// variable is what makes the OSC 52 rung reachable in the case that needs it.
func nativeClipboardTool() (name string, args []string, ok bool) {
	look := func(bin string, args ...string) (string, []string, bool) {
		path, err := exec.LookPath(bin)
		if err != nil {
			return "", nil, false
		}
		return path, args, true
	}

	switch runtime.GOOS {
	case "darwin":
		return look("pbcopy")
	case "windows":
		return look("clip.exe")
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if n, a, found := look("wl-copy"); found {
				return n, a, true
			}
		}
		if os.Getenv("DISPLAY") != "" {
			if n, a, found := look("xclip", "-selection", "clipboard"); found {
				return n, a, true
			}
			if n, a, found := look("xsel", "--clipboard", "--input"); found {
				return n, a, true
			}
		}
	}
	return "", nil, false
}

// copyToPrimary also puts the text on the X11/Wayland PRIMARY selection, which
// is what a middle click pastes. Best effort: the clipboard proper already has
// the text, so a failure here is not worth a word to the user.
func copyToPrimary(text string) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return
	}
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if path, err := exec.LookPath("wl-copy"); err == nil {
			_ = runClipboardTool(path, []string{"--primary"}, text)
		}
	case os.Getenv("DISPLAY") != "":
		if path, err := exec.LookPath("xclip"); err == nil {
			_ = runClipboardTool(path, []string{"-selection", "primary"}, text)
		}
	}
}

// runClipboardTool feeds text to a command's stdin and waits for it.
func runClipboardTool(name string, args []string, text string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	// A clipboard tool that has something to say is reporting a failure, and
	// its message is more useful than "exit status 1".
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return err
	}
	return nil
}

// countLines counts the lines of copied text the way a user would: the number
// of rows the selection covered.
func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
}

// firstLine is the first line of s, for error text that may run long.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// nativeSelectionModifier is the key held to bypass the app's mouse reporting
// and use the terminal's own selection instead. Terminals disagree and there is
// no way to ask, so this is a table keyed on what identifies the terminal, with
// Shift as the answer almost everywhere else.
func nativeSelectionModifier() string {
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		return "Option"
	case "Apple_Terminal":
		return "Fn"
	}
	return "Shift"
}
