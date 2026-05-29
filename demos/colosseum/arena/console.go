package arena

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// console renders the live play-by-play to a terminal. Colors are plain ANSI so
// the demo has zero UI dependencies; they are disabled when output is not a
// terminal or when NO_COLOR is set.
type console struct {
	w     io.Writer
	color bool
}

const (
	cReset  = "\033[0m"
	cDim    = "\033[2m"
	cBold   = "\033[1m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cBlue   = "\033[34m"
	cPurple = "\033[35m"
	cCyan   = "\033[36m"
)

func newConsole(w io.Writer) *console {
	color := os.Getenv("NO_COLOR") == ""
	if f, ok := w.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			color = false // piped/redirected: no escape codes
		}
	}
	return &console{w: w, color: color}
}

func (c *console) paint(code, s string) string {
	if !c.color {
		return s
	}
	return code + s + cReset
}

func (c *console) line(s string)             { fmt.Fprintln(c.w, s) }
func (c *console) printf(f string, a ...any) { fmt.Fprintf(c.w, f, a...) }

// banner prints a phase or section header.
func (c *console) banner(code, title string) {
	bar := strings.Repeat("─", 60)
	c.line("")
	c.line(c.paint(code, bar))
	c.line(c.paint(code+cBold, "  "+title))
	c.line(c.paint(code, bar))
}

func (c *console) night(round int) { c.banner(cBlue, fmt.Sprintf("🌙 ROUND %d — NIGHT", round)) }
func (c *console) day(round int)   { c.banner(cYellow, fmt.Sprintf("☀️  ROUND %d — DAY", round)) }

func (c *console) speak(actor, msg string) {
	c.printf("%s %s\n", c.paint(cBold+cCyan, actor+":"), msg)
}

func (c *console) reasoning(actor, reasoning string) {
	c.printf("    %s\n", c.paint(cDim, "↳ ("+actor+" thinks) "+reasoning))
}

func (c *console) vote(actor, target string) {
	if target == "" {
		c.printf("  🗳  %s %s\n", c.paint(cBold, actor), c.paint(cDim, "abstains"))
		return
	}
	c.printf("  🗳  %s → %s\n", c.paint(cBold, actor), c.paint(cRed, target))
}

func (c *console) death(msg string)  { c.line("  " + c.paint(cRed, "☠  "+msg)) }
func (c *console) info(msg string)   { c.line("  " + c.paint(cDim, msg)) }
func (c *console) reveal(msg string) { c.line("  " + c.paint(cPurple, "🔎 "+msg)) }
func (c *console) warn(msg string)   { c.line("  " + c.paint(cYellow, "⚠  "+msg)) }
func (c *console) result(msg string) { c.line(c.paint(cBold+cGreen, msg)) }
