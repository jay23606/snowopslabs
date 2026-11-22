// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"slices"
	"strings"
)

// Tracks are the areas of platform work a catalog item can belong to. Adding
// a track to this list is the only code change a new one needs.
var Tracks = []string{"kubernetes", "cicd", "iac"}

// Levels are the difficulty steps, easiest first.
var Levels = []string{"beginner", "intermediate", "advanced"}

var trackLabels = map[string]string{"kubernetes": "Kubernetes", "cicd": "CI/CD", "iac": "IaC"}

var levelLabels = map[string]string{"beginner": "Beginner", "intermediate": "Intermediate", "advanced": "Advanced"}

// TrackLabel is the name a learner reads for a track id, such as "CI/CD" for
// "cicd". An unknown id is returned unchanged.
func TrackLabel(id string) string {
	if l, ok := trackLabels[id]; ok {
		return l
	}
	return id
}

// LevelLabel is the name a learner reads for a level id, such as "Beginner"
// for "beginner". An unknown id is returned unchanged.
func LevelLabel(id string) string {
	if l, ok := levelLabels[id]; ok {
		return l
	}
	return id
}

// Defaults for an item that sets neither field.
const (
	DefaultTrack = "kubernetes"
	DefaultLevel = "intermediate"
)

// Meta is the browsing metadata every scenario, incident, challenge and
// learning path carries, so a learner can filter the catalog by area, level
// and time. It is embedded inline in each item's YAML.
type Meta struct {
	// Track is the area of work: one of Tracks. Empty means DefaultTrack.
	Track string `yaml:"track,omitempty" json:"track"`
	// Level is the difficulty: one of Levels. Empty means DefaultLevel.
	Level string `yaml:"level,omitempty" json:"level"`
	// Minutes is the expected time in minutes. Zero means not stated.
	Minutes int `yaml:"minutes,omitempty" json:"minutes,omitempty"`
}

// ApplyDefaults fills an empty track and level, so listings always show both.
func (m *Meta) ApplyDefaults() {
	m.Track = strings.TrimSpace(m.Track)
	m.Level = strings.TrimSpace(m.Level)
	if m.Track == "" {
		m.Track = DefaultTrack
	}
	if m.Level == "" {
		m.Level = DefaultLevel
	}
}

// Validate reports every malformed field in the words an author needs to fix
// it. Empty track and level are valid, since they take the defaults.
func (m Meta) Validate() []string {
	var errs []string
	if t := strings.TrimSpace(m.Track); t != "" && !slices.Contains(Tracks, t) {
		errs = append(errs, fmt.Sprintf("track %q is not a known track (use %s)", t, orList(Tracks)))
	}
	if l := strings.TrimSpace(m.Level); l != "" && !slices.Contains(Levels, l) {
		errs = append(errs, fmt.Sprintf("level %q is not a known level (use %s)", l, orList(Levels)))
	}
	if m.Minutes < 0 {
		errs = append(errs, fmt.Sprintf("minutes must be a positive number of minutes, got %d", m.Minutes))
	}
	return errs
}

// Filter selects catalog items by track and level. An empty field matches
// everything.
type Filter struct {
	Track string
	Level string
}

// trackAliases are short spellings a filter accepts for a track id. Content
// files must still use the id.
var trackAliases = map[string]string{"k8s": "kubernetes"}

// ParseFilter checks the values of a --track or ?track= filter, so a typo is
// an error that names the valid values instead of an empty list. Case is
// ignored, and a track alias such as k8s is normalised to its id.
func ParseFilter(track, level string) (Filter, error) {
	f := Filter{
		Track: strings.ToLower(strings.TrimSpace(track)),
		Level: strings.ToLower(strings.TrimSpace(level)),
	}
	if id, ok := trackAliases[f.Track]; ok {
		f.Track = id
	}
	if f.Track != "" && !slices.Contains(Tracks, f.Track) {
		return Filter{}, fmt.Errorf("unknown track %q: use %s", f.Track, orList(Tracks))
	}
	if f.Level != "" && !slices.Contains(Levels, f.Level) {
		return Filter{}, fmt.Errorf("unknown level %q: use %s", f.Level, orList(Levels))
	}
	return f, nil
}

// Match reports whether an item passes the filter. A missing track or level
// counts as its default.
func (f Filter) Match(m Meta) bool {
	m.ApplyDefaults()
	return (f.Track == "" || f.Track == m.Track) && (f.Level == "" || f.Level == m.Level)
}

// Active reports whether the filter narrows anything.
func (f Filter) Active() bool { return f.Track != "" || f.Level != "" }

// Classified is anything that carries Meta. Embedding Meta provides it.
type Classified interface{ Classification() Meta }

// Classification returns the metadata itself, so a type that embeds Meta
// satisfies Classified.
func (m Meta) Classification() Meta { return m }

// Select returns the items that pass the filter, in their original order. An
// inactive filter returns items unchanged.
func Select[T Classified](items []T, f Filter) []T {
	if !f.Active() {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if f.Match(it.Classification()) {
			out = append(out, it)
		}
	}
	return out
}

// orList renders "a, b or c".
func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}
