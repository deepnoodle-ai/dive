# CLI Real-Terminal Testing

_Last updated: 2026-09-02_
_Status: proposal. Phase 0 spikes were run on 2026-09-02 with throwaway code and all six
questions are answered — see [Phase 0](#phase-0-spikes--run-2026-09-02-all-six-answered);
the harness itself is not written. Companion to [CLI Managed Screen](cli-managed-screen.md):
the scenarios here are the executable form of that document's Appendix J (test plan)
and Appendix K (rollout checklist). Environment facts below were checked on the
development Mac on the date above: iTerm2 3.6.11, macOS 14.6, Apple Silicon, Retina
display._

A harness that runs the real `dive` binary inside a real iTerm2 window, drives it with
real keyboard and mouse events, and captures what a user would see: pixel screenshots,
the terminal's own text buffer, the bytes the app emitted, and the events the app
decoded. It exists to answer the questions the simulated tests (`renderLiveView` in
`app_interactive_test.go`, `termtest`) structurally cannot.

## Summary

Feasible, cheap, and worth building **before** the managed-screen migration rather than
after it, so the current inline app gets a recorded baseline that the migration is then
measured against.

- **Everything needed is already on this Mac except the harness itself.** AppleScript
  control of iTerm2 works today with no setup (verified: read the current session's
  rows, columns, profile, and full contents). Accessibility permission is granted for
  processes launched from iTerm2, and OS-level key, mouse and wheel events were
  confirmed to reach it. Screen Recording was granted during Phase 0, which window
  screenshots need; it took effect without restarting iTerm2. The Swift toolchain is
  present. No Python packages are required.
- **Recommended shape:** a Go test package behind a build tag that controls iTerm2
  through `osascript`, posts input through a ~150-line Swift helper (`CGEvent`),
  captures with `screencapture` and iTerm2's session buffer, and runs the CLI against a
  scripted Anthropic-format server on localhost via the existing `DIVE_API_ENDPOINT`.
  One small addition to `dive`: a decoded-event log.
- **It verifies three things nothing else can.** What iTerm2 actually sends for a key
  chord or wheel tick under the modes the app enables, and whether the app decodes it
  as intended. How a frame looks in a real font, with real wide glyphs (`⏺`, emoji),
  real colours, and real timing. What happens at the boundaries: resize mid-stream,
  alternate-screen entry and exit, scrollback after exit, the shell prompt after a
  panic.
- **Not a CI job.** A run takes over the keyboard and screen for a few seconds per
  scenario and depends on TCC permissions that macOS CI runners cannot grant. It is a
  developer-run tool (and one Claude can run and read the screenshots from in a
  session). The CI-safe layer remains the simulated tests.
- **Estimate:** [Phase 0](#phase-0-spikes--run-2026-09-02-all-six-answered) is done —
  it cost about an hour, not half a day, and retired all six unknowns plus seven the
  plan had not anticipated. About two days remain for the harness, the fake model, and
  the first ten scenarios; the calibration app and the `DIVE_TTY_LOG` change both drop
  out of scope.

## What the simulated tests cannot tell us

The existing harness renders `LiveView()` through `tui.Fprint` into a `termtest.Screen`
(`app_interactive_test.go:209`). It is fast, deterministic, and blind to:

1. **Input encoding.** Whether iTerm2 honours the Kitty keyboard protocol the inline app
   enables unconditionally (`app.go:2004`), what bytes it sends for Shift+Enter, PgUp,
   Ctrl+Home, or a wheel tick, and whether wonton's decoder turns those bytes into the
   event the app expects. The managed-screen design lists Shift+Enter under tmux and the
   "wheel becomes arrow keys" behaviour as things to _verify_; this is the only way to
   verify them.
2. **Appearance.** Glyph widths in the actual font (the simulator trusts `runewidth`),
   colour rendering in the user's theme, the hardware cursor, tearing during a page
   scroll, flicker from unclosed markdown fences while streaming. A managed screen
   repaints every cell, so these matter more than they do today.
3. **Boundaries.** What the terminal shows after `\x1b[?1049l` (alternate screen off),
   whether the exit-time transcript dump lands in scrollback, whether the prompt is
   intact and free of stray bytes after Ctrl+C twice or a panic, what a resize does to
   inline scrollback (the known cost the design accepts).
4. **Timing.** Real 30 FPS on a real pty, with GC. The design's benchmarks measure
   render cost in isolation; only a real session shows whether the result is smooth.
5. **Terminal-native behaviour** the app cannot see: Option-drag selection and
   copy-on-select while mouse reporting is on, iTerm2's own key bindings intercepting a
   chord, OSC 52 gated behind a preference.

## Design

### Layers

```
go test -tags iterm2 ./experimental/cmd/dive/uitest
  │
  ├─ iTerm2 control ── osascript        new window with the dive-uitest profile, write text,
  │                                     read contents (screen + scrollback), rows/columns, select, close
  ├─ OS input ──────── uievent (Swift)   CGEvent key / type / click / drag / wheel;
  │                                     window id + bounds, frontmost app, pointer save/restore
  ├─ capture ───────── screencapture     PNG of the window by CGWindowID
  │                    contents          → termtest.Screen for text assertions
  │                    session.cast      iTerm2 automatic session logging → bytes with timing
  │                    events.log        what dive decoded (new DIVE_DEBUG_EVENTS)
  ├─ model ─────────── fake Anthropic    SSE server in the test process; DIVE_API_ENDPOINT
  └─ subject ───────── dive              unmodified binary; temp HOME, temp git repo, fixed shell
```

**Why AppleScript rather than the iTerm2 Python API.** The Python API is the richer,
actively developed surface (screen-change streaming, profile control, per-session
variables), but it has to be enabled in iTerm2's preferences, prompts for
authorisation on first connect, and needs the `iterm2` and `pyobjc` packages and a
Python toolchain in the loop. AppleScript covers what the harness needs — create a
window with a named profile, `write text … newline no`, `contents`, get and set
`rows` / `columns`, `select`, `close` — works today with no setup, and is called from
Go with `exec`. It costs roughly 100–200 ms per call, which a twenty-step scenario
absorbs without noticing. Switch to the Python API only if polling `contents` proves
flaky; nothing else in the design depends on the choice.

**Why a Swift helper rather than pyobjc or cgo.** All Quartz calls — `CGEvent` posting,
`CGWindowListCopyWindowInfo` for the window id and bounds, `AXIsProcessTrusted` and
`CGPreflightScreenCaptureAccess` for the permission preflight — live in one ~150-line
Swift command-line tool, compiled with `swiftc` on first use and cached. Go stays
cgo-free and the harness stays Go-shaped. Command surface:

```
uievent key <chord>            shift+enter, pgup, ctrl+home, esc, ctrl+c, cmd+v …
uievent type <text>            per-character key events, for the few cases that need them
uievent click <x> <y> [mods]   alt for Option-click
uievent drag x1 y1 x2 y2 [mods]
uievent wheel <dy> <x> <y>     line units; negative = up
uievent window --owner iTerm2 --title <t>   → CGWindowID and bounds (CGEvent coordinate space)
uievent frontmost              → bundle id of the active app
uievent pointer [save|restore]
uievent doctor                 → accessibility=…, screen_recording=…
```

**Why a fake model in the test process.** The Anthropic provider posts to
`DIVE_API_ENDPOINT` verbatim (`providers/anthropic/anthropic.go:1057`), so an
`httptest.Server` in the Go test speaks the same SSE format already fixtured in
`providers/anthropic/stream_retry_test.go` and the CLI runs its real provider client,
real streaming, real tool loop against it. The test controls pacing exactly — hold a
stream open mid-message, resize, release — which is what makes "resize while
streaming" reproducible. Scripted `tool_use` blocks trigger the real permission dialog.
No API spend, no network variance. Use a real model name (`--model claude-sonnet-5`) so
pricing and catalog lookups behave normally.

### Two input paths, on purpose

- **`Type(text)`** writes bytes into the pty via AppleScript `write text`. Fast and
  reliable, but it bypasses iTerm2's keyboard handling entirely — no Kitty encoding, no
  bracketed paste. Use it for prose.
- **`Key(chord)`, `Click`, `Drag`, `Wheel`, `Paste`** post OS-level events with
  `CGEvent`. They traverse iTerm2's key bindings, its keyboard-protocol encoding, and its
  mouse-reporting logic before reaching the pty. Use them for everything in the
  managed-screen design's bindings table (Appendix F). This is the whole point of the
  harness.

Every binding is asserted at three levels: the OS event was posted; `events.log` shows
what `dive` decoded (`key=Enter mods=[shift]`, `mouse=wheel-up x=40 y=10`); the screen
shows the effect. A failure at level two with success at level one is a terminal or
decoder issue; a failure at level three alone is an app bug.

### Determinism

- **A dedicated dynamic profile, `dive-uitest`**, written by the harness to
  `~/Library/Application Support/iTerm2/DynamicProfiles/dive-uitest.json` (idempotent,
  left in place). Fixed font (Menlo 13), default dark colours, cursor blink off, no
  transparency or blur, unlimited scrollback, "close session when done" off,
  automatic session logging on (see [capture](#capture-and-artifacts)), and
  "applications in terminal may access clipboard" on so OSC 52 can be tested. Mouse
  reporting and keyboard-protocol settings at iTerm2's defaults, because that is what
  users have. An `Options.Profile = "user"` escape hatch runs a scenario in the
  developer's own profile for eyeball checks.
- **Isolated subject environment.** Temporary `HOME` (the CLI stores sessions under
  `~/.dive/sessions`, `main.go:287`), a temporary git repository on branch `main` so
  the status line is stable, `/bin/zsh -f` with `PS1='$ '` and `clear` before launch,
  `TERM` as iTerm2 sets it. The window is created at a fixed size in cells
  (`set columns` / `set rows`) and a fixed screen position.
- **Screenshots only at rest.** Spinners, elapsed-time counters, and the cursor make
  mid-turn frames non-reproducible. A step captures after `WaitStable`; mid-stream
  captures are allowed but marked as review-only.
- **Volatile-text normalisation** before text goldens: `thinking (12s)` durations,
  session ids, dates, spinner glyphs.

### Synchronization

No sleeps as synchronisation. `WaitFor(substring | predicate, timeout)` polls
`contents` at 50–100 ms; `WaitStable(quiet)` returns when two consecutive reads are
identical for `quiet`. The fake model adds explicit barriers: a scripted response can
`Hold()` after N chunks until the test calls `Release()`, so "the assistant is halfway
through a code block" is a state the test can name.

### Capture and artifacts

Per step, under `experimental/cmd/dive/uitest/out/<TestName>/` (gitignored):

| Artifact         | Source                                               | Use                                                                                                                                                                       |
| ---------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `NN-<step>.png`  | `screencapture -x -o -l <CGWindowID>`                | Visual review. Retina, so 2× pixels. Claude can open these with `Read`, which closes the loop for iterating on rendering in a session.                                    |
| `NN-<step>.txt`  | last `rows` paragraphs of `contents`                 | Text assertions via `termtest.NewScreen(cols, rows)` + `WriteString`; goldens with `-update` in `testdata/`, following the existing convention.                           |
| `scrollback.txt` | everything above the visible rows in `contents`      | "The transcript dump and resume line are in scrollback after exit."                                                                                                       |
| `events.log`     | `DIVE_DEBUG_EVENTS`                                  | Decoded key / mouse / resize events; frame metrics line at exit.                                                                                                          |
| `session.cast`   | iTerm2 automatic session logging in asciicast format | Byte-exact record with timing. Feed to `termtest.Screen` for style-level assertions (`contents` is text-only) and to wonton's `gif.RenderCast` for a shareable recording. |
| `index.html`     | harness                                              | Contact sheet: each step's screenshot, text, and the events decoded since the previous step.                                                                              |

Pixel goldens are **not** asserted: font rasterisation and GPU differences make exact
comparison brittle. When a previous run exists the harness writes a diff image for
review and prints the changed-pixel percentage as information, not as a failure.

`contents` includes scrollback (verified: 521 paragraphs for a 22-row window), which is
what the after-exit scenarios need. On the alternate screen it returns scrolled-off
main-buffer history plus the live alternate screen, and nothing the alternate screen
displayed survives the switch back (Phase 0, spike 2).

### Safety: it drives the same iTerm2 that hosts the developer's session

The harness will usually be launched from an iTerm2 window — Claude Code runs in one
here (`TERM_PROGRAM=iTerm.app`). Posting keystrokes to the wrong window would type into
that session.

- **Always a new window, never the current one.** The harness records the previously
  active window and re-selects it on cleanup.
- **Focus gate before every OS-level input:** the frontmost bundle id must be
  `com.googlecode.iterm2` _and_ iTerm2's `id of current window` must be the harness
  window; otherwise abort the scenario without posting anything.
- **Dead-man's switch:** if the pointer moved between steps by more than a few points
  without the harness moving it, the user has touched the mouse; abort.
- Pointer and clipboard (text) saved before a scenario and restored after.
- Cleanup runs on failure too: artifacts first, then `close` the session (iTerm2 sends
  SIGHUP; no `kill -9`), then restore focus. `DIVE_UITEST_KEEP=1` leaves the window open
  for inspection.
- Runs are short by construction — a scenario budget of 30 s, enforced.
- **`CGEventPostToPid` works while iTerm2 is in the background** (Phase 0, spike 5), so
  the harness should post to iTerm2's pid by default and never steal focus. The gates
  above stay as the fallback for the `post(tap:)` path, and the window must still be
  iTerm2's key window.
- **Never SIGTERM or SIGKILL the subject.** A killed app skips its mode cleanup, leaving
  the Kitty stack pushed and mouse reporting on; the latter makes iTerm2 inject a
  notification banner that shifts content and invalidates captures (Phase 0). Start each
  scenario by popping the Kitty stack and asserting `CSI ? u` replies `\x1b[?0u`.

### Changes to `dive`

1. **`DIVE_DEBUG_EVENTS=<path>`** — append one JSON line per event at the top of
   `HandleEvent` (`app.go:773`): kind, key name, modifiers, rune, mouse button and
   cell, resize size, timestamp; and one `metrics` line at exit from
   `terminal.GetMetrics()` (average and worst frame time, FPS). This is the same
   instrumentation the managed-screen design proposes as `DIVE_DEBUG_FRAMES`; one
   variable can serve both.
2. ~~**`DIVE_TTY_LOG=<path>`**~~ — **not needed.** Phase 0 confirmed iTerm2's automatic
   session logging works from a dynamic profile and is byte-exact, so `dive` needs no
   output tee and the managed-screen runtime needs no `terminal.WithOutput` option. The
   only caveat is that the log is raw, not asciicast (spike 3), so frame timing must
   come from the `DIVE_DEBUG_EVENTS` metrics line.
3. Nothing in the render path. No test-mode branches: the harness must see exactly what
   users see.

### API sketch

```go
//go:build iterm2

func TestShiftEnterInsertsNewline(t *testing.T) {
	m := fakellm.New(t)                       // Anthropic SSE on 127.0.0.1
	s := uitest.Start(t, uitest.Options{Cols: 100, Rows: 30, Model: m})
	defer s.Close()                            // artifacts, close window, restore focus

	s.WaitFor("Type a message")               // intro rendered
	s.Type("first line")                       // pty bytes
	s.Key("shift+enter")                       // real key event
	s.Type("second line")
	s.WaitStable(300 * time.Millisecond)
	s.Snap("two-line-input")                   // PNG + text

	assert.True(t, s.Events().HasKey("Enter", "shift"))
	termtest.AssertContains(t, s.Screen(), "second line")
}
```

```go
m.OnRequest(1).Text("## Hello\n\nA paragraph…").Pace(20 * time.Millisecond).HoldAfterChunks(12)
m.OnRequest(2).ToolUse("bash", `{"command":"echo hi"}`).Then().Text("done")
```

**Cell geometry.** `uievent window` returns the window's bounds in the CGEvent
coordinate space, but the content origin (title bar, margins) and cell size are not
exposed. **No calibration app is needed** (Phase 0): with mouse reporting enabled, two
clicks at known screen points report their own cells in the SGR sequence, which solves
the affine mapping directly — Menlo 13 gave `cell_w=6.944`, `cell_h=16.667`, content
origin `(9.7, 66.7)`. Cache per (profile, cols, rows). Once `dive` itself reports mouse
events (managed screen), its own `events.log` serves the same purpose. Wheel events only
need to land inside the transcript region, so they do not depend on calibration
precision.

### Scenario catalogue

The first ten run against the **current inline app** and become the baseline. Each
names what it asserts and through which channel: **S** screen text, **E** events log,
**P** screenshot for review, **B** scrollback, **C** clipboard, **M** metrics.

| #   | Scenario                                                                                                                                                                                                                                                                         | Asserts         |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| 1   | Launch at 100×30, intro box, input focused; `/exit`; prompt intact, no stray bytes                                                                                                                                                                                               | S, B, P         |
| 2   | Send a prompt; fake model streams three markdown paragraphs and a code block; rendered at rest                                                                                                                                                                                   | S, P            |
| 3   | Shift+Enter inserts a newline. Answered in Phase 0: iTerm2 sends a bare `\n` in _both_ Kitty modes (its own key binding), and wonton maps that to Enter+shift. Guards a decoder accident, not a protocol                                                                         | E, S            |
| 4   | Scripted `tool_use` → permission dialog → approve with real keys → tool row and result                                                                                                                                                                                           | S, E, P         |
| 5   | Ctrl+C twice exits; prompt intact (managed screen later: transcript and resume line in scrollback)                                                                                                                                                                               | S, B            |
| 6   | Resize 100→70 columns while the model is held mid-stream, then release; documents today's mangled scrollback, later asserts a clean re-wrap                                                                                                                                      | P, E (`resize`) |
| 7   | Cmd+V with a 200-line clipboard → bracketed paste → input shows the line count, transcript unmoved                                                                                                                                                                               | E, S            |
| 8   | Wheel over the transcript. Corrected in Phase 0: with reporting off the app receives **nothing**, on the main screen _and_ the alternate screen — wheel-as-arrow-keys is `AlternateMouseScroll`, off by default. With reporting on, `E` shows SGR wheel events at the right cell | E, S            |
| 9   | Option-drag across a rendered message; copy-on-select puts the text on the clipboard                                                                                                                                                                                             | C, P            |
| 10  | `--resume` a seeded 200-message session; PgUp / PgDn / End; worst frame under 10 ms at rest and while streaming; no output gap over 100 ms in `session.cast` during a paced stream                                                                                               | M, E, P         |

Added with the managed screen: scrolled-up indicator and Esc-to-bottom; `/mouse` then a
plain drag-select; `/copy` via `pbcopy` and via OSC 52 with the clipboard preference on
and off; exit dump cap and resume line; the injected-panic path with a readable trace
and a restored terminal; `dive | cat` refused before any alternate-screen byte is
written (this last one is a plain `exec` test and needs no terminal).

### Beyond iTerm2

The terminal-specific surface is small: open at a size, type bytes, read text, resize,
window id, close. Behind a `Terminal` interface:

- **iTerm2** — AppleScript, as above.
- **Terminal.app** — AppleScript too (`contents of selected tab`, `number of columns`);
  the interesting difference is that wonton skips the Kitty probe there.
- **Ghostty** (installed, 1.3.1) — no scripting channel for text, but CGEvent input and
  `screencapture` work unchanged. Text assertions come from `DIVE_TTY_LOG` fed through
  `termtest`, which is style-exact and works for _any_ terminal; the terminal adapter
  shrinks to open, resize, window id, close.
- **tmux inside iTerm2** — `send-keys` and `capture-pane -e` (not installed today).
  Worth a scenario once it is, because the managed-screen design flags Kitty-under-tmux
  as unverified.

### Phase 0: spikes — run 2026-09-02, all six answered

Run on the development Mac (iTerm2 3.6.11, macOS 14.6.1, Apple Silicon, Retina) with a
throwaway `uievent` Swift helper, a wonton-decoder probe, a scripted Anthropic SSE
server, and the real `dive` binary. **The headline result is that the plan works
end to end** — the loop of "drive real iTerm2 → capture → assert" was demonstrated
against the actual CLI, including a screenshot legible enough to review rendering from.
Four of the six answers change the design; they are folded into the sections above and
summarised here.

1. **Screenshots: the grant is required, but no restart is.** With Screen Recording
   _not_ granted, `screencapture -l <id>` fails outright (`could not create image from
window`) and `-R <rect>` silently returns desktop wallpaper with every window
   missing — the dangerous failure, since it exits 0 and writes a plausible PNG. After
   granting once, both work **immediately**: macOS shows a "relaunch to take effect"
   nag, but freshly spawned `screencapture` children are unaffected, so iTerm2 need not
   be restarted. `uievent doctor` (`CGPreflightScreenCaptureAccess`) reports the state
   correctly and must gate the run, because a missing grant otherwise produces
   wallpaper screenshots rather than an error.
2. **`contents` on the alternate screen returns scrolled-off main-buffer history plus
   the live alternate screen** — but _not_ the main screen's on-screen rows, which are
   hidden behind the alternate buffer until `\x1b[?1049l`. After the restore they come
   back intact, and **the alternate screen's own content vanishes completely: none of
   it lands in scrollback.** Consequence for the managed screen: the exit-time
   transcript dump has to be written _after_ leaving the alternate screen, or it leaves
   no trace at all. Text assertions against a managed-screen frame must read `contents`
   while the app is still running.
3. **Automatic session logging works; asciicast does not come from the profile.**
   A dynamic profile with `"Automatically Log": true` and `"Log Directory"` logs
   reliably and the profile's `Columns`/`Rows` are honoured. But `"Logging Style"` in a
   dynamic profile is inert — values 0–4 all produced **raw bytes** — because the style
   is an app-level default (`NoSyncLoggingStyle`), not a per-profile key. iTerm2 3.6.11
   does ship asciicast support (`iTermLoggingHelper`, `queueWriteAsciicastPrologue`),
   but selecting it means mutating a global preference that also affects the
   developer's normal sessions. **Take the raw log:** it is byte-exact, feeds
   `termtest.Screen` directly, and needs no `DIVE_TTY_LOG` change to `dive` — item 2 of
   [Changes to `dive`](#changes-to-dive) can be dropped. The cost is that raw carries no
   timing, so scenario 10's "no output gap over 100 ms" needs its source elsewhere
   (the `DIVE_DEBUG_EVENTS` metrics line).
4. **iTerm2 honours `\x1b[>1u` — but it buys nothing for Shift+Enter.** Measured with
   the Kitty flag state confirmed by `CSI ? u` (`\x1b[?0u` = off) rather than assumed:

   | chord            | flags = 0       | flags = 1 (`\x1b[>1u`) | wonton decodes as    |
   | ---------------- | --------------- | ---------------------- | -------------------- |
   | Esc              | `\x1b`          | `\x1b[27u`             | `Escape`             |
   | Shift+Tab        | `\x1b[Z`        | `\x1b[9;2u`            | `Tab mods=[shift]`   |
   | **Shift+Enter**  | **`\n` (0x0a)** | **`\n` (0x0a)**        | `Enter mods=[shift]` |
   | Ctrl+Shift+Enter | `\r`            | `\x1b[13;2u`           | `Enter mods=[shift]` |
   | Alt+Enter        | `\r`            | `\x1b[13u`             | `Enter` (alt lost)   |
   | Ctrl+J           | `\n` (0x0a)     | `\n` (0x0a)            | `Enter mods=[shift]` |

   So the protocol is real and active, but **Shift+Enter is an iTerm2 key binding that
   intercepts before the protocol encoder** and emits a bare LF in both modes.
   Shift+Enter works in `dive` today only because wonton maps `0x0a` to Enter+shift —
   an accident of the decoder, not the protocol. Two consequences: **Ctrl+J and
   Shift+Enter are indistinguishable** and always will be under this binding, and
   **Ctrl+Enter is unusable** (plain `\r` at flags 0, `\x1b[13;2u` — i.e. shift, not
   ctrl — at flags 1). Verified against the real app: Shift+Enter does insert a newline
   in the input box without submitting.

5. **`CGEventPostToPid` is delivered while iTerm2 is not frontmost — yes.** With Finder
   activated, keys posted to iTerm2's pid arrived at the harness window with Kitty
   encoding intact (`\x1b[27u`, `\x1b[9;2u`), and nothing leaked to Finder. **The
   harness can run without stealing focus**, which makes it far less intrusive than
   [Safety](#safety-it-drives-the-same-iterm2-that-hosts-the-developers-session)
   assumes: the focus gate and dead-man's switch become a fallback for the
   `post(tap:)` path rather than the primary safety mechanism. The target window must
   still be iTerm2's own key window, so "always a new window" and focus restoration
   stay.
6. **Interception map.** Cmd-anything is swallowed as expected (Cmd+Enter produced no
   bytes at all) — except Cmd+V, which iTerm2 turns into a real bracketed paste.
   Everything the design flagged as uncertain is in fact **not** intercepted and
   decodes correctly: PgUp/PgDn (`\x1b[5~`/`\x1b[6~`), Home/End, **Ctrl+Home/Ctrl+End**
   (`\x1b[1;5H`/`\x1b[1;5F`), Ctrl+O, F1–F2, and Shift+arrows. Modifiers are dropped on
   Shift+Space and Ctrl+Backspace.

#### Findings the spikes turned up that the plan did not anticipate

- **iTerm2's AppleScript window `id` _is_ the `CGWindowID`.** `create window` returned
  `8004` and `CGWindowListCopyWindowInfo` listed the same number. No title matching, no
  set-diffing around window creation, and `uievent window` shrinks to a bounds lookup.
  This matters twice over, because **`kCGWindowName` is empty without Screen
  Recording** on macOS 14 — title-based matching would have coupled window identity to
  the same TCC grant as screenshots.
- **Cell geometry needs no `uitest/calib` app.** With mouse reporting on, two clicks at
  known screen points report their cells directly (`\x1b[<0;14;3M` → cell 13,2), which
  solves the affine mapping in one step: Menlo 13 gives `cell_w=6.944`,
  `cell_h=16.667`, content origin `(9.7, 66.7)`. The 40-line calibration app in
  [Cell geometry](#api-sketch) can be deleted from the plan; a least-squares fit over
  three or four points would tighten the ~1-row residual if drag-select ever needs it.
- **Wheel injection is not one call.** A single `CGEvent` scroll with `wheel1: 3`
  yielded _one_ collapsed event carrying a **spurious Ctrl modifier** (`\x1b[<81;...`)
  from inherited flag state. Posting one event per tick with `ev.flags = []`, after a
  `CGWarpMouseCursorPosition` and a synthetic `mouseMoved`, gives the expected clean
  sequence (`\x1b[<64` × 3 up, `\x1b[<65` × 3 down). Any harness `Wheel()` must do all
  four things.
- **Scenario 8's premise is wrong.** With mouse reporting off, the wheel sent **no
  bytes at all** — on the main screen _and_ on the alternate screen. iTerm2 does not
  translate wheel to arrow keys by default; that is `AlternateMouseScroll` ("Scroll
  wheel sends arrow keys when in alternate screen mode"), which is **unset on this
  machine**. The managed screen therefore cannot rely on wheel-as-arrow-keys and must
  enable mouse reporting to receive scroll at all. iTerm2 also has an
  `AskAboutAlternateMouseScroll` prompt, which is another modal hazard.
- **Killing the subject corrupts the next run.** SIGTERM skips the app's mode cleanup,
  which leaves both the Kitty stack pushed (changing every chord encoding for the next
  scenario — this silently confounded the first flags-off run) and mouse reporting on,
  which makes **iTerm2 inject a notification banner** ("Looks like mouse reporting was
  left on…") that shifts terminal content down and invalidates screenshots and text
  goldens. The harness must never SIGTERM/SIGKILL the subject, and must start every
  scenario by popping the Kitty stack and asserting `CSI ? u` returns `\x1b[?0u`.
- **Bracketed paste newlines arrive as CR.** A real Cmd+V of a 200-line clipboard
  produced one `\x1b[200~ … \x1b[201~` event (2892 bytes, decoded as a single paste),
  with the embedded newlines as `0x0d`, not `0x0a`. Code that splits pasted text on
  `\n` sees one line. No multi-line paste confirmation dialog appeared.
- **Terminal query replies surface as junk key events.** wonton's decoder turned the
  `CSI ? u` reply into a `KeyEvent key=Unknown`. Any reply to a capability query the
  app itself did not consume reaches the app as a spurious keypress.

#### What was demonstrated end to end

Against the real binary in a real window, with an isolated `HOME` and the scripted
server on `DIVE_API_ENDPOINT`: launch and intro box (scenario 1); prompt submitted and
three markdown paragraphs plus a syntax-highlighted code fence streamed and rendered at
rest, with real wide glyphs and the token/cost line (scenario 2); Shift+Enter inserting
a newline in the input box without submitting (scenario 3); Ctrl+C showing "Press
Ctrl+C again to exit", the second exiting cleanly to an intact, working shell prompt
with the transcript left in scrollback (scenario 5). Scenarios 4, 6, 7, 9 and 10 were
not run; scenario 7's paste mechanics were verified against the probe rather than the
app.

The spike code (`uievent.swift`, the decoder probe, the AppleScript driver, `fakellm`)
is throwaway but working, and is the starting point for Phase 1 rather than a design
to redo.

### Plan

- **Phase 1 (≈1 day):** Swift helper with `doctor`; Go `uitest` package (window
  lifecycle, focus gate, `Type`/`Key`/`Wheel`, `WaitFor`/`WaitStable`, `Snap`,
  contents → `termtest`, contact sheet); `fakellm`; `DIVE_DEBUG_EVENTS`; scenarios 1–5.
  `make uitest` runs `go test -tags iterm2 ./experimental/cmd/dive/uitest -count=1`;
  without the tag the package compiles to nothing, so `go test ./...` is unaffected.
- **Phase 2 (≈½ day):** calibration, `Click`/`Drag`/`Paste`, scenarios 6–10, text
  goldens, a `docs/guides/experimental/` page on running it and granting permissions.
- **Phase 3 (with the managed-screen work):** the managed-screen scenarios; the
  Appendix K rollout checklist becomes executable wherever a scenario can express it;
  scenario 10 becomes the frame-budget regression guard that the design's benchmark
  cannot be.

### Alternatives considered

- **iTerm2 Python API** — richer, but more setup and a second toolchain; revisit if
  polling proves inadequate.
- **`termsession` + `gif` in wonton** (pty recorder, asciicast, GIF renderer) and
  **`vhs`** — simulated terminals. Deterministic and CI-safe, good for documentation
  GIFs and byte-level regression tests, and `termsession` is already a dependency. They
  cannot answer any of the five questions above, because there is no iTerm2 in the
  loop. Recommended as the CI layer, not as a substitute.
- **`expect` / `pexpect`** — same category, less capable than `termsession`.
- **A pure-AppleScript harness** — workable for scenarios 1, 2, and 5 only; without OS
  events it cannot test a single binding, which is most of the value.
- **macOS CI runners** — Accessibility and Screen Recording cannot be granted
  non-interactively on a runner; out of scope.

### Non-goals

- Pixel-exact golden images.
- Replacing the simulated tests. They stay the fast, CI-safe layer; this harness is the
  slow, honest one.
- Automating terminals without a text channel beyond screenshots plus `DIVE_TTY_LOG`.
- Testing Windows Terminal or Linux terminals from this harness; the `Terminal`
  interface leaves room, the macOS input layer does not.
