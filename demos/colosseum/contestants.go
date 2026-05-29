package main

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/arena"
	"github.com/deepnoodle-ai/dive/demos/colosseum/provider"
)

// modelOverrides collects repeated --model key=value flags.
type modelOverrides map[string]string

func (m modelOverrides) String() string { return "" }
func (m modelOverrides) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	k, val = strings.TrimSpace(k), strings.TrimSpace(val)
	if !ok || k == "" || val == "" {
		return fmt.Errorf("expected key=model, got %q", v)
	}
	spec, found := provider.Resolve(k)
	if !found {
		return provider.ErrUnknownProvider(k)
	}
	m[spec.Key] = val
	return nil
}

// seat is a parsed --players entry before id assignment.
type seat struct {
	base     string         // base id: provider key, or a remote label
	provider string         // provider key, or "a2a" for remote
	spec     *provider.Spec // set for local seats
	url      string         // set for remote (A2A) seats
}

// parseSeat interprets one --players token. Forms:
//
//	claude                       local provider seat
//	a2a:http://host:port         remote A2A seat (default label "remote")
//	mylabel@http://host:port     remote A2A seat with a custom label
//	http://host:port             remote A2A seat (shorthand)
func parseSeat(token string) (seat, error) {
	token = strings.TrimSpace(token)
	label, rest, hasLabel := strings.Cut(token, "@")
	if hasLabel && strings.Contains(rest, "://") {
		return seat{base: sanitizeLabel(label), provider: "a2a", url: rest}, nil
	}
	if u, ok := strings.CutPrefix(token, "a2a:"); ok {
		return seat{base: "remote", provider: "a2a", url: u}, nil
	}
	if strings.Contains(token, "://") {
		return seat{base: "remote", provider: "a2a", url: token}, nil
	}
	spec, found := provider.Resolve(token)
	if !found {
		return seat{}, provider.ErrUnknownProvider(token)
	}
	return seat{base: spec.Key, provider: spec.Key, spec: spec}, nil
}

func sanitizeLabel(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "remote"
	}
	return s
}

// buildContestants turns the --players list into seated contestants: it resolves
// each token, verifies local providers' API keys, picks model tiers, and gives
// duplicate base ids unique suffixes (e.g. claude-1, claude-2).
func buildContestants(playersCSV string, overrides modelOverrides, premium bool) ([]arena.Contestant, error) {
	names := splitCSV(playersCSV)
	if len(names) < 3 {
		return nil, fmt.Errorf("need at least 3 players, got %d", len(names))
	}

	seats := make([]seat, len(names))
	counts := map[string]int{}
	var missing []string
	seenMissing := map[string]bool{}
	for i, name := range names {
		s, err := parseSeat(name)
		if err != nil {
			return nil, err
		}
		seats[i] = s
		counts[s.base]++
		if s.spec != nil {
			if ok, keys := s.spec.EnvSatisfied(); !ok && !seenMissing[s.spec.Key] {
				seenMissing[s.spec.Key] = true
				missing = append(missing, fmt.Sprintf("%s (set one of: %s)", s.spec.Key, strings.Join(keys, ", ")))
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing API keys:\n  - %s", strings.Join(missing, "\n  - "))
	}

	idx := map[string]int{}
	contestants := make([]arena.Contestant, len(seats))
	for i, s := range seats {
		id := s.base
		if counts[s.base] > 1 {
			idx[s.base]++
			id = fmt.Sprintf("%s-%d", s.base, idx[s.base])
		}
		c := arena.Contestant{ID: id, Provider: s.provider}
		if s.spec != nil {
			model := s.spec.ModelFor(overrides[s.spec.Key], premium)
			c.Model = model
			c.LLM = s.spec.New(model)
		} else {
			c.RemoteURL = s.url
			c.Model = s.url
		}
		contestants[i] = c
	}
	return contestants, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
