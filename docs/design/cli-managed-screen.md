# CLI Managed Screen

_Last updated: 2026-09-02_
_Status: research complete, no code written. Revised after a second pass over the
`dive` CLI and the wonton source. The main body is the design; the
[Appendix](#appendix-detailed-recommendations) carries concrete API sketches,
algorithms, and checklists. Decisions still open are listed in
[Decisions and open questions](#decisions-and-open-questions)._

Research into moving the `dive` CLI from its current inline, scrollback-preserving
rendering to a fully-managed screen — the app owns the alternate screen buffer and
every cell in it, and the conversation transcript becomes application state rendered
into a scrollable viewport.

Measured against `dive` v1.27.0 and `github.com/deepnoodle-ai/wonton` v0.0.39.

## Summary

The migration is well supported by the TUI library and is mostly mechanical, with two
genuine engineering problems and one prerequisite.

1. **A naive port cannot hold a 30 FPS frame budget on a real-length conversation.**
   `View()` is rebuilt every frame, markdown caching is per-view-instance, and
   `ScrollView` measures its entire inner content regardless of clipping — so cost is
   O(transcript) per frame. Measured at 200 messages: ~50 ms/frame, spiking to 74 ms,
   against a 33 ms budget. Message-level virtualization brings that to 0.7 ms and is
   therefore not an optimization to add later, but a constraint on the design.

2. **Application code cannot build an exact viewport on its own.** Wonton's `View`
   interface has unexported methods (`view.go:11`), so the CLI can neither implement
   a custom view nor ask an existing one how tall it is. Virtualizing the transcript
   needs the exact height of the transcript region, and that is only known inside
   wonton at render time. The clean answer is a small wonton addition — a `Measure`
   helper and a `Viewport` view, see [Appendix A](#a-proposed-wonton-additions). The
   fallback without it works but is fragile.

3. **Prerequisite: the status line shells out to `git` on every frame.**
   `statusLineView` (`render.go:44`) calls `detectGitBranch` (`app.go:2961`), which
   runs `git rev-parse --abbrev-ref HEAD` — about 5 ms per call, 30 times a second,
   in the current inline app and in any managed screen. That has to be cached before
   any frame-budget number means anything, and it is a worthwhile fix on its own.

The real cost of the change is not performance, it is the loss of terminal-native
scrollback, search, and selection. Everything a user might want to find or copy has
to become findable and copyable inside the app.

The sharpest of those edges is mouse selection. Wheel scrolling needs mouse reporting,
and mouse reporting takes drag-select away from the terminal. Claude Code's fullscreen
renderer — its default for new installs since May 2026 — made exactly this trade and
answers it with an in-app selection: drag highlights, release copies to the clipboard,
double-click selects a word, and a per-terminal modifier still gives the terminal's
own selection. The design adopts that behaviour wholesale, with the selection anchored
to transcript content so it survives scrolling and streaming
([Appendix G](#g-copy-paste-and-mouse-interaction)). That needs three small wonton
additions beyond `Viewport`: a mouse mode that reports drags without hover noise, cell
read-back on the render frame, and click counting.

For context, both models ship in production agent CLIs as of this writing. Claude Code
now has both: a classic inline renderer, and a fullscreen renderer on the alternate
screen with a virtualized transcript, mouse support, and in-app selection, which is
the default for new installs since May 2026 and the behaviour this design tracks
([Fullscreen rendering](https://code.claude.com/docs/en/fullscreen)). Codex CLI
went the other way on purpose: an inline viewport with finished history inserted
into the terminal's scrollback through DEC scroll regions, no mouse capture at all,
selection left to the terminal, and the alternate screen only for overlays
(`codex-rs/tui/src/insert_history.rs`, `tui/event_stream.rs:250`). Its history is
instructive: wheel support first broke drag-select ("drag-to-select or scrolling,
but not both", May 2025), an append-only log fixed that structurally in July 2025,
and a later `tui2` experiment with app-managed selection and a draggable scrollbar
was retired. Gemini CLI shipped
an alternate-screen mode and reverted its default until copying works everywhere
without a mode toggle. OpenCode and Crush own the alternate screen with in-app
selection. The choice is about what we are willing to reimplement in-app, not about
what is possible — and Claude Code's fullscreen page is, in effect, the list of what
has to be reimplemented.

## Where the CLI is today

`experimental/cmd/dive` runs on `tui.InlineApp` (`app.go:2001`). `LiveView()` renders a
live region pinned to the bottom of the terminal; finished content is pushed into the
terminal's real scrollback with `runner.Print` / `runner.Printf`.

Surface area that the migration touches:

| Thing                                           | Where                                     | Count |
| ----------------------------------------------- | ----------------------------------------- | ----- |
| `runner.Print*` calls (non-test)                | `app.go`, `context_demo_ui.go`            | 36    |
| `runner.SendEvent` calls                        | `app.go`, `main.go`, `context_demo_ui.go` | 34    |
| Pre-runner `tui.Print` calls                    | `app.go:2030`, `app.go:2100`              | 2     |
| `runner.ClearScrollback`                        | `app.go:2251`                             | 1     |
| Second `InlineApp` (session picker)             | `session_picker.go:79`                    | 1     |
| Places that mutate an existing message in place | `app.go`                                  | 10    |

The `SendEvent` sites carry over unchanged — the full-screen `Runtime` has the same
method with the same goroutine-safety guarantee (`runtime.go:697`). The in-place
mutation sites matter because each one has to invalidate a render cache entry
([Appendix B](#b-transcript-model-and-render-cache)).

Facts about the current app that shape the port:

1. **The transcript is already application state.** `a.messages []Message`
   (`app.go:158`) holds the whole conversation; scrollback is a second, lossy copy of
   it. A managed screen deletes the second copy rather than inventing a new model.
2. **There are already two render paths that must stay in visual sync.**
   `messageView` / `textMessageView` / `toolCallView` (live region, plain text,
   `render.go:309`) and `messageViewStatic` / `textMessageViewStatic` /
   `toolCallViewStatic` (scrollback, markdown, `render.go:778`). Roughly 150 lines of
   near-duplicate code, and the live path deliberately skips markdown because it
   re-renders every frame. A managed screen collapses these into one path.
3. **Streaming text is not shown live today.** `buildLiveView` (`app.go:2613`) renders
   a spinner plus at most four recent tool-call / reasoning rows; the assistant's text
   accumulates in `a.messages` and is printed once at turn end. A managed screen can
   stream the markdown as it arrives.
4. **The app ignores `ResizeEvent` entirely.** There is no handler anywhere in the
   CLI. Inline mode gets away with this because the live region is re-measured each
   frame; a managed screen must react (cache invalidation, viewport width).
5. **Kitty keyboard handling differs between the two runners.** The inline app enables
   the protocol blindly (`inline_app.go:393`); the full-screen `Runtime` probes for it
   (`runtime.go:146`) and skips the probe under tmux, screen, and Apple Terminal
   (`terminal.go:1032`). Shift+Enter behaviour under tmux needs to be verified
   explicitly after the switch.

## What wonton already supports

The target architecture is directly supported. There is a working reference
implementation in the wonton module at `examples/tui/claude/main.go` — a
Claude-Code-shaped full-screen chat — with two caveats noted below.

- `tui.Run(app, opts...)` with the `Application.View()` interface. **Alternate screen
  is on by default** (`run.go:20`).
- `tui.Scroll(content, &scrollY)` — a viewport with an external scroll offset, plus
  `MouseButtonWheelUp` / `WheelDown` events. **Caveat:** `.Bottom()` is honoured only
  when no external offset is supplied (`scroll_view.go:92`), and the view writes the
  clamped offset back into the pointer. So "follow the bottom while content streams"
  is the app's job: set the offset to a large value on every frame while following,
  not once on send as the `claude` example does — that example silently stops
  following as soon as content grows.
- `StackView` honors flex. `Stack(Scroll(transcript), footer)` gives the footer its
  natural height with no manual arithmetic — the `Height(footerHeight, ...)`
  computation in the `claude` example is not necessary.
- Kitty keyboard is **auto-detected** in full-screen mode (`runtime.go:146`), where
  inline mode requires opting in via `WithInlineKittyKeyboard`. Bracketed paste, mouse
  tracking, paste tab width, and backslash-Enter all have `Run` options.
- Rendering is double-buffered with dirty-region diffing, and the render path installs
  the focus manager and view registries (`runtime.go:414`), so `InputField`, focus, and
  animations behave identically to inline mode.
- `Terminal.Close()` (`terminal.go:2136`) performs the entire teardown — mouse
  tracking, bracketed paste, enhanced keyboard, cursor, alternate screen, raw mode,
  style reset — in the right order. After it returns, `tui.Print` writes into the real
  scrollback, which is exactly what an exit-time transcript dump needs.
- An end-to-end test harness exists: `tui.NewTestTerminal(w, h, &buf)` plus
  `tui.NewRuntime` plus `Runtime.SetInputSource`, and `termtest.Screen` parses the
  cursor-positioning sequences the terminal emits (`termtest/ansi.go:218`). Wonton's own
  `runtime_lifecycle_test.go` is the pattern.

Gaps and sharp edges:

- **`tui.Run` does not return the `*Runtime`**, which is needed for `SendEvent` from
  the agent goroutines. Construct `Terminal` + `Runtime` manually instead — about 25
  lines, following `examples/tui/metrics/main.go` and
  [Appendix E](#e-runtime-construction-and-lifecycle). `Runtime` has `SendEvent` and
  `Stop`, but no `Print` / `Printf` / `ClearScrollback` (none of which are needed once
  the transcript is app state).
- **No mouse mode reports drags without hover noise.** `WithMouseTracking(true)`
  enables any-motion tracking (`?1003h`, `terminal.go:1188`): every mouse movement
  becomes an event and a re-render. `Terminal.EnableMouseButtons` (`terminal.go:1205`)
  enables `?1000h` alone, which reports presses and releases but no motion, so a drag
  arrives as a press and a distant release. Button-event tracking (`?1002h` — motion
  only while a button is held) is what a drag selection needs, and nothing enables it.
  The rest of the pipeline is ready: motion with a button held already decodes as
  `MouseDrag` (`terminal/mouse.go:209`), the runtime synthesizes `MouseClick` from a
  press/release pair at the same cell (`runtime.go:622`), and `MouseEvent` carries
  Shift/Alt/Ctrl modifiers and a `ClickCount` field that only the unused
  `MouseHandler` component fills (`tui/mouse.go:333`). Neither mode has a `Run`
  option, which is a second reason to construct the runtime manually.
- **Rendered cells cannot be read back during a render.** `RenderFrame`
  (`terminal.go:236`) is write-only. `Terminal.GetCell` (`terminal.go:2236`) reads the
  back buffer but is documented as a test aid and is not reachable from a view.
  Painting a selection highlight over text a child view has already drawn, and
  extracting that text for the clipboard, both need a read path.
- **No synchronized output in the full-screen flush path.** The inline `LivePrinter`
  wraps updates in DEC 2026 (`print.go:510`); `Terminal.flushInternal` does not. A
  page scroll repaints every cell of the viewport, so tearing is possible on slower
  terminals. Small wonton fix ([Appendix A4](#a-proposed-wonton-additions)).
- **`View` is sealed.** `Canvas` / `CanvasContext` (`canvas_view.go`) is the only
  imperative escape hatch: it receives the exact viewport size but cannot render child
  views into it. This is what makes an exact, app-private viewport awkward.
- **`clipboard` shells out** to `pbcopy` / `xclip` / `wl-copy` and does not implement
  OSC 52, so in-app copy does nothing useful over SSH today.

## The forced constraint: per-frame cost

`View()` is rebuilt from scratch every frame at 30 FPS — the runtime renders on every
tick whether or not anything changed (`runtime.go:322`). `MarkdownView` memoizes its
render and invalidates on width change (`markdown_view.go:58`), but **the cache lives
on the view instance**, which is discarded each frame. And `ScrollView` measures its
entire inner content to compute max scroll (`scroll_view.go:73`) — clipping does not
save the measure.

Why the naive path costs what it does:

- The transcript is measured three to four times per frame, not once.
  `Runtime.render` calls `size()` then `render()`; `StackView.render` re-measures its
  children; `ScrollView.size` measures the inner content twice when unconstrained and
  `ScrollView.render` measures it again.
- `TextView.Wrap()` re-wraps on every measure (`text_view.go:264`); only `MarkdownView`
  has a memo, and only if the instance survives.
- Each `Runtime.render` clears the whole frame (`frame.Fill`) and diffs W×H cells. That
  part is cheap and constant; it is the transcript that scales.

### Measurements

Representative transcript (markdown assistant messages with headings, bullets, inline
code and a fenced code block, alternating with tool-call rows), 100 columns, Apple M1,
Go 1.26, `-benchtime=3s -count=3`. Frame budget at 30 FPS is 33 ms.

| Approach                                     | 200 messages                      | Alloc/frame |
| -------------------------------------------- | --------------------------------- | ----------- |
| Naive rebuild                                | **~50 ms** (45–74 ms across runs) | 76 MB       |
| Cache per-message view objects across frames | **~33 ms** (32–35 ms)             | 59 MB       |
| Virtualize — build only the visible messages | **0.69 ms**                       | 1.1 MB      |

Scaling of the naive path: 10 messages → 2.3 ms, 50 → 10.7 ms, 200 → ~50 ms.

Not in the table, because it is independent of the render path: `detectGitBranch`
costs ~5.4 ms per call (30 sequential `git rev-parse` invocations from a subprocess
harness, same machine) and is called once per frame from `statusLineView`. That is a
sixth of the budget spent before a single cell is drawn, in both modes.

Three conclusions:

- A naive port feels fine in a demo and degrades badly in a real session. The failure
  is gradual, so it will not show up in a quick manual test.
- **Caching alone is insufficient.** At 200 messages the cached path lands right on the
  33 ms budget with zero headroom — and that budget still has to cover the input area,
  status line, spinner animations, and the diff/flush to the terminal. Allocating
  ~59 MB per frame is untenable GC pressure regardless of wall time.
- **The naive path's variance is the tell.** Run-to-run spread of 45–74 ms at a fixed
  workload is GC, not measurement noise, and it will surface as visible stutter during
  streaming.

**Method caveats:**

- Measured via `tui.Fprint` to `io.Discard`, which renders the full content height
  rather than a viewport, so these are an upper bound on the blit portion. The markdown
  parse and wrap in the measure phase dominates, and `ScrollView` pays that in full, so
  the ranking holds.
- Short `-benchtime` runs are badly unreliable here — a 10-iteration run of the naive
  path reported 138 ms, roughly 3× the stable figure. Use `-benchtime=3s -count=3` or
  better.
- Absolute numbers are Apple M1 specific. Re-measure on the target hardware before
  treating any of them as a threshold.

### Benchmark source

Reproduce with a scratch module requiring `github.com/deepnoodle-ai/wonton v0.0.39`:

````go
package main

import (
	"fmt"
	"io"
	"testing"

	"github.com/deepnoodle-ai/wonton/tui"
)

func sampleMsg(i int) string {
	return fmt.Sprintf(`## Step %d

Here is a paragraph of assistant output that wraps across several lines in a
typical terminal, describing what the model did and why it matters.

- bullet one about %d
- bullet two with `+"`inline code`"+`
- bullet three

`+"```go\nfunc f%d() int { return %d }\n```"+`

And a closing sentence.`, i, i, i, i)
}

// buildTranscript mimics rebuilding the whole view tree each frame.
func buildTranscript(n int) tui.View {
	views := make([]tui.View, 0, n*3)
	for i := 0; i < n; i++ {
		views = append(views, tui.Text(""))
		views = append(views, tui.ZStack(
			tui.PaddingLTRB(2, 0, 0, 0, tui.Markdown(sampleMsg(i), nil)),
			tui.Text("⏺"),
		).Align(tui.AlignLeft))
		views = append(views, tui.Group(tui.Text("⏺ "), tui.Text("Bash(go test ./... #%d)", i)))
	}
	return tui.Stack(views...)
}

func benchN(b *testing.B, n int) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tui.Fprint(io.Discard, buildTranscript(n), tui.WithWidth(100))
	}
}

func BenchmarkFrame10(b *testing.B)  { benchN(b, 10) }
func BenchmarkFrame50(b *testing.B)  { benchN(b, 50) }
func BenchmarkFrame200(b *testing.B) { benchN(b, 200) }

// Cached: per-message view objects survive across frames, so MarkdownView's
// width-keyed memo actually hits.
func BenchmarkFrame200Cached(b *testing.B) {
	cached := buildTranscript(200)
	tui.Fprint(io.Discard, cached, tui.WithWidth(100)) // warm the caches
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tui.Fprint(io.Discard, cached, tui.WithWidth(100))
	}
}

// Virtualized: only the messages intersecting a ~40-line viewport are built.
func BenchmarkFrame200Viewport(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tui.Fprint(io.Discard, buildTranscript(3), tui.WithWidth(100), tui.WithHeight(40))
	}
}
````

Run with `go test -bench=. -benchtime=3s -count=3 -run=NONE .` — the default and any
short `-benchtime=Nx` give wildly unstable results for these workloads.

### The design this forces

Virtualize at the message level, and let the viewport own the scroll math:

- Cache, per message, a built view plus its height at the current width. Invalidate
  one entry when that message changes (streaming append, tool result, expand/collapse)
  and all entries on a width change.
- Store the scroll position as an **anchor** — `(message index, line within that
message)` — plus a `follow` flag, not as an absolute line offset. An anchor survives
  resizes and content arriving above or below it, and it means only the messages from
  the anchor forward need measuring to lay out a frame. Total content height is not
  required for anything in the first version.
- Build only the messages whose lines intersect the viewport, cut the top message at
  the anchor line, and draw from there.
- The viewport must know its own exact height. Wonton knows it at render time; the app
  does not, because the footer's height depends on wrapped input text, attachments,
  autocomplete, and the status line. Hence the wonton `Viewport` view in Appendix A,
  with the app-side fallback described there if it is rejected.

## What the change buys

- **Resize works correctly.** Resizing an inline app leaves mangled scrollback today; a
  managed screen re-wraps the entire transcript. (The app has to start handling
  `ResizeEvent` to get there.)
- **One render path.** The static/live duplication in `render.go` goes away.
- **Streaming assistant text becomes visible**, with real markdown, instead of a
  spinner followed by the finished message.
- **Scroll, jump-to-top, collapse/expand tool output, in-app search** — all impossible
  today, because once a line is printed it is gone.
- **Dialogs stay where they are.** The permission / select / input dialogs already
  render in the live region in place of the input (`LiveView`, `app.go:413`); in a
  managed screen they occupy the same footer slot, unchanged. Centered `ZStack`
  overlays become _possible_ (for a help popup, say) but are not part of the migration.
- **`/clear` becomes trivial** — `a.messages = nil` plus a cache reset, replacing
  `ClearScrollback` and ~40 lines of state reset at `app.go:2251`.
- **Resumed session history becomes live.** `printSessionHistoryToScrollback`
  (`app.go:2070`) is a one-shot dump today; it becomes scrollable and re-rendered on
  resize.
- **A whole class of bug disappears.** The `Print()`-height-mismatch ghost-line problem
  documented in wonton's README is worked around explicitly in `handleProcessingEnd`
  (`app.go:1772`); in a managed screen it cannot occur.

## What the change costs

- **Terminal-native scrollback and Cmd+F are gone.** This is the real cost, and the one
  that determines whether the change is a net win. Anything a user wants to find must
  be findable in-app.
- **Mouse selection moves in-app.** Wheel scrolling requires mouse reporting, and with
  reporting on the terminal hands drags to the app instead of selecting. Claude Code's
  fullscreen renderer made the same trade, and the design copies its answer: an
  app-managed selection that behaves like a terminal's — drag highlights, release
  copies, double-click selects a word, the selection can be longer than the screen —
  plus the terminal's own modifier bypass and a `/mouse` toggle
  ([Appendix G](#g-copy-paste-and-mouse-interaction)). What it cannot reproduce is a
  terminal's knowledge of its own wrapping: copied text is what was on screen, gutters
  and wrapped lines included. Turning mouse reporting off is not a free alternative:
  most terminals then translate wheel motion into Up/Down arrow keys on the alternate
  screen, which the input consumes as history recall; the same failure is filed
  against Claude Code's renderer. Paste is unaffected: it arrives as bracketed-paste
  bytes, not mouse events.
- **The transcript vanishes on exit**, since the alternate screen restores what was
  there before. Mitigation: after `terminal.Close()`, `tui.Print` the transcript (or
  at minimum a `session saved — resume with dive --resume <id>` line) into real
  scrollback ([Appendix H](#h-exit-time-transcript-dump)).
- **The shell context disappears while the app runs.** Users who glance back at the
  command they ran or its output lose that until they exit.
- **tmux/screen copy-mode integration shrinks** to whatever is on screen.
- **Screen-reader and IME friendliness drops.** Scrollback text is more accessible than
  a repainting viewport. `InputField` draws its own cursor cell in both modes; the
  hardware cursor stays hidden in full-screen (`WithHideCursor` default), which
  matters to IME candidate windows and screen readers that follow it. Verify whether
  the inline runner actually differs here before calling it a regression.
- **Non-TTY invocation needs an explicit guard.** `InlineApp.Run` refuses a non-TTY
  stdin; `Runtime.Run` does not (it silently skips raw mode, for tests). The CLI
  must check stdin _and_ stdout are terminals before touching the alternate screen,
  and point piped users at the existing `--print` mode.

## Migration plan

Steps 0–2 are improvements on their own merits and de-risk everything after them; they
can land before any decision on the managed screen itself.

0. **Cache the git branch.** Refresh on a timer (every few seconds on tick) or at turn
   boundaries; never from `View()`. Removes ~5 ms from every frame in both modes.
1. **Collapse the duplicate render paths** into a single `messageView(msg, opts)`.
   `opts` carries what differs today: whether the tool-call marker animates, whether
   the result is expanded, and (for the exit dump) that animations must be static.
2. **Make `a.messages` the only output channel.** Replace the 36 `runner.Print*` sites
   with appends — `appendSystem`, `appendNotice`, and a `report` message that carries a
   pre-built view for `/usage`, `/help`, `/todos`, `/context`
   ([Appendix B](#b-transcript-model-and-render-cache)). In inline mode this step is
   invisible: the new messages are printed to scrollback exactly as before.
3. **Land the wonton additions** ([Appendix A](#a-proposed-wonton-additions)):
   `Measure`, `Viewport` + `ViewportState` with selection, `EnableMouseDrag` and click
   counting, cell read-back on `RenderFrame`, synchronized output in the full-screen
   flush, OSC 52 in `clipboard`, and `Runtime.Suspend`. Tag a release and bump `dive`.
4. **Add `View()` alongside `LiveView()`**, wired to a manually constructed `Terminal`
   - `Runtime` ([Appendix E](#e-runtime-construction-and-lifecycle)), behind
     `--screen` / `DIVE_SCREEN=1`. Introduce the small runner interface so tests and
     both runners share it. Handle `ResizeEvent`.
5. **Scrolling** — wheel, PgUp/PgDn, top/bottom, Esc-to-bottom, snap-to-bottom on
   input, the "scrolled up" indicator ([Appendix F](#f-key-and-mouse-bindings)).
6. **Selection and copy** — drag-to-select with copy on release, double/triple click,
   auto-scroll at the edges, the `Copied N lines` notice, `/copy` (selection, picker,
   `all`), `/mouse`, and the input's paste placeholder
   ([Appendix G](#g-copy-paste-and-mouse-interaction)).
7. **Exit-time transcript dump**, `/scrollback` on the same renderer, TTY guards,
   Ctrl+O expand, click-to-toggle tool output.
8. **Flip the default**, keeping `--inline` as an escape hatch for one release, then
   delete the inline path and the `messageViewStatic` family.

### Test impact

- `renderLiveView` (`app_interactive_test.go:209`) becomes `renderScreen(app, w, h)`
  over `View()`. It already goes through `tui.Fprint` + `termtest.Screen`; the only
  change is that `WithHeight(h)` becomes mandatory, since the transcript region is
  flex-sized.
- `app.runner` is a concrete `*tui.InlineApp` (`app.go:257`) that tests assign directly
  (`app_interactive_test.go:231`). It should become a two-method interface —
  `SendEvent`, `Stop` — that `*tui.InlineApp`, `*tui.Runtime`, and a recording test
  fake all satisfy.
- Scroll math becomes unit-testable as pure functions over a slice of heights,
  independent of any terminal ([Appendix J](#j-test-plan)).
- Runtime-level tests become possible with `tui.NewTestTerminal` and a scripted input
  source, parsed back through `termtest`.
- `session_picker.go:79` is a second, independent `InlineApp` that runs before the main
  app. It stays inline: its output lands in the main screen buffer before the
  alternate screen is entered, which is harmless.

## Decisions and open questions

1. **Default or opt-in?** Recommendation: opt-in behind `--screen` / `DIVE_SCREEN=1`
   for one release, dogfood on the terminal matrix in
   [Appendix K](#k-rollout-checklist), then flip with `--inline` as the escape hatch.
2. **Where does the virtualization live?** Recommendation: in wonton. `Measure` is
   required regardless (the footer's height cannot be computed any other way), and
   `Viewport` is the only way to get an exact viewport without guessing. The
   app-private fallback in Appendix A is documented in case the wonton API surface is
   unwelcome. Open: the exact API shape.
3. **Are in-app search and copy in scope for the first version?** Recommendation:
   drag-to-copy, `/copy`, and `/mouse` yes — without them the change is a regression
   against the terminal for everyone who copies from the transcript, which is
   everyone. Gemini CLI shipped an alternate-screen mode and then reverted its
   default until copying works on every terminal without a mode toggle, which is the
   clearest evidence that copy decides whether a managed screen is accepted. Search
   no: Claude Code's answer to `Cmd+F` is to hand the transcript back
   to the terminal on demand, and a `/scrollback` command that does the same
   ([G9](#g-copy-paste-and-mouse-interaction)) covers the need with a fraction of the
   code, leaving in-app search as a follow-up. The exit-time transcript dump stays
   mandatory, and OSC 52 moves onto the critical path for SSH users.
4. **Exit dump policy.** Recommendation: the full transcript, rendered statically at the
   current width and capped at the last 2,000 lines, followed by the resume line.
   Open: whether to expose `--exit-transcript=full|turn|none`.
5. **Streaming markdown.** Recommendation: render the streaming message as markdown on
   every flush and measure; add the prefix/tail split in Appendix B only if it shows
   up in profiles. Open: whether transient flicker from unclosed fences is acceptable.
6. **Copy on release, or highlight only?** Recommendation: copy on release, which is
   Claude Code's and OpenCode's default and what tmux does with the mouse on. The
   terminals themselves are split — copy-on-select is on by default in iTerm2,
   WezTerm, and Ghostty, off in kitty, Alacritty, Windows Terminal, VS Code, and
   GNOME Terminal — so about half of users will find a clipboard overwrite on an
   accidental drag unfamiliar. Ship it on by default behind a `copy_on_select`
   setting, as Claude Code does with its `Copy on select` toggle; when off, a drag
   only highlights, `/copy` with no argument sends the highlight, and Ctrl+C with a
   selection copies instead of cancelling ([G5](#g-copy-paste-and-mouse-interaction)).

---

## Appendix: detailed recommendations

### A. Proposed wonton additions

These are ordered by how hard the migration is without them. All are small, and all
have use beyond `dive`.

#### A1. `tui.Measure` — required

```go
// Measure returns the size a view would occupy when laid out within maxWidth
// (0 = unconstrained). It runs only the measure phase; nothing is drawn.
func Measure(v View, maxWidth int) (width, height int) {
	return v.size(maxWidth, 0)
}
```

One line. Needed for the footer (whose height depends on wrapped input, attachments,
autocomplete and status rows) and for any app-side item height cache. Side effects
are the same as a normal measure pass (`StackView` records child sizes), which is
harmless.

#### A2. `tui.Viewport` + `ViewportState` — strongly recommended

A bottom-anchored, virtualized list whose scroll state lives in an app-owned struct,
following the `Scroll(content, &scrollY)` convention of passing state by pointer.

```go
// ViewportItems is the application's list. Item(i) is called at most once per
// index until Invalidate(i) or a width change; the returned view is retained in
// the state so per-instance caches (MarkdownView) keep hitting.
type ViewportItems interface {
	Len() int
	Item(i int) View
}

// ViewportState is owned by the application and passed to Viewport every frame.
type ViewportState struct {
	// Follow pins the viewport to the end of the content. Set by ScrollToBottom
	// and by the view when a downward scroll reaches the end; cleared by any
	// upward scroll.
	Follow bool

	// Written back by the view on every render; read them in HandleEvent.
	Width, Height int  // viewport size at the last render
	AtBottom      bool // no content below the viewport
	LinesBelow    int  // content lines below the viewport

	// unexported: anchor {index, line}, per-item view+height cache, gap,
	// the items reference and width seen at the last render.
}

func (s *ViewportState) Invalidate(i int) // item i changed
func (s *ViewportState) InvalidateAll()   // everything changed (e.g. /clear)
func (s *ViewportState) ScrollBy(lines int) // +down, −up; clamps; clears Follow on up
func (s *ViewportState) PageUp()          // Height−1 lines
func (s *ViewportState) PageDown()
func (s *ViewportState) ScrollToTop()
func (s *ViewportState) ScrollToBottom()  // Follow = true
func (s *ViewportState) ScrollToItem(i int)

func Viewport(state *ViewportState, items ViewportItems) *ViewportView
func (v *ViewportView) Gap(rows int) *ViewportView // blank rows between items

// Selection (Appendix G). Endpoints are (item, line, col), so the highlight
// follows its text through scrolling, streaming, and resizes.
func (s *ViewportState) HandleMouse(ev MouseEvent) MouseResult
func (s *ViewportState) HasSelection() bool
func (s *ViewportState) SelectedText() string // cell text, rows joined by \n
func (s *ViewportState) ClearSelection()
func (v *ViewportView) SelectionStyle(st Style) *ViewportView // default: reverse video

type MouseResult struct {
	Kind       MouseResultKind // Ignored, Consumed, SelectionDone, Click
	Item, Line int             // set for Click: what was under the pointer
}
```

Semantics:

- `size()` returns the constraints it is given, and `flex()` returns 1, so the view
  fills whatever a `Stack` leaves after the fixed children — the footer keeps its
  natural height with no arithmetic.
- `render(ctx)` reads the exact width and height from `ctx.Size()`. If the width
  differs from the last render, it drops all cached heights and views. It then lays
  out from the anchor (or, when `Follow` is set, from the end backwards until the
  viewport is full), measuring only the items it touches, caching each `(view,
height)` it computes, and drawing the first item through an offset frame — the
  same `scrollRenderFrame` that `ScrollView` uses — to cut it at the anchor line.
- The scrolling methods walk the height cache across item boundaries, measuring on
  demand using the items reference and width from the last render. Before the first
  render they are no-ops.
- `Invalidate(i)` drops the cache entry for `i`; the anchor is unaffected unless it
  points into `i`, in which case the line is clamped on the next render.
- Items may be appended freely; `Len()` is re-read each frame.

This is roughly 200 lines plus tests, and the scroll math is pure and table-testable;
selection adds about 150 more (G1–G3). `dive` calls `Invalidate` at its ten mutation
sites, forwards mouse events to `HandleMouse`, and otherwise just appends.

#### A3. Drag-capable mouse mode and click counting — required for selection

```go
// EnableMouseDrag enables SGR mouse reporting for presses, releases, wheel, and
// motion while a button is held (?1006 + ?1000 + ?1002). Hover is not reported.
func (t *Terminal) EnableMouseDrag()

func WithMouseButtons(enabled bool) RunOption // ?1000: clicks and wheel
func WithMouseDrag(enabled bool) RunOption    // ?1002: clicks, wheel, and drags
```

`DisableMouseTracking` (`terminal.go:1217`) should also emit `?1002l`. The decoder
needs nothing: it already classifies motion with a button held as `MouseDrag`. The
runtime's click synthesis (`runtime.go:622`) should count clicks — same cell, within
500 ms, as `MouseHandler` does at `tui/mouse.go:105` — and set `ClickCount` on the
`MouseClick` it emits, so views can offer word and line selection without each
re-implementing the timer. `dive` constructs the runtime manually and only needs the
`Terminal` method, but the options are what every other full-screen wonton app wants
and cost a few lines each. Claude Code's renderer highlights hovered rows and so
accepts any-motion tracking; nothing in this design needs hover.

#### A4. Synchronized output in the full-screen flush — recommended

Wrap the byte stream `Terminal.flushInternal` emits in `\x1b[?2026h` … `\x1b[?2026l`,
as `LivePrinter.update` already does (`print.go:510`). Terminals that do not support
DEC 2026 ignore it. This removes tearing on page scrolls, where every cell in the
viewport changes in one frame.

#### A5. OSC 52 in `clipboard` — on the critical path for SSH

```go
// WriteOSC52 asks the terminal attached to w to place text on the system
// clipboard. Works over SSH; the payload is base64 and some terminals cap it.
func WriteOSC52(w io.Writer, text string) error
```

Sequence: `\x1b]52;c;<base64>\x07`, written to the same descriptor the runtime
renders on. Do not wrap it in a tmux DCS passthrough: tmux's default
`set-clipboard external` blocks passthrough from applications, and when it is `on`
the passthrough races tmux's own OSC 52 writes (OpenCode collected a set of issues
this way). Under tmux, hand the text to tmux instead
([G4](#g-copy-paste-and-mouse-interaction)). Where the sequence lands is uneven:
write is on by default in kitty, WezTerm, Ghostty, Alacritty, Windows Terminal, and
VS Code locally; off by default in iTerm2 ("Applications in terminal may access
clipboard") and xterm (`allowWindowOps`); dropped by VS Code over Remote SSH; and
unsupported in Terminal.app and GNOME Terminal (VTE). xterm and hterm cap a
sequence at 100,000 bytes and kitty truncates long single sequences, so truncate
with a warning rather than silently dropping. Writing it from
`HandleEvent` is safe: that runs on the render goroutine, between frames, and the
sequence changes no cells.

`clipboard.Write` could try the native tool first and fall back to OSC 52 when
`Available()` is false or `SSH_TTY` is set, but an explicit function is enough for
`dive`; the transport choice is spelled out in
[G4](#g-copy-paste-and-mouse-interaction).

#### A6. Optional: hand the runtime to the app

`tui.Run` could call an optional interface before starting the loop:

```go
type RuntimeAware interface{ SetRuntime(*Runtime) }
```

This would let `dive` use `tui.Run` and its options instead of 25 lines of manual
setup. Not necessary — the manual path is needed anyway for `EnableMetrics` and for
the exit-time `Close()`-then-`Print` ordering — so this is a nicety.

#### A7. Cell read-back on the render frame — required for selection

```go
// Cell returns the cell most recently written at (x, y) in frame coordinates;
// the zero Cell outside the frame.
Cell(x, y int) Cell            // added to terminal.RenderFrame
func (c *RenderContext) Cell(x, y int) Cell
```

`Terminal.GetCell` (`terminal.go:2236`) already reads the back buffer under the lock;
this exposes it through the frame and sub-frames so a view can paint a highlight over
cells its children drew — re-`SetCell` with the same character and a reversed style —
and read the text out for the clipboard. About 30 lines. It also makes an offscreen
render readable, which `SelectedText()` needs for rows that have scrolled out of view
(G3).

#### A8. `Runtime.Suspend` — recommended, for `/scrollback`

```go
// Suspend stops rendering and input, leaves the alternate screen with the
// mouse released and the cursor shown, runs fn, then re-enters and repaints.
func (r *Runtime) Suspend(fn func()) error
```

The terminal pieces exist (`DisableAlternateScreen`, `DisableMouseTracking`, and
their inverses), but the runtime's input goroutine owns stdin, so this has to be a
runtime method rather than app code. It is what `/scrollback`
([G9](#g-copy-paste-and-mouse-interaction)) and a future `$EDITOR` hand-off need, and
it is how editors implement `:!`.

#### Fallback if the wonton changes are rejected

- **Measure** via `tui.Sprint(view, tui.WithWidth(w))` and counting lines. One
  full render per measurement, acceptable because results are cached per message.
  Measuring the _footer_ this way is the weak spot: it renders an `InputField`
  outside the runtime's render context, which is untested territory for focus and
  cursor state.
- **Exact viewport height** as `termHeight − footerHeight`, with `termHeight` tracked
  from `ResizeEvent` (the runtime sends one before the first render) and the
  transcript wrapped in `tui.Height(viewportH, ...)`. Any off-by-one here shows up as
  a cut-off or blank bottom row.
- **Sub-message offset** via `tui.Scroll(tui.Stack(visible...), &offset)`: build
  exactly the items covering the viewport, and let `ScrollView` clamp `offset`.
  Because it only ever sees a handful of cached items, its measure-everything habit
  no longer matters.
- **Follow-bottom** by walking heights backwards from the last item, as in the
  wonton version, but in app code.
- **Selection** in screen coordinates only: extract text on release with
  `Terminal.GetCell` (the app owns the `*Terminal`), and either skip the highlight or
  fake it with a `ZStack` overlay rebuilt each frame from the previous frame's cells,
  one frame behind. No auto-scroll, and the highlight must be cleared on any scroll or
  streamed append. Enough for a prototype; not something to ship.

A `CanvasContext` that blits pre-rendered cells (from `tui.SprintScreen`) would give
an exact viewport with no wonton change at all, but it loses animations and
hyperlinks in the transcript and needs a `termtest.Style` → `tui.Style` conversion.
Not recommended.

### B. Transcript model and render cache

**Messages are append-mostly.** The ten sites that mutate an existing message in
place — `flushStreamBuffer`, `flushThinkingStreamBuffer`, `handleToolResult`,
`handleToolStream`, `handleToolProgress`, and the todo/expand paths — each get an
`a.touch(i)` call that forwards to `viewport.Invalidate(i)`. A `Rev uint32` field on
`Message`, bumped by `touch`, is a cheap belt-and-braces for tests and for any
app-side cache.

**One `messageView(msg Message, opts viewOpts) tui.View`**, where `opts` is:

```go
type viewOpts struct {
	animate  bool // pulse the marker of a running tool call (false in the exit dump)
	expanded bool // show full tool output instead of first line + "… +N lines"
}
```

The static variants disappear. The blank line between messages moves from an explicit
`tui.Text("")` item into `Viewport(...).Gap(1)`, so it is not part of any message's
cached height.

**New message kinds replace the `Print` sites.** Current roles are `user`,
`assistant`, `reasoning`, `system`, `context`, `intro`, plus tool-call messages.
Add:

| Kind                                  | Replaces                                                                               | Rendering                                                                              |
| ------------------------------------- | -------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `notice`                              | `Printf` warnings, mid-turn compaction line, context-demo `◇` rows, "Did not attach …" | dim single line                                                                        |
| `report` with a `View tui.View` field | `/usage`, `/help`, `/todos`, `/context`                                                | the stored view, re-wrapped by its own `Text().Wrap()` / `Markdown` children on resize |
| `intro` (exists)                      | `printIntroToScrollback`, `printIntroViaRunner`                                        | unchanged; `/clear` appends a fresh one                                                |

Storing a built view inside a `report` message is fine: wonton views are inert until
measured, and `Wrap`/`Markdown` children re-lay out at whatever width the viewport
gives them.

**Streaming message.** On every flush (at most once per tick) the streaming
message's content grows and its cache entry is invalidated, so it is re-parsed as
markdown once per frame while streaming — on the order of 0.2–0.5 ms for a few KB.
That is within budget. Two refinements, in order of need:

1. Only flush when the buffer is non-empty (already the case), so idle frames do not
   re-parse.
2. If profiles show the parse, split the content at the last blank line: everything
   before it is a separate cached `Markdown` view (only rebuilt when the split point
   moves); the tail is plain wrapped text. Move the split point back to before any
   unclosed code fence so fences never straddle the boundary. This also damps the
   visual flicker of a fence that has not closed yet.

**Large tool outputs.** A collapsed tool result is one line plus a count. An expanded
5,000-line result becomes one tall `Text` view: measuring it wraps 5,000 lines (a few
ms, once), and rendering it clips row by row through the offset frame. Fine, and the
user asked for it.

**Width.** The viewport's content width is the terminal width minus the transcript's
horizontal padding. The cache keys on the width the viewport actually rendered at, so
the app never needs to compute it.

### C. Scroll model

State: `anchor = (index, line)` — the item and the line within it at the top of the
viewport — and `follow bool`. All algorithms operate on `h(i)`, the cached height of
item `i` at the current width, plus `gap`.

**Layout from the anchor.**

```
y := -anchor.line
for i := anchor.index; i < n && y < H; i++ {
    draw item i at rows [y, y+h(i))   // clipped to [0, H)
    y += h(i) + gap
}
```

**Follow (pin to bottom).** Walk backwards until the viewport is full:

```
rows := 0                    // content rows accounted for so far
i := n
for i > 0 && rows < H {
    i--
    if i < n-1 { rows += gap }
    rows += h(i)
}
if rows <= H { anchor = (0, 0) }        // everything fits: top-align
else         { anchor = (i, rows - H) } // first visible item, and how many of its rows are hidden
```

The anchor line may land inside the gap below an item (when the item is entirely
hidden but its gap is not); the layout loop above handles that without a special
case, and `ScrollBy` normalizes it on the next move.

A transcript shorter than the viewport renders from the top, the way a fresh chat
does; it starts scrolling only once it overflows.

**ScrollBy(−k) (up).** Subtract from `anchor.line`; while it goes negative and
`anchor.index > 0`, step to the previous item and add `h(prev) + gap`. Clamp at
`(0, 0)`. Any upward scroll sets `follow = false`.

**ScrollBy(+k) (down).** Add to `anchor.line`; while it exceeds `h(index) + gap - 1`
and there is a next item, step forward. Then clamp so the last item's bottom does not
rise above the viewport bottom; if the clamp engaged, set `follow = true`.

**Resize.** Width change drops all heights. The anchor index is kept; the line is
clamped to the new `h(index) − 1` on the next render. `follow` is unchanged. This is
what makes a resize feel stable: the user keeps looking at the same message.

**Content arriving while scrolled up** does not move the anchor. The footer shows
`↓ N new lines · End to jump` using `LinesBelow`; when the user returns to the bottom
it clears.

**Snap-to-bottom** on: submit, any character typed into the input, `/clear`, and a
new turn starting. This mirrors what a terminal does with scrollback when you type.

**Not in v1.** A scrollbar or "34%" indicator needs total content height, which
needs every item measured. Skip it; if wanted later, measure lazily in idle frames
and show the indicator only once complete.

### D. Screen layout

```go
func (a *App) View() tui.View {
	return tui.Stack(
		tui.PaddingLTRB(1, 0, 1, 0, tui.Viewport(&a.viewport, a).Gap(1)), // flex
		a.footerView(),                                                   // natural height
	)
}
```

`footerView`, top to bottom, is what `LiveView` builds today minus the transcript-ish
parts, so most of `app.go:409–530` moves unchanged:

1. **Scroll indicator** (only when `!follow`): `↓ N new lines · End to jump`.
2. **Activity row** while processing: the spinner / `thinking (12s) · esc to interrupt`
   line from `buildLiveView`, and the transient compaction notice. Tool-call and
   reasoning rows no longer appear here — they are in the transcript, animating in
   place while the tool runs.
3. **Todo list** when toggled.
4. Blank line, divider, attachments, the `InputField`, divider.
5. **Status line** or the autocomplete list, then the two footer rows (exit hint,
   compaction stats).

**Dialogs.** When `dialogState.Active`, the footer is `dialogView()` and nothing else —
exactly the branch at the top of `LiveView` today. Focus IDs (`confirm-dialog`,
`select-list`, `multiselect-list`, `dialog-input`) and the `showDialogEvent` /
`hideDialogEvent` flow are untouched. The transcript above keeps rendering, so the
tool call being approved stays visible, which is the point of putting the prompt at the
bottom rather than in a centered modal. If a centered overlay is ever wanted, note that
`ZStack` does not clear behind its children: wrap the overlay in `Panel(...).Bg(...)`.

**Footer height changes** (input growing to 10 lines, autocomplete's 8 rows) shrink the
viewport. With `follow` set the bottom stays pinned and the transcript slides up, which
is the expected chat behaviour; with `follow` clear the anchor holds and the bottom of
the viewport is simply covered.

### E. Runtime construction and lifecycle

```go
type uiRunner interface {
	SendEvent(tui.Event)
	Stop()
}

func (a *App) runScreen() (err error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("interactive mode needs a terminal; use --print for piped use")
	}
	terminal, err := tui.NewTerminal()
	if err != nil {
		return err
	}
	defer terminal.Close() // idempotent; also runs if Run re-panics

	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	terminal.EnableMouseDrag() // wheel, clicks, drags; no hover flood (A3)
	terminal.EnableBracketedPaste()
	if os.Getenv("DIVE_DEBUG_FRAMES") != "" {
		terminal.EnableMetrics()
	}

	rt := tui.NewRuntime(terminal, a, 30)
	a.runner = rt
	a.terminal = terminal // for /mouse and metrics

	err = rt.Run()   // probes Kitty, enables raw mode, blocks; restores both on return
	terminal.Close() // mouse, paste, cursor, alternate screen, style reset
	a.printExitTranscript()
	return err
}
```

Points that are easy to get wrong:

- **Order at exit.** `Runtime.Run` restores raw mode and the Kitty protocol itself;
  `terminal.Close()` does the rest, including leaving the alternate screen. The
  transcript dump must come _after_ `Close()`, or it lands in the buffer that is about
  to be discarded. The deferred `Close()` covers the panic path: `Run` re-panics after
  restoring raw mode, unwinding runs `Close()`, and the stack trace prints on the main
  screen where it can be read.
- **Kitty under tmux.** The probe is skipped there, so Shift+Enter will not insert a
  newline unless `rt.SetBackslashEnter(true)` is set — which delays every typed
  backslash by up to 100 ms. Today's inline app sends the Kitty enable sequence
  unconditionally instead. Decide which to keep after testing in tmux; the default
  should match current behaviour.
- **Resize.** Handle `tui.ResizeEvent` in `HandleEvent` (there is no handler today).
  The viewport re-measures on its own from the width it is rendered at; the app only
  needs the size if it wants it for the status line.
- **`/clear`** becomes: reset the session (unchanged), `a.messages = a.messages[:0]`,
  `a.viewport.InvalidateAll()`, `a.viewport.ScrollToBottom()`, append a fresh intro.
- **Initial prompt.** The 50 ms `time.Sleep` before `SendEvent(initialPromptEvent)`
  (`app.go:2018`) is a workaround for the inline runner's startup; with `Runtime` the
  events channel exists before `Run`, so the event can be sent immediately and is
  processed after `Init`.
- **The session picker** stays an `InlineApp`; it finishes before the alternate screen
  is entered.

### F. Key and mouse bindings

The focused `InputField` sees keys first and consumes: arrows, Home/Ctrl+A,
End/Ctrl+E, Enter, Backspace, Delete, Ctrl+W, Ctrl+U, Ctrl+K (`tui/text_input.go`).
Everything else falls through to `HandleEvent`. Keys the app wants _despite_ focus go
through the input's `OnKey` hook, which `handleInputNavKey` already uses.

| Input                                             | Action                                                          | Notes                                                                                                                                                                                    |
| ------------------------------------------------- | --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Wheel up / down                                   | Scroll 3 lines                                                  | Any button-reporting mode (A3); `MouseScroll` events                                                                                                                                     |
| PgUp / PgDn                                       | Half a screen                                                   | Not consumed by the input; Claude Code's fullscreen default                                                                                                                              |
| Ctrl+Home / Ctrl+End                              | Top / bottom                                                    | The decoder applies xterm modifiers to Home/End (`decoder.go:285`); Kitty not required. Mac keyboards cannot send Ctrl+End (Claude Code documents the same gap), so the next row matters |
| Home / End with empty input                       | Top / bottom                                                    | Via `OnKey`, returning `true` only when the input is empty                                                                                                                               |
| Esc, idle and scrolled up                         | Back to bottom                                                  | Esc while processing still cancels; with autocomplete open still clears it                                                                                                               |
| Ctrl+O                                            | Toggle expanded tool output                                     | Not consumed by the input; Claude Code's classic renderer uses it this way, its fullscreen one for transcript mode (G9)                                                                  |
| Typing, Enter, paste                              | Snap to bottom                                                  | `follow = true`                                                                                                                                                                          |
| Ctrl+C ×2                                         | Exit (then dump)                                                | Unchanged                                                                                                                                                                                |
| Left drag over the transcript                     | Select; copy on release                                         | Content-anchored; auto-scrolls at the edges; needs `?1002` (A3)                                                                                                                          |
| Click                                             | Clear the selection; on a tool call's header, toggle its output | Runtime-synthesized `MouseClick`; see G8                                                                                                                                                 |
| Double-click / triple-click                       | Select the word / the line, and copy                            | Needs `ClickCount` from the runtime (A3)                                                                                                                                                 |
| Shift-drag (Option in iTerm2, Fn in Terminal.app) | Terminal-native selection                                       | Never reaches the app; the terminal keeps it                                                                                                                                             |
| Typing, Enter, plain arrows                       | Clear the selection                                             | Esc, PgUp/PgDn, Ctrl+Home/End, and the wheel keep it (Claude Code's rules)                                                                                                               |
| Middle / right click                              | Ignored                                                         | Terminals differ on whether they even report them                                                                                                                                        |

Avoid: Ctrl+U / Ctrl+D for half-page (Ctrl+U is kill-to-start in the input, Ctrl+D
reads as EOF), Shift+PgUp/PgDn (terminals intercept them for native scrollback),
Alt/Option+arrows (Option-as-Meta is off by default in macOS terminals), and plain
letters (they are text).

### G. Copy, paste, and mouse interaction

The target is Claude Code's fullscreen renderer, which is where the gesture in the
brief comes from: drag over the transcript and the text is on the clipboard when the
button comes up. That renderer captures the mouse, keeps the selection in-app, copies
on release, and still lets the terminal's own selection through under a modifier
([Fullscreen rendering](https://code.claude.com/docs/en/fullscreen), "Use the mouse"
and "Keep native text selection"). Its classic renderer never enabled the mouse, so a
drag there was the terminal's selection and the copy was the terminal's
copy-on-select setting — on by default in iTerm2, off in Terminal.app. This section
adopts the fullscreen behaviour, escape hatches included, and notes where it diverges.

#### G1. Selection model

The selection lives in `ViewportState` and is anchored to content, not to the screen:

```go
type SelectionPoint struct {
	Item int // index into ViewportItems
	Line int // row within the item's rendered rows; height(item) means the gap below it
	Col  int // column within the viewport width
}

// Anchor is where the press happened; Focus follows the pointer. Either may
// come first in reading order.
type selection struct{ anchor, focus SelectionPoint }
```

Anchoring to content is what makes the rest simple:

- **It survives scrolling.** Auto-scroll during a drag, a wheel after release, or a
  streamed append below the selection all leave the highlight on the same text. A
  screen-coordinate selection would have to be cleared the moment anything moved.
- **It can be longer than the screen.** Dragging past the top or bottom edge scrolls
  the viewport and the focus moves onto the newly revealed rows — the one thing a
  terminal's selection does that a naive TUI selection cannot.
- **It reuses the layout the viewport already computes.** Layout from the anchor
  ([C](#c-scroll-model)) produces, for each screen row, the `(item, line)` it shows;
  the reverse map is a scan of that list. Gap rows map to `(item, height(item))`, one
  past the item's last line, so they behave as blank lines.

Columns follow a terminal's stream selection: the first row runs from the start
column to the row's end, middle rows are whole, the last row runs from column zero to
the end column. An endpoint that lands on the continuation cell of a wide character
snaps to the character's first cell. Rectangular selection is a non-goal.

#### G2. Mouse state machine

`ViewportState.HandleMouse(ev MouseEvent) MouseResult` receives every mouse event the
app gets and tests it against the absolute bounds recorded at the last render
(`ctx.AbsoluteBounds()`, the pattern `FilterableListView` already uses at
`filterable_list_view.go:487`). Events outside the bounds are ignored except during a
drag.

| Event                              | Effect                                                                                                                                                                                                                              |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MousePress`, left, inside         | Clear the old selection; record the anchor; remember `Follow` and clear it so the view holds still                                                                                                                                  |
| `MouseDrag`, left                  | Move the focus to the pointer's cell; above or below the bounds, clamp to the edge row and start auto-scroll                                                                                                                        |
| `MouseRelease`, left               | End the drag; restore `Follow` if it was set and the viewport is still at the bottom; return `SelectionDone` if the selection is non-empty                                                                                          |
| `MouseClick` (runtime-synthesized) | A press and release with no movement: clear the selection and return `Click{Item, Line}` for [G8](#g8-clicks-that-do-something). With `ClickCount` 2 or 3, select the word or the line under the pointer and return `SelectionDone` |
| `MouseScroll`                      | Scroll three lines as today; during a drag also move the focus, since different text is now under the pointer                                                                                                                       |
| Middle, right, any modifier        | Ignored. Shift- and Option-drags never reach the app; the terminal keeps them                                                                                                                                                       |

A drag never becomes a click: the runtime synthesizes `MouseClick` only when the
press and release land on the same cell (`runtime.go:622`), so a drag that starts on
a dialog option cannot activate it — a bug OpenCode shipped.

Auto-scroll runs in the viewport's render: one line per frame while the pointer is
held outside the bounds, three per frame beyond two rows out, the way terminals scroll
a selection at the edge. Mutating scroll state during render is how `ScrollView`
already writes its clamped offset back, so it is not a new precedent.

`Follow` is suspended for the duration of the drag so a streaming append cannot move
the text under the pointer. Layout from the anchor keeps every row above the
viewport's bottom stable, so a streamed append during a drag only ever adds rows below
the selection. (Codex's transcript overlay applies the same rule on every append and
resize: re-pin to the tail only if the view was already there, `pager_overlay.rs:602`.)

Clearing follows Claude Code's rules: the next press, a typed character, Enter, or a
plain arrow key clears the selection; Esc performs its usual action (interrupt,
dismiss) and leaves the highlight alone, as do PgUp/PgDn, Ctrl+Home/End, and wheel
scrolling. `Invalidate` and `InvalidateAll` clear it when they remove the items it
points into. Nothing else does, and nothing needs to, because the copy already
happened on release.

Click counting belongs in the runtime: `processMouseEvent` (`runtime.go:622`) already
tracks the press location for click synthesis, and extending it to count clicks
within 500 ms at the same cell fills the `ClickCount` field that `MouseEvent` declares
and that only the standalone `MouseHandler` (`tui/mouse.go:105`) sets today
([A3](#a-proposed-wonton-additions)).

#### G3. What gets copied

Text is extracted from rendered cells, exactly as a terminal does it — no attempt to
reverse markdown rendering. For each selected row: concatenate `Cell.Char` plus
`Cell.Trailing` for every non-continuation cell in the column range, trim trailing
whitespace, join rows with `\n`, no trailing newline. Gap rows become empty lines.

The visible rows are read back during render through the frame read path
([A7](#a-proposed-wonton-additions)), so `SelectedText()` is exact for what is on
screen. Rows that scrolled out of view since the drag began are recovered on release
by rendering the affected items into an offscreen frame at the viewport width — the
same cached views, so one render per item in the range (about 0.25 ms each at the
sizes in the measurements), paid once per copy rather than once per frame.

Consequences, every one shared with copying Claude Code's output from a terminal:

- Soft-wrapped lines copy as separate lines. Claude Code's classic renderer has the
  same behaviour because Ink wraps text itself before the terminal sees it.
- Gutters, tool-call borders, and the `●` markers copy when the selection covers
  them. The message prefix column at the left is the most common nuisance.
- Rendered markdown copies as rendered: bullet glyphs, no `**`, no syntax colours.

For the cases where that matters — a code block someone wants to run, a whole answer
to paste elsewhere — `/copy` ([G6](#g6-keyboard-copy-copy)) provides the source text.

#### G4. Writing the clipboard

The ladder below is what Claude Code documents for its own copy, adjusted for what
the terminal research turned up. The first rung that applies wins, except that tmux's
buffer is always set when running inside tmux.

1. **Native tool** via `clipboard.Write` when `clipboard.Available()`
   (`clipboard.go:186`) and `SSH_TTY` is unset. It shells out (`pbcopy`, `wl-copy`,
   `xclip`, `clip.exe`) with a 5 s timeout, so it must not run on the render
   goroutine: return a `tui.Cmd` from `HandleEvent` and deliver the outcome as an
   event. On X11 and Wayland also write the PRIMARY selection (`xclip -selection
primary`, `wl-copy --primary`) so middle-click paste works, as Claude Code does;
   `clipboard` has no option for that today. This rung is the only verifiable one —
   the tool's exit status says whether the copy happened.
2. **tmux** when `TMUX` is set: `tmux load-buffer -w -` sets tmux's paste buffer so
   tmux's own paste key sees the selection, and with `-w` asks tmux to forward it to
   the outer terminal through tmux's own OSC 52 write, governed by `set-clipboard`
   (default `external`, which forwards tmux's buffers but blocks applications'
   passthrough). When rung 1 did not apply — SSH into a box running tmux — this
   forward is the system-clipboard path, and it needs no passthrough escape.
3. **OSC 52** ([A5](#a-proposed-wonton-additions)) otherwise: over SSH without tmux,
   in containers, and wherever no tool is installed. It is a write to the terminal's
   output stream, so it _must_ run on the render goroutine between frames — which is
   where `HandleEvent` runs — or it will interleave with a frame flush. The app
   cannot tell whether it worked: iTerm2 and xterm drop it by default, Terminal.app
   and GNOME Terminal do not implement it, and VS Code discards it over Remote SSH.
   `DIVE_CLIPBOARD=osc52` forces this rung for users whose terminal supports it and
   whose native tool is misconfigured, a common state on Linux desktops.

Cap OSC 52 payloads at 100 KB of base64 — the xterm and hterm limits — and say
`copied the first N lines` when truncated. A drag selection is a few screens at most;
only `/copy` of a long message approaches the cap. Codex's clipboard code
(`codex-rs/tui/src/clipboard_copy.rs`) settles on the same ladder — native library,
then tmux, then OSC 52 under SSH, with a 100,000-byte cap — which is some evidence
that this is where the trade-offs land.

#### G5. Feedback and escape hatches

- **Notice.** On success the status line shows `Copied 12 lines (pbcopy)` for two
  seconds. Claude Code's toast names the path too, and the name matters: for OSC 52
  the honest wording is `Sent 12 lines to the terminal clipboard (OSC 52)`, because
  the app cannot confirm delivery — Crush shows a success toast on GNOME Terminal
  while nothing is copied, which is the failure to avoid. On a verifiable failure:
  `Copy failed: <reason> — hold Shift to select with the terminal`, with the key
  chosen as below.
- **Modifier bypass.** Every terminal in the rollout matrix except Terminal.app
  documents a modifier that gives a native selection while an application has the
  mouse: Shift in kitty, WezTerm, Ghostty, Alacritty, Windows Terminal, and GNOME
  Terminal; Option in iTerm2; Alt in VS Code on Windows and Linux, and Option on
  macOS only with `terminal.integrated.macOptionClickForcesSelection`. Claude Code's
  docs say Fn for Terminal.app; Apple's do not say. tmux defers to the outer terminal.
  Pick the key for the hint from `TERM_PROGRAM` (`iTerm.app`, `Apple_Terminal`,
  `vscode`, `WezTerm`, `ghostty`) and list the candidates when it is unset, as under
  SSH or tmux. The user gets the terminal's own selection, with its own soft-wrap
  joining and copy rules, at no cost to the app. Say so in `/help` and the first-run
  box ([K](#k-rollout-checklist) has the matrix).
- **`/mouse`** turns reporting off (`terminal.DisableMouseTracking()`) and back on,
  which is why the app keeps its `*Terminal`; `DIVE_DISABLE_MOUSE=1` starts that way,
  the counterpart of Claude Code's `CLAUDE_CODE_DISABLE_MOUSE` and OpenCode's
  `tui.mouse`. With reporting off, plain drags are native and the wheel stops being
  useful: most terminals translate wheel motion into arrow keys on the alternate
  screen, and the input consumes those as history recall. Emit `CSI ?1007 l`
  alongside the disable to ask terminals that honour it to stop translating (nothing
  in wonton emits it today), and say `PgUp/PgDn scroll while the mouse is off` in the
  notice. Codex relies on the opposite setting — it enables `?1007` for its
  alternate-screen overlays so the wheel arrives as arrow keys they can scroll on
  (`tui.rs:251`) — which works there because those overlays have no text input.
- **`copy_on_select`** setting, default true, the counterpart of Claude Code's
  `Copy on select` toggle. When false, a drag only highlights; `/copy` with no
  argument sends the highlight, Ctrl+C with a selection active copies it instead of
  cancelling, and Ctrl+Shift+C copies on the terminals that pass it through (the
  Kitty-protocol ones). For the half of terminals whose users are not used to
  copy-on-select.

#### G6. Keyboard copy: `/copy`

`/copy` is the semantic complement to drag-select: it copies source text, not cells.
Codex has the better shape here — its `/copy` opens a picker listing the whole
response and each code block and blockquote separately
(`codex-rs/tui/src/chatwidget/interaction.rs:315`) — and `dive` already has the
select-list dialog to build it on.

| Form        | Copies                                                                                                                                                                  |
| ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/copy`     | The current selection if there is one; otherwise opens a picker over the last assistant message: whole response, then each fenced code block by language and first line |
| `/copy N`   | The same picker for the N-th assistant message from the end                                                                                                             |
| `/copy all` | The whole transcript as markdown, capped as in [H](#h-exit-time-transcript-dump)                                                                                        |

No default key binding for copy. Ctrl+Shift+C and Cmd+C are the terminal's own copy
everywhere that matters, and they reach the app only under the Kitty protocol and
only in some terminals. A slash command works everywhere and is discoverable through
autocomplete; Claude Code ships a `/copy` too, and Codex additionally binds Ctrl+O to
"copy the last response as markdown", which is an argument for making that a
rebindable action rather than for taking Ctrl+O away from tool-output expansion.

#### G7. Paste

Paste already works and the managed screen changes nothing about it: bracketed paste
is on, the decoder delivers a paste as one `KeyEvent` with `Paste` set
(`terminal/key.go`), and the focused `InputField` inserts it through `HandlePaste`
(`input_view.go:87`). Three refinements belong in the same change:

- **Collapse large pastes.** `InputField.PastePlaceholder(true)` renders a multi-line
  paste as `[pasted 27 lines]` while `Value()` keeps the full text — Claude Code's
  `[Pasted text #1 +27 lines]`, which it applies above 800 characters or three lines,
  and Codex's `[Pasted Content 1523 chars]` above 1,000 characters
  (`chat_composer.rs:363`), with `#2`, `#3` suffixes only when two pending pastes have
  the same size. Either stops a 500-line stack trace from swallowing the footer.
  Wonton's trigger is any paste containing a newline; a character threshold is a
  one-line addition.
  `dive` does not enable it today (`app.go:443`). Check that Backspace and Ctrl+W
  remove the placeholder as a unit, and that `handleInputChange`'s inserted-range
  diff (`attachments.go:326`) still sees the actual text so dropped-file detection
  keeps working.
- **File drops** arrive as a paste of the path and are handled in
  `handleInputChange`. Mouse reporting does not affect them — a drop is a paste, not a
  mouse event — but verify per terminal on the alternate screen, since a few terminals
  treat drops differently while an app has the mouse. `scanPathTokens`
  (`attachments.go:143`) already covers what Codex's normalizer does — quotes,
  backslash escapes, `file://` URLs, `~` — short of Windows, UNC, and WSL path forms,
  which are out of scope.
- **Paste while scrolled up, or with a dialog open,** goes to the focused input and
  the transcript snaps to the bottom, as for typing. Nothing to add.

Image paste — Ctrl+V in both Claude Code and Codex, because Cmd+V is the terminal's text
paste — is a separate feature: `clipboard` reads text only, and an image would need
`pngpaste` / `wl-paste -t image/png` / `xclip -t image/png` plus a temp file fed to the
existing attachment flow. Codex's reader is the model: it prefers a file list on the
clipboard over raw pixels, because a Finder copy yields files while a browser copy
yields pixels, and encodes to PNG (`clipboard_paste.rs:51`). It is a wonton
`clipboard.ReadImage` follow-up, not part of this change. `terminal.PasteHandler`
(`terminal/key.go`) exists as a type, but nothing in v0.0.39 calls it; the input's own
paste path is the one to build on.

#### G8. Clicks that do something

Claude Code's fullscreen renderer makes clicks do: place the cursor in the input, pick
an option in a select or multi-select dialog, accept an autocomplete row, expand a
collapsed tool result, jump to the bottom, and open a link with Cmd or Ctrl. Once the
viewport maps a click to `(item, line)`, the transcript half of that list is nearly
free:

- **Click a tool call's header row** to toggle its output — the mouse equivalent of
  Ctrl+O. The header is line 0 of a tool-call message; the app checks the message
  kind, flips the flag, and `Invalidate`s the item.
- **Click a `report` message's title** to collapse it (`/context`, `/usage`).
- **Click the scroll indicator** to jump to the bottom, as Claude Code's floating
  `Jump to bottom` button does. The indicator lives in the footer, outside the
  viewport, so it registers as a clickable region the way `Button` does
  (`button_view.go:333`), and the runtime routes `MouseClick` to it
  (`runtime.go:382`).
- **Links** need nothing from the app: markdown links render as OSC 8
  (`tui/markdown.go:580`), and terminals open them with their own modifier-click
  whether or not the app has the mouse. Confirm per terminal in
  [K](#k-rollout-checklist).

Deferred: click-to-position the cursor in the input and click-to-pick in dialog lists
(`InputField` and the list views have no mouse handling; a wonton follow-up), and
hover effects (Claude Code highlights hovered rows, which needs the any-motion
tracking this design avoids).

#### G9. Search: hand the transcript to the terminal

Claude Code's fullscreen answer to `Cmd+F` is not an in-app search bar first. A key in
its transcript mode writes the whole conversation into the terminal's native
scrollback, tool output expanded, so `Cmd+F`, tmux copy mode, and native selection all
work on it until the user returns. A `/scrollback` command that does the same — leave
the alternate screen, print the static transcript with the exit-dump renderer from
[H](#h-exit-time-transcript-dump), wait for a key, re-enter and repaint — covers
search, bulk copy, and "let me read this in my own terminal" with almost no new code.
It needs one wonton addition, `Runtime.Suspend` ([A8](#a-proposed-wonton-additions)).
Codex adds one refinement worth copying: `/raw` re-renders its history as plain
source text "for clean terminal selection" (`chatwidget.rs:1630`). `/scrollback raw`
should print markdown source instead of rendered cells, so a native drag over it
yields text that pastes cleanly.

Search-bar sketch for later: `Ctrl+F` opens a find bar in the footer; matches are
computed over `Message.Content`, not rendered cells; `n` / `N` (or Enter / Shift+Enter
in the bar) call `ScrollToItem` on the next match; the footer shows `match 3/7`.
Highlighting matches in rendered cells becomes possible with the frame read path from
A7, the same way the selection is drawn.

### H. Exit-time transcript dump

After `terminal.Close()`:

1. Render the transcript with `messageView(msg, viewOpts{animate: false})` for every
   message, at the current terminal width, via `tui.Print`. Cap at the last 2,000
   rendered lines to bound both the time (a full 200-message render is ~50 ms) and the
   scrollback noise; print `… earlier messages omitted` above the cap.
2. Print `Session <id> saved — resume with dive --resume <id>`.

Skip step 1 when exiting with an error or after a panic; the resume line is still
worth printing when the session was saved. If a flag is added, use
`--exit-transcript=full|turn|none` with `full` as the default; `turn` prints only the
last user turn and response, which is what someone who just wants the answer in their
scrollback needs.

### I. Performance budget and instrumentation

**Fix first:** cache the git branch. Refresh on tick every 5 s, or at
`handleProcessingEnd`; never call `exec` from `View()`.

**Budget** at 200 messages, 100 columns, on the same M1: `View()` + measure + render
under 5 ms typical and 10 ms worst case; allocations under 2 MB per frame. Idle frames
cost the same as busy ones — the runtime renders every tick — so the idle figure is
the one to watch.

**Instrumentation:** `DIVE_DEBUG_FRAMES=1` enables `terminal.EnableMetrics()` and adds
`avg 1.8ms · max 9.2ms · 29.7 fps` from `terminal.GetMetrics()` to the status line.
Wonton's `DebugInfo` view can render the full snapshot if more is wanted.

**Regression guards** in the CLI's own tests:

- `BenchmarkView200`: synthesize a 200-message transcript, run
  `tui.Fprint(io.Discard, app.View(), WithWidth(100), WithHeight(40))`.
- A test using `testing.AllocsPerRun` over the same render with a generous ceiling
  (for example 20,000 allocations per frame). A regression to O(transcript) work
  blows through that by an order of magnitude; normal drift does not.

### J. Test plan

Real-terminal verification of these scenarios (iTerm2, OS-level input, screenshots) is
designed separately in [CLI Real-Terminal Testing](cli-real-terminal-testing.md).

- **Scroll math**: table tests over a `[]int` of heights and a gap — layout from
  anchor, follow, `ScrollBy` across item boundaries, clamping at both ends, resize
  re-clamp. These live in wonton if `Viewport` does.
- **Screen goldens** via `renderScreen(app, w, h)` (`tui.Fprint` with width _and_
  height into `termtest.Screen`): input on the bottom rows; last message visible with
  `follow`; the `↓ N new lines` row after `PageUp` plus a streamed append; a dialog
  replacing the input; rendering the same app at 100 then 60 columns with no
  truncated words; a `report` message re-wrapping.
- **Runtime-level**: `tui.NewTestTerminal(80, 24, &buf)`, `tui.NewRuntime`, a scripted
  `InputSource`, stream events through `rt.SendEvent`, then `rt.Stop()`; parse `buf`
  with `termtest.Screen` (it handles CSI cursor positioning and erase sequences) and
  assert the final screen. Wonton's `runtime_lifecycle_test.go` shows the shape.
- **Existing tests**: `renderLiveView` → `renderScreen`; direct `app.runner =
tui.NewInlineApp(...)` assignments → a fake `uiRunner` that records events.
  `TestAppWithInlineRunner` keeps exercising the inline path until it is deleted.
- **Exit dump**: render the dump for a transcript with animations and assert no
  animation frames leak into the output (no pulse colours), and that the cap and
  resume line appear.
- **Selection, pure**: the row ↔ `(item, line)` maps and the stream-selection column
  rules over a `[]int` of heights; extraction over a hand-built cell grid with wide
  characters (CJK, an emoji with a continuation cell), trailing spaces, and a gap
  row; word and line selection at Unicode boundaries; a reverse drag (focus before
  the anchor); clamping when the anchored item is invalidated or the width changes.
- **Selection, runtime**: script SGR bytes through the input source — `\x1b[<0;5;3M`
  press, `\x1b[<32;20;5M` drag, `\x1b[<0;20;5m` release — and assert that the parsed
  screen carries the reverse attribute on exactly the selected cells, and that the
  clipboard function injected into `App` received the expected text. A drag held at
  row −1 for five frames moves the anchor up five lines. A double-click sequence
  (press, release, press, release within the window) selects the word under it.
  Clipboard access goes through a function field on `App` so no test touches the real
  clipboard.
- **Paste**: a 200-line paste renders as a single placeholder row and `Value()`
  round-trips the text; a pasted file path still becomes `[Image #1]`.

### K. Rollout checklist

Terminals: iTerm2, Terminal.app (Kitty probe skipped), kitty, WezTerm, Ghostty,
Alacritty, tmux inside any of those (probe skipped, clipboard via tmux's buffer), the
VS Code integrated terminal, Windows Terminal, and one SSH session.

What the vendors document, as of 2026-09-02, so testers verify rather than discover;
the answers go into `/help` and the copy notice:

| Terminal             | Native-selection key with the mouse reported              | Copy-on-select default   | OSC 52 write                                              |
| -------------------- | --------------------------------------------------------- | ------------------------ | --------------------------------------------------------- |
| iTerm2               | Option                                                    | on                       | off until "Applications in terminal may access clipboard" |
| Terminal.app         | Fn (Claude Code's docs; Apple's are silent)               | off                      | unsupported                                               |
| kitty                | Shift                                                     | off                      | on; long single sequences truncated                       |
| WezTerm              | Shift                                                     | on                       | on                                                        |
| Ghostty              | Shift                                                     | on                       | on                                                        |
| Alacritty            | Shift                                                     | off (PRIMARY only)       | on                                                        |
| Windows Terminal     | Shift                                                     | off                      | on                                                        |
| GNOME Terminal / VTE | Shift                                                     | none                     | unsupported                                               |
| VS Code              | Alt; Option on macOS with `macOptionClickForcesSelection` | off                      | on locally; dropped over Remote SSH                       |
| tmux                 | the outer terminal's key                                  | copy mode → paste buffer | forwards its own buffers (`set-clipboard external`)       |

Record for each terminal:

- That the key above works, and that the hint names it.
- Whether `?1002` drags arrive with correct coordinates past column 223 (SGR mode),
  and whether the wheel still arrives during a drag.
- Whether OSC 52 lands on the local clipboard, including through tmux and SSH.
- Whether a file drop still pastes its path on the alternate screen with the mouse
  reported, and whether modifier-click on an OSC 8 link still opens it.
- Whether `?1007l` stops wheel-to-arrow translation after `/mouse` turns reporting off.
- Whether the terminal can clear the screen behind the app (Cmd+K in iTerm2 and
  Terminal.app). Wonton's diff renderer will not notice; Claude Code detects and
  repaints. At minimum, Ctrl+L forces a full repaint.
- Under tmux: `set -g mouse on` is required for the wheel and drags to reach the app
  (print a one-time hint if it is off, as Claude Code does); `tmux -CC` integration
  mode breaks the alternate screen and mouse tracking and should be documented as
  unsupported; tmux before 3.7 has no synchronized output, so expect flicker there.
- Windows Terminal and other ConPTY hosts can leave stale fragments from positioned
  writes; Claude Code ships a full-repaint escape hatch for exactly this. Plan for
  `DIVE_FULL_REPAINT=1`.

Scenarios per terminal:

- Resize while streaming; resize while scrolled up (the same message stays in view).
- `--resume` a 500-message session; PgUp to the top; wheel back down.
- Paste 5,000 lines into the input; the footer grows to its cap and the transcript
  stays pinned.
- Permission dialog arrives while scrolled up; approve it; Esc returns to bottom.
- `/clear` while scrolled up; `/copy` with and without a clipboard tool; `/mouse` then
  a plain drag-select and a wheel.
- Drag-select inside one message; across three messages with auto-scroll past the top;
  while the last message is streaming (the highlight must not drift); then paste the
  result into the input and check wide characters survived. Double-click a word,
  triple-click a line. Shift-drag (Option-drag) and confirm the terminal's own
  selection appears instead. Over SSH, confirm the copy landed on the local clipboard.
- `copy_on_select=false`: a drag highlights only, and `/copy` sends the highlight.
- Paste 200 lines: one placeholder row; Backspace removes it whole; submit sends the
  full text.
- Ctrl+C twice: the transcript and resume line are in scrollback; the shell prompt is
  intact; no stray escape bytes.
- Panic path (temporarily inject one): stack trace readable, terminal restored.
- `dive < file` and `dive | cat`: refused with the `--print` hint, no alternate-screen
  bytes emitted.

Flags: `--screen` / `DIVE_SCREEN=1` for one release; then default on with `--inline`;
then delete the inline path, the `messageViewStatic` family, and the two
`print…ToScrollback` helpers.

### L. Non-goals

- A scrollbar or percentage indicator (needs total height; see C).
- In-app search with highlighting (see G9).
- Rectangular (column) selection and hover effects. Extending a selection with
  Shift+arrows, as Claude Code allows, is a follow-up: it depends on Shift reaching
  the app on arrow keys, which the Kitty protocol guarantees and legacy encodings
  only sometimes do.
- Reconstructing source markdown from an arbitrary drag: drags copy cells, `/copy`
  copies source.
- Click-to-position the cursor in the input, and image paste from the clipboard
  (both wonton follow-ups, see G7 and G8).
- Converting the session picker.
- Persisting scroll position across `--resume`.
- Supporting terminals that lack the alternate screen (`TERM=dumb`): they get the
  `--print` hint.
