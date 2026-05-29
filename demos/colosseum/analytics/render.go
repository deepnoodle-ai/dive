package analytics

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// RenderStandings writes the leaderboard as an aligned text table, sorted by
// ELO. It is the terminal view of "which model is actually best at Werewolf?".
func RenderStandings(w io.Writer, lb *Leaderboard) {
	standings := lb.Standings()
	if len(standings) == 0 {
		fmt.Fprintln(w, "No matches recorded yet.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tMODEL\tELO\tW-L\tWIN%\tWOLF W%\tDECEPT\tDEDUCT\tPERSUADE")
	for i, r := range standings {
		fmt.Fprintf(tw, "%d\t%s\t%.0f\t%d-%d\t%.0f%%\t%s\t%.2f\t%.2f\t%.2f\n",
			i+1, r.Model, r.Elo, r.Wins, r.Losses, 100*r.WinRate(),
			wolfWinPct(r), r.DeceptionAvg(), r.DeductionAvg(), r.PersuasionAvg())
	}
	tw.Flush()
}

func wolfWinPct(r *Rating) string {
	if r.GamesAsWolf == 0 {
		return "  —"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(r.WolfWins)/float64(r.GamesAsWolf))
}

// RenderHighlights writes highlight cards as a readable list.
func RenderHighlights(w io.Writer, hs []Highlight) {
	if len(hs) == 0 {
		fmt.Fprintln(w, "No highlights detected.")
		return
	}
	for _, h := range hs {
		fmt.Fprintf(w, "★ [R%d] %s\n", h.Round, h.Title)
		if h.Detail != "" {
			fmt.Fprintf(w, "    %s\n", h.Detail)
		}
	}
}

// RenderMatch writes a one-match summary: outcome, per-player metrics, and
// highlights.
func RenderMatch(w io.Writer, a *MatchAnalysis) {
	fmt.Fprintf(w, "Match (seed %d) — winner: %s in %d rounds\n\n", a.Seed, orNone(a.Winner), a.Rounds)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PLAYER\tMODEL\tROLE\tRESULT\tDECEPT\tDEDUCT\tPERSUADE")
	for _, p := range a.Players {
		result := "lost"
		if p.Won {
			result = "WON"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			p.ID, p.Model, p.Role, result,
			metric(p.Deception), metric(p.Deduction), metric(p.Persuasion))
	}
	tw.Flush()
	fmt.Fprintln(w)
	RenderHighlights(w, a.Highlights)
}

func metric(v *float64) string {
	if v == nil {
		return "  —"
	}
	return fmt.Sprintf("%.2f", *v)
}

func orNone(s string) string {
	if s == "" {
		return "(undecided)"
	}
	return s
}
