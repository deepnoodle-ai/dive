# CLI Direction: Phased Plan

_Last updated: 2026-09-02_
_Status: proposal. Execution plan sequencing [CLI Managed Screen](cli-managed-screen.md)
(the design) and [CLI Real-Terminal Testing](cli-real-terminal-testing.md) (the way we
verify it). Those two documents stay the reference; this one says what gets built, in
what order, and what has to be true before each step starts._

## The direction

Move `dive`'s interactive CLI from inline, scrollback-preserving rendering to a
fully-managed alternate screen: the transcript becomes application state rendered into a
virtualized, scrollable viewport, with in-app selection and copy replacing what the
terminal used to provide.

The migration is mostly mechanical. What makes it a multi-phase project rather than a
patch is that three things must be true before the first managed frame is worth
looking at: the per-frame cost has to be O(viewport) instead of O(transcript), wonton
has to expose a viewport and a mouse mode it does not expose today, and we need a way
to verify real terminal behaviour that the simulated tests structurally cannot reach.

Three of the seven phases below ship user-visible value on their own and none of them
depend on the managed screen. That is deliberate: the project should be abandonable at
the end of Phase 3 with everything before it still worth having.

## Decisions taken

The managed-screen design left six questions open and recommended an answer to each.
Taking the recommendations, so the phases below have something concrete to build:

| #   | Question                        | Decision                                                                                                                                                                                            |
| --- | ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Default or opt-in?              | Opt-in behind `--screen` / `DIVE_SCREEN=1` for one release; flip in Phase 6 with `--inline` as the escape hatch; delete inline in Phase 7.                                                          |
| 2   | Where does virtualization live? | In wonton. `Measure` is required regardless, and `Viewport` is the only way to get an exact viewport height without guessing. The app-private fallback is not built.                                |
| 3   | Search and copy in v1?          | Copy yes (drag-to-copy, `/copy`, `/mouse`) — without it the change is a regression for everyone who copies. Search no; `/scrollback` hands the transcript back to the terminal and covers the need. |
| 4   | Exit dump policy                | Full transcript, static render at current width, capped at the last 2,000 lines, then the resume line. No flag in v1.                                                                               |
| 5   | Streaming markdown              | Re-render as markdown on every flush. Add the prefix/tail split only if it shows up in a profile.                                                                                                   |
| 6   | Copy on release?                | Yes, default on, behind a `copy_on_select` setting. When off: drag highlights only, `/copy` sends the highlight, Ctrl+C with a selection copies instead of cancelling.                              |

Two more decisions this plan adds:

7. **The wonton additions land as two tagged releases**, not one: v0.0.40 carries
   everything a read-only managed screen needs, v0.0.41 carries selection and
   clipboard. This lets Phase 4 start while Phase 5's wonton work is still in review.
8. **The real-terminal harness is built before the migration, not after** — the reason
   being that it produces a recorded baseline of the _current_ inline app, which is the
   only artifact that can tell us whether the managed screen is better or merely
   different.

## Settled since

- **Kitty under tmux.** Keep the inline app's unconditional enable. The runtime now
  takes `SetKittyKeyboard(true)` (wonton v0.0.40), which skips the probe the way
  `WithInlineKittyKeyboard` always has, so Shift+Enter behaves the same inside and
  outside tmux and startup does not wait on a reply that tmux will not send. See the
  managed-screen design, appendix E.
- **Decision 1 was revised: `--screen` never shipped.** Rather than a release of
  opt-in, Phase 5 was pulled forward and the flag deleted in the same batch — the
  managed screen is the default, `--inline` is the way out. What that trades away is
  the release of dogfooding decision 1 bought; what it buys is not asking users to
  learn a flag we intended to delete two releases later. The rollout matrix
  (Phase 6.4) therefore runs against the default rather than against an opt-in, and
  is the gate on the release rather than on the flip.
- **`copy_on_select` is `DIVE_COPY_ON_SELECT=0`, not a settings key.** The CLI reads
  no settings file at all today — `experimental/settings` has no caller — so a JSON
  key would have meant wiring the loader in as a side effect of a copy feature. It
  sits with `DIVE_CLIPBOARD` and `DIVE_DISABLE_MOUSE` until something else needs
  settings.
- **The paste placeholder collapses any multi-line paste**, not the ~3 lines or 800
  characters the plan named: the threshold lives in wonton's `textInput`, and the
  bound value stays the real text either way. A file drop is one line and is never
  collapsed, so the dropped-file scan still sees what it needs.

## Still open

- **`/copy`'s picker slipped.** `/copy` with no selection lists the last reply's code
  blocks and `/copy N` takes one, which needs no select-list dialog and is what
  shipped. The interactive picker is still the better shape.
- **`--exit-transcript=full|turn|none`.** Add only if dogfooding asks for it.
- **Phase 6's rollout matrix is not started.** It is now the last thing between this
  work and a release, since the flip it was meant to gate has already happened. The
  kill criterion still stands: if copy does not work on iTerm2, Terminal.app and one
  SSH session without a mode toggle, the default goes back.

## Sequencing

```
Phase 1  dive, inline        frame hygiene + one render path       ──┐
Phase 2  new package         real-terminal harness + baseline      ──┼─ can overlap
Phase 3  wonton v0.0.40      Measure, Viewport, mouse, Suspend     ──┘
Phase 4  dive, --screen      managed screen, read-only                 needs 1 + 3
Phase 3b wonton v0.0.41      selection, cell read-back, OSC 52          can overlap 4
Phase 5  dive, --screen      selection, copy, /copy, /mouse             needs 4 + 3b
Phase 6  dive                exit dump, /scrollback, rollout, flip      needs 5 + 2
Phase 7  dive                delete the inline path                     one release later
```

Phases 1, 2, and 3 are independent of each other and of the migration decision. Phases
4–7 are strictly sequential.

---

## Phase 1 — Frame hygiene and one render path

**Goal:** remove the two things that make any frame-budget number meaningless, and
collapse the duplicate rendering, entirely within the inline app. No user-visible
change except that the CLI gets faster.

**Work:**

1. **Cache the git branch.** `detectGitBranch` (`app.go:2961`) is called from
   `statusLineView` (`render.go:58`) on every frame — ~5.4 ms of `git rev-parse`, 30
   times a second, a sixth of the frame budget spent before a cell is drawn. Refresh on
   a 5 s tick or at `handleProcessingEnd`; never from `View()`.
2. **Collapse the render paths.** `messageView` / `textMessageView` / `toolCallView`
   (live, plain text) and `messageViewStatic` / `textMessageViewStatic` /
   `toolCallViewStatic` / `todoListViewStatic` (`render.go:778–862`, scrollback,
   markdown) become one `messageView(msg, viewOpts{animate, expanded})`. About 150
   lines of near-duplicate code goes away, and the two paths stop drifting.
3. **Make `a.messages` the only output channel.** Replace the 36 `runner.Print*` sites
   (32 in `app.go`, 4 in `context_demo_ui.go`) with appends: `appendSystem`,
   `appendNotice`, and a `report` message kind carrying a pre-built `tui.View` for
   `/usage`, `/help`, `/todos`, `/context`. In inline mode these print to scrollback
   exactly as before, so the step is invisible.
4. **Add `touch(i)`** at the ten in-place mutation sites (`flushStreamBuffer`,
   `flushThinkingStreamBuffer`, `handleToolResult`, `handleToolStream`,
   `handleToolProgress`, the todo and expand paths) with a `Rev uint32` on `Message`.
   It forwards to nothing yet; in Phase 4 it becomes `viewport.Invalidate(i)`.
5. **Introduce the `uiRunner` interface** (`SendEvent`, `Stop`) and change
   `app.runner` (`app.go:257`) from `*tui.InlineApp` to it, so tests can inject a
   recording fake and Phase 4 can drop in `*tui.Runtime`.

**Exit criteria:**

- `go test ./...` green; `TestAppWithInlineRunner` passes against the fake runner.
- No `exec.Command` reachable from `View()` / `LiveView()`.
- One `messageView`; the `*Static` family is deleted.
- `BenchmarkView200` exists and is recorded as the pre-migration number.

**Estimate:** 1–1.5 days. Ships on its own; no dependency on anything else here.

---

## Phase 2 — Real-terminal harness and the inline baseline

**Goal:** be able to drive the real binary in a real iTerm2 window with real input, and
capture what a user would see. Then record the inline app doing it, so the migration has
something to be measured against.

Phase 0 of the testing doc is done — all six spike questions answered on 2026-09-02,
plus seven findings the plan had not anticipated. The throwaway spike code
(`uievent.swift`, the AppleScript driver, `fakellm`, the decoder probe) is the starting
point, not something to redo.

**Work:**

1. **`uievent` Swift helper** (~150 lines, `swiftc` on first use, cached): `key`,
   `type`, `click`, `drag`, `wheel`, `window`, `frontmost`, `pointer`, `doctor`. Post
   with `CGEventPostToPid` by default — verified to work while iTerm2 is in the
   background, so the harness never steals focus. `Wheel()` must warp the pointer,
   post a synthetic `mouseMoved`, clear `ev.flags`, and post one event per tick;
   anything less produces a collapsed event with a spurious Ctrl modifier.
2. **Go `uitest` package** behind `//go:build iterm2`: window lifecycle through
   `osascript` (the AppleScript window `id` _is_ the `CGWindowID`, so no title
   matching), the `dive-uitest` dynamic profile, focus gate, dead-man's switch,
   `WaitFor` / `WaitStable` (no sleeps), `Snap`, `contents` → `termtest.Screen`, and the
   `index.html` contact sheet. `doctor` gates the run: without Screen Recording,
   `screencapture -R` returns wallpaper at exit 0, which is worse than an error.
3. **`fakellm`** — Anthropic-format SSE on `httptest.Server`, reached through
   `DIVE_API_ENDPOINT`, with `Pace`, `HoldAfterChunks`, `Release`, and scripted
   `tool_use`. This is what makes "resize while streaming" a reproducible state.
4. **`DIVE_DEBUG_EVENTS=<path>` in `dive`** — one JSON line per event at the top of
   `HandleEvent` (`app.go:773`), plus a `metrics` line at exit from
   `terminal.GetMetrics()`. Same variable serves the managed screen's `DIVE_DEBUG_FRAMES`.
   This is the _only_ change to `dive`; `DIVE_TTY_LOG` is not needed (iTerm2's automatic
   session logging is byte-exact from a dynamic profile).
5. **Scenarios 1–10** against the inline app, artifacts committed as the baseline.
6. **Never SIGTERM the subject.** A killed app leaves the Kitty stack pushed and mouse
   reporting on, and iTerm2 then injects a notification banner that shifts content and
   invalidates every capture. Close the session; start each scenario by popping the
   Kitty stack and asserting `CSI ? u` replies `\x1b[?0u`.

**Exit criteria:**

- `make uitest` runs scenarios 1–10 green on the dev Mac; `go test ./...` unaffected
  without the tag.
- Baseline artifacts for the inline app: screenshots, text, scrollback, events, metrics.
- A `docs/guides/experimental/` page on running it and granting the two TCC permissions.

**Estimate:** ~2 days (Phase 1 ≈1 day for scenarios 1–5, Phase 2 ≈½ day for 6–10, plus
the doc). Not a CI job and never will be — TCC permissions cannot be granted on a
runner. The simulated tests stay the CI layer.

---

## Phase 3 — wonton v0.0.40

**Goal:** everything a read-only managed screen needs. Separate repo
(`github.com/deepnoodle-ai/wonton`); the local checkout is at v0.0.38 and needs a pull
to v0.0.39 before starting.

**Work:**

- **`tui.Measure(v View, maxWidth int) (w, h int)`** — one line over `v.size()`.
  Required regardless of the rest: the footer's height depends on wrapped input,
  attachments, and autocomplete, and there is no other way to get it.
- **`tui.Viewport` + `ViewportState`** — bottom-anchored virtualized list, scroll state
  in an app-owned struct, `(item, line)` anchor plus `Follow`, per-item view+height
  cache keyed on render width, `flex() == 1` so `Stack` gives the footer its natural
  height. ~200 lines plus table tests over a `[]int` of heights.
- **`Terminal.EnableMouseDrag()`** (`?1006` + `?1000` + `?1002`) plus
  `WithMouseButtons` / `WithMouseDrag` run options, and `?1002l` in
  `DisableMouseTracking`. Motion-while-held only; nothing here needs hover.
- **Click counting in `Runtime.processMouseEvent`** (`runtime.go:622`) — same cell
  within 500 ms, filling the `ClickCount` field `MouseEvent` already declares.
- **Synchronized output** (`\x1b[?2026h`/`l`) around `Terminal.flushInternal`, as
  `LivePrinter.update` already does.
- **`Runtime.Suspend(fn)`** — leave the alternate screen, release the mouse, show the
  cursor, run `fn`, re-enter and repaint. Has to be a runtime method because the input
  goroutine owns stdin. Needed by `/scrollback` in Phase 6.

**Exit criteria:** scroll-math table tests green; `examples/tui/claude` migrated to
`Viewport` and demonstrably still following the bottom as content grows (it silently
stops today); tagged v0.0.40; `dive` bumped.

**Estimate:** 2–3 days including tests.

---

## Phase 3b — wonton v0.0.41

Can start once v0.0.40 is tagged and overlap with Phase 4.

- **`RenderFrame.Cell(x, y)` and `RenderContext.Cell`** — expose the existing
  `Terminal.GetCell` read through the frame and sub-frames, so a view can paint a
  highlight over cells its children drew and read the text back. ~30 lines.
- **Selection on `ViewportState`** — `HandleMouse`, `HasSelection`, `SelectedText`,
  `ClearSelection`, `SelectionStyle`; endpoints as `(item, line, col)` so the highlight
  survives scrolling, streaming, and resize. Auto-scroll at the edges. ~150 lines.
- **`clipboard.WriteOSC52(w, text)`** — `\x1b]52;c;<base64>\x07`, capped at 100 KB.
  No tmux DCS passthrough; under tmux the app hands text to tmux instead.

**Exit criteria:** selection unit tests over a hand-built cell grid (wide characters,
trailing spaces, gap rows, reverse drags); tagged v0.0.41.

**Estimate:** 2 days.

---

## Phase 4 — Managed screen, read-only, behind `--screen`

**Goal:** a scrollable managed screen that is complete except for selection and copy.
Dogfoodable, opt-in, with the inline path untouched beside it.

**Work:**

1. **`View()` alongside `LiveView()`**: `Stack(Padding(Viewport(&a.viewport, a).Gap(1)),
a.footerView())`. `footerView` is `app.go:409–530` minus the transcript-ish parts;
   dialogs keep the same footer slot and the same focus IDs.
2. **Manual runtime construction** (`Terminal` + `NewRuntime`, ~25 lines) because
   `tui.Run` does not hand back the `*Runtime` that agent goroutines need for
   `SendEvent`, and because `EnableMouseDrag`, `EnableMetrics`, and the exit-time
   `Close()`-then-`Print` ordering all need the `*Terminal`.
3. **Handle `ResizeEvent`** — there is no handler anywhere in the CLI today.
4. **Wire `touch(i)` to `viewport.Invalidate(i)`** at the ten mutation sites.
5. **Scrolling**: wheel (1 line), PgUp/PgDn, Ctrl+Home/End, Home/End on empty input via
   `OnKey`, Esc-to-bottom when idle and scrolled up, snap-to-bottom on typing and
   submit, and the `↓ N new lines · End to jump` indicator.
6. **`/clear`** becomes `a.messages = a.messages[:0]` + `InvalidateAll()` +
   `ScrollToBottom()` + a fresh intro, replacing `ClearScrollback` and ~40 lines of
   state reset at `app.go:2251`.
7. **TTY guard** — `Runtime.Run` does not refuse a non-TTY the way `InlineApp.Run` does.
   Check stdin _and_ stdout before touching the alternate screen; point piped users at
   `--print`.
8. **`DIVE_DEBUG_FRAMES=1`** → `EnableMetrics` plus `avg 1.8ms · max 9.2ms · 29.7 fps`
   in the status line.

**Exit criteria:**

- 200-message transcript: `View()` + measure + render under 5 ms typical, 10 ms worst,
  under 2 MB allocated per frame — measured with `DIVE_DEBUG_FRAMES` in a real session,
  not only in a benchmark. An `AllocsPerRun` ceiling guards the regression.
- Screen goldens via `renderScreen(app, w, h)` (the old `renderLiveView` with
  `WithHeight` now mandatory).
- Resize while streaming and resize while scrolled up both keep the same message in view.
- `dive < file` and `dive | cat` refused with the `--print` hint and no alternate-screen
  bytes emitted.
- The Kitty-under-tmux question is answered by a scenario and written down.

**Estimate:** 3–4 days.

---

## Phase 5 — Selection and copy

**Status: shipped**, less the `/copy` picker (see "Still open"). Item 8's click
targets needed `ViewportState.ItemAt` in wonton, which is where a click stops being
a screen row and becomes an item.

**Goal:** close the regression the managed screen opens. This is the phase that decides
whether the change is acceptable — Gemini CLI shipped an alternate-screen mode and
reverted its default until copying worked everywhere, which is the clearest available
evidence that copy is the gate.

**Work:**

1. **Drag-to-select with copy on release**, double-click for a word, triple-click for a
   line, auto-scroll at the edges, `Follow` suspended for the duration of a drag so a
   streamed append cannot move the text under the pointer.
2. **The clipboard ladder**: native tool (off the render goroutine, via `tui.Cmd`, plus
   PRIMARY on X11/Wayland) → `tmux load-buffer -w -` when `TMUX` is set → OSC 52 (on the
   render goroutine, capped at 100 KB). `DIVE_CLIPBOARD=osc52` forces the last rung.
3. **Honest feedback**: `Copied 12 lines (pbcopy)` when verifiable,
   `Sent 12 lines to the terminal clipboard (OSC 52)` when not — the app cannot confirm
   an OSC 52 delivery and must not claim it did.
4. **Modifier bypass hint** chosen from `TERM_PROGRAM` (Option in iTerm2, Shift almost
   everywhere else, Fn in Terminal.app), listed in `/help` and the first-run box.
5. **`/mouse`** to toggle reporting (plus `CSI ?1007 l`), `DIVE_DISABLE_MOUSE=1`,
   and the `copy_on_select` setting with its Ctrl+C-copies-instead-of-cancels branch.
6. **`/copy`** — selection, or a picker over the last assistant message's code blocks;
   `/copy N`, `/copy all`. Source text, not cells.
7. **Paste placeholder** — `InputField.PastePlaceholder(true)` above ~3 lines or 800
   characters, with Backspace/Ctrl+W removing it as a unit and `handleInputChange`'s
   dropped-file detection still seeing the real text.
8. **Click actions**: tool-call header toggles output, `report` title collapses, the
   scroll indicator jumps to bottom.

**Exit criteria:**

- Runtime-level tests: scripted SGR bytes through the input source produce the reverse
  attribute on exactly the selected cells, and the injected clipboard function receives
  the expected text. Clipboard access goes through a field on `App` so no test touches
  the real clipboard.
- Harness scenarios: drag inside one message; across three with auto-scroll; while the
  last message streams (the highlight must not drift); Option-drag yields iTerm2's own
  selection; over SSH the copy lands locally.
- Bracketed-paste newlines arrive as CR, not LF — any code splitting on `\n` is wrong.

**Estimate:** 3–4 days.

---

## Phase 6 — Exit, escape hatches, rollout, flip

**Status: 1, 2, 3 and 5 shipped** — the exit dump, `/scrollback`, Ctrl+L and the flip
to `--inline` — pulled forward with Phase 5 so the default never had to move twice.
Item 4, the rollout matrix, is **partly run**: the ten-terminal sweep was cut to what
one Mac can answer, and most of it turned out to be automatable after all. See
"Rollout results" below.

**Work:**

1. **Exit-time transcript dump** after `terminal.Close()` — this ordering is not
   optional: the alternate screen's content lands in scrollback _nowhere_, so a dump
   written before the restore leaves no trace at all. Static render, 2,000-line cap,
   `… earlier messages omitted` above it, then
   `Session <id> saved — resume with dive --resume <id>`. Skipped on error or panic;
   the resume line still prints.
2. **`/scrollback`** and `/scrollback raw` on `Runtime.Suspend` — the answer to Cmd+F
   and to bulk copy, at a fraction of an in-app search's cost.
3. **Ctrl+L full repaint**, and `DIVE_FULL_REPAINT=1` for ConPTY hosts that leave stale
   fragments.
4. **Run the rollout matrix**: iTerm2, Terminal.app, kitty, WezTerm, Ghostty, Alacritty,
   tmux inside one of them, VS Code, Windows Terminal, one SSH session. Record for each:
   the native-selection modifier, whether `?1002` drags carry correct coordinates past
   column 223, whether OSC 52 lands, whether a file drop still pastes its path, whether
   modifier-click still opens an OSC 8 link, and whether `?1007l` stops wheel-to-arrow
   translation. tmux without `set -g mouse on` gets a one-time hint; `tmux -CC` is
   documented as unsupported.
5. **Flip the default**, `--inline` as the escape hatch, CHANGELOG entry naming the
   trade explicitly (scrollback and Cmd+F for scroll, resize, and streaming markdown).

**Exit criteria:** the matrix is filled in, not sampled; the harness scenarios added in
Phase 3 of the testing plan pass; one release ships with `--screen` opt-in before the
flip.

**Estimate:** 2–3 days plus a release cycle of dogfooding.

### Rollout results (2026-09-02, one Mac)

The ten-terminal sweep was scoped down to this machine. The useful discovery is that
the matrix is mostly _not_ manual: driving the built binary under `pty.fork()` and
injecting SGR mouse reports answers every app-side question, and `pbpaste` /
`tmux show-buffer` verify the result. Only the questions about what a _terminal
emulator_ does need a human or an AppleScript window.

App-side, all verified end to end against the real binary on wonton v0.1.0:

| Question                        | Result                                                             |
| ------------------------------- | ------------------------------------------------------------------ |
| Drag selects and copies         | yes — `Copied N lines (pbcopy)`, exact region                      |
| Double-click / triple-click     | word / whole line                                                  |
| Coordinates past column 223     | correct at 300 columns — SGR `?1006` is unbounded                  |
| tmux rung                       | `Copied N lines (tmux)`, right text in the buffer                  |
| OSC 52 rung, forced             | one write, correct base64, reported as _Sent_ not _Copied_         |
| OSC 52 rung, natural fallback   | reached with no clipboard tool and no `$TMUX`                      |
| `/mouse` off, then on           | both directions; a drag while off copies nothing                   |
| `?1007l` / `?1007h`             | one each — we only ever restore a mode we set                      |
| Mode balance                    | `?1049` `?1002` `?1006` `?2004` `?25` all N/N                      |
| `/copy`, `/copy N`, `/copy all` | list, single block, all blocks; source text, tabs intact           |
| `/copy 9` out of range          | `No code block 9 — the last reply has 2.`                          |
| `/scrollback raw`               | labelled transcript, fences intact, Enter returns, modes rebalance |
| Exit dump + resume line         | after the alt-screen restore, in scrollback                        |

**The one bad finding — OSC 52 is off by default on this Mac.** Probed by emitting the
sequence in each terminal and reading `pbpaste`, with a marker file proving the probe
ran:

| Terminal     | Honours OSC 52 out of the box                                          |
| ------------ | ---------------------------------------------------------------------- |
| Terminal.app | **no** (no support at all)                                             |
| iTerm2       | **no** — `AllowClipboardAccess` is unset, i.e. off                     |
| Ghostty      | not established; its default `clipboard-write` is `ask`, so it prompts |

This does not affect local use: on macOS `pbcopy` is rung 1 and copy works everywhere.
It bites in exactly one place — **dive over SSH on a remote host, from Terminal.app or
a default iTerm2**. There the ladder correctly falls to OSC 52, honestly reports
"Sent … (OSC 52)" rather than claiming a copy, and nothing lands. `/scrollback` is the
working escape hatch in that case, which is an argument for naming it in the notice.

**Not covered here:** kitty, WezTerm, Alacritty, VS Code, Windows Terminal, a real SSH
hop (sshd is not running on this machine), file-drop paste, OSC 8 modifier-click, and
whether `?1007l` actually stops wheel-to-arrow translation in each emulator — that last
one is an emulator behaviour, not an app one, and iTerm2's `AlternateMouseScroll` is off
by default anyway.

**Kill criterion:** not triggered. Copy works unassisted on iTerm2, Terminal.app and
Ghostty locally. The SSH case fails silently-but-honestly rather than wrongly, and has a
documented way out.

---

## Phase 7 — Delete the inline path

One release after the flip: remove `LiveView()`, the `print…ToScrollback` helpers, the
`ClearScrollback` call site, and `--inline`. The session picker
(`session_picker.go:79`) stays an `InlineApp` — it runs to completion before the
alternate screen is entered, which is harmless.

**Estimate:** half a day.

---

## Critical path and total

| Phase                  | Days  | Blocks                |
| ---------------------- | ----- | --------------------- |
| 1 — frame hygiene      | 1–1.5 | 4                     |
| 2 — harness            | 2     | 6 (and de-risks 4, 5) |
| 3 — wonton v0.0.40     | 2–3   | 4                     |
| 3b — wonton v0.0.41    | 2     | 5                     |
| 4 — managed screen     | 3–4   | 5                     |
| 5 — selection and copy | 3–4   | 6                     |
| 6 — rollout and flip   | 2–3   | 7                     |
| 7 — delete inline      | 0.5   | —                     |

Critical path is 1 → 3 → 4 → 5 → 6 → 7, about 12–16 working days, with 2 and 3b
absorbed in parallel. Add a release cycle of dogfooding between 6's matrix and 6's flip.

## Kill criteria

The project should stop, not slip, if any of these hold:

- **After Phase 4:** the managed screen cannot hold 10 ms worst-case frames on a
  200-message session on the target hardware. Virtualization was supposed to buy a
  70× margin; if it does not, the design is wrong rather than under-optimized.
- **After Phase 5:** copy does not work on iTerm2, Terminal.app, and one SSH session
  without a mode toggle. That is the bar Gemini CLI failed to clear, and shipping under
  it trades a working terminal affordance for a broken app one.
- **At any point:** the wonton additions turn out to be unwelcome in wonton. The
  app-private fallback exists on paper but is explicitly not something to ship — an
  exact viewport height computed as `termHeight − footerHeight` is one off-by-one away
  from a cut-off bottom row, and screen-coordinate selection has to be cleared on every
  scroll and every streamed append.

Phases 1 and 2 keep their value in all three cases.
