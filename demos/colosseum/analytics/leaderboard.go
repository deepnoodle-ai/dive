package analytics

import (
	"encoding/json"
	"math"
	"os"
	"sort"
)

// DefaultElo is every model's starting rating.
const DefaultElo = 1000.0

// defaultK is the ELO K-factor — how much a single pairing moves a rating.
const defaultK = 24.0

// Rating is one model's cumulative tournament standing. Ratings are keyed by
// model id (not provider) so the leaderboard answers the headline question:
// "which model is actually best at Werewolf?" Aggregate sums are persisted so
// averages keep accumulating across runs.
type Rating struct {
	Model    string  `json:"model"`
	Provider string  `json:"provider"`
	Elo      float64 `json:"elo"`
	Matches  int     `json:"matches"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`

	GamesAsWolf    int `json:"games_as_wolf"`
	WolfWins       int `json:"wolf_wins"`
	GamesAsVillage int `json:"games_as_village"`
	VillageWins    int `json:"village_wins"`

	// Metric accumulators (sum + count) so per-run averages can continue.
	DeceptionSum  float64 `json:"deception_sum"`
	DeceptionN    int     `json:"deception_n"`
	DeductionSum  float64 `json:"deduction_sum"`
	DeductionN    int     `json:"deduction_n"`
	PersuasionSum float64 `json:"persuasion_sum"`
	PersuasionN   int     `json:"persuasion_n"`
}

// WinRate is wins per match played.
func (r *Rating) WinRate() float64 {
	if r.Matches == 0 {
		return 0
	}
	return float64(r.Wins) / float64(r.Matches)
}

func (r *Rating) DeceptionAvg() float64  { return avg(r.DeceptionSum, r.DeceptionN) }
func (r *Rating) DeductionAvg() float64  { return avg(r.DeductionSum, r.DeductionN) }
func (r *Rating) PersuasionAvg() float64 { return avg(r.PersuasionSum, r.PersuasionN) }

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// Leaderboard accumulates ratings across matches. It is JSON-serializable so a
// tournament can persist and resume the standings.
type Leaderboard struct {
	Ratings map[string]*Rating `json:"ratings"`
	K       float64            `json:"k"`
}

// NewLeaderboard returns an empty leaderboard.
func NewLeaderboard() *Leaderboard {
	return &Leaderboard{Ratings: map[string]*Rating{}, K: defaultK}
}

func (lb *Leaderboard) rating(model, provider string) *Rating {
	r, ok := lb.Ratings[model]
	if !ok {
		r = &Rating{Model: model, Provider: provider, Elo: DefaultElo}
		lb.Ratings[model] = r
	}
	return r
}

// Update folds a single match's analysis into the standings: it records each
// seat's win/loss and metrics, then applies pairwise ELO updates between the
// winning and losing teams. Multiple seats of the same model update that
// model's rating multiple times, which is the standard multiplayer treatment.
func (lb *Leaderboard) Update(a *MatchAnalysis) {
	if lb.K == 0 {
		lb.K = defaultK
	}

	// Per-seat bookkeeping.
	var winners, losers []PlayerResult
	for _, p := range a.Players {
		r := lb.rating(p.Model, p.Provider)
		r.Matches++
		if p.Won {
			r.Wins++
		} else if a.Winner != "" {
			r.Losses++
		}
		switch p.Team {
		case teamWerewolf:
			r.GamesAsWolf++
			if p.Won {
				r.WolfWins++
			}
		case teamVillage:
			r.GamesAsVillage++
			if p.Won {
				r.VillageWins++
			}
		}
		if p.Deception != nil {
			r.DeceptionSum += *p.Deception
			r.DeceptionN++
		}
		if p.Deduction != nil {
			r.DeductionSum += *p.Deduction
			r.DeductionN++
		}
		if p.Persuasion != nil {
			r.PersuasionSum += *p.Persuasion
			r.PersuasionN++
		}

		if a.Winner == "" {
			continue // undecided match: count it but don't move ELO
		}
		if p.Won {
			winners = append(winners, p)
		} else {
			losers = append(losers, p)
		}
	}

	// Pairwise ELO: each winner beat each loser.
	for _, w := range winners {
		for _, l := range losers {
			lb.updatePair(w.Model, l.Model)
		}
	}
}

// updatePair applies one ELO game where `winner` beat `loser`.
func (lb *Leaderboard) updatePair(winner, loser string) {
	wr := lb.Ratings[winner]
	lr := lb.Ratings[loser]
	if wr == nil || lr == nil {
		return
	}
	expW := 1.0 / (1.0 + math.Pow(10, (lr.Elo-wr.Elo)/400.0))
	expL := 1.0 - expW
	wr.Elo += lb.K * (1.0 - expW)
	lr.Elo += lb.K * (0.0 - expL)
}

// Standings returns ratings sorted by ELO (descending), tie-broken by win rate.
func (lb *Leaderboard) Standings() []*Rating {
	out := make([]*Rating, 0, len(lb.Ratings))
	for _, r := range lb.Ratings {
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Elo != out[j].Elo {
			return out[i].Elo > out[j].Elo
		}
		if out[i].WinRate() != out[j].WinRate() {
			return out[i].WinRate() > out[j].WinRate()
		}
		return out[i].Model < out[j].Model // deterministic final tiebreak
	})
	return out
}

// Save writes the leaderboard to a JSON file.
func (lb *Leaderboard) Save(path string) error {
	data, err := json.MarshalIndent(lb, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadLeaderboard reads a leaderboard from a JSON file, returning a fresh empty
// one if the file does not exist.
func LoadLeaderboard(path string) (*Leaderboard, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewLeaderboard(), nil
	}
	if err != nil {
		return nil, err
	}
	lb := NewLeaderboard()
	if err := json.Unmarshal(data, lb); err != nil {
		return nil, err
	}
	if lb.Ratings == nil {
		lb.Ratings = map[string]*Rating{}
	}
	if lb.K == 0 {
		lb.K = defaultK
	}
	return lb, nil
}
