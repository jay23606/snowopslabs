// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMeta_Validate(t *testing.T) {
	tests := []struct {
		name string
		meta Meta
		want string // substring of the one expected message; "" means valid
	}{
		{"empty takes the defaults", Meta{}, ""},
		{"every track", Meta{Track: "iac", Level: "advanced", Minutes: 90}, ""},
		{"unknown track names the valid ones", Meta{Track: "k8s"}, `track "k8s" is not a known track (use kubernetes, cicd or iac)`},
		{"track is case sensitive", Meta{Track: "CICD"}, `track "CICD"`},
		{"unknown level names the valid ones", Meta{Level: "easy"}, `level "easy" is not a known level (use beginner, intermediate or advanced)`},
		{"negative minutes", Meta{Minutes: -3}, "minutes must be a positive number of minutes, got -3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.meta.Validate()
			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("Validate() = %v, want valid", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tt.want) {
				t.Fatalf("Validate() = %v, want one message containing %q", got, tt.want)
			}
		})
	}
}

func TestMeta_ValidateReportsEveryField(t *testing.T) {
	got := Meta{Track: "x", Level: "y", Minutes: -1}.Validate()
	if len(got) != 3 {
		t.Fatalf("Validate() = %v, want a message per bad field", got)
	}
}

func TestMeta_ApplyDefaults(t *testing.T) {
	m := Meta{Minutes: 20}
	m.ApplyDefaults()
	if m.Track != DefaultTrack || m.Level != DefaultLevel || m.Minutes != 20 {
		t.Errorf("ApplyDefaults() = %+v, want default track and level and minutes kept", m)
	}
	set := Meta{Track: " cicd ", Level: "beginner"}
	set.ApplyDefaults()
	if set.Track != "cicd" || set.Level != "beginner" {
		t.Errorf("ApplyDefaults() changed what the author set: %+v", set)
	}
}

func TestParseFilter_AliasNormalises(t *testing.T) {
	f, err := ParseFilter(" k8s ", "")
	if err != nil || f.Track != "kubernetes" {
		t.Errorf("ParseFilter(k8s) = %+v, %v, want track kubernetes", f, err)
	}
	if errs := (Meta{Track: "k8s"}).Validate(); len(errs) != 1 {
		t.Errorf("content must stay strict about aliases, Validate() = %v", errs)
	}
}

func TestLabels(t *testing.T) {
	for _, id := range Tracks {
		if TrackLabel(id) == id {
			t.Errorf("track %q has no display label", id)
		}
	}
	for _, id := range Levels {
		if LevelLabel(id) == id {
			t.Errorf("level %q has no display label", id)
		}
	}
	tests := []struct{ got, want string }{
		{TrackLabel("kubernetes"), "Kubernetes"}, {TrackLabel("cicd"), "CI/CD"}, {TrackLabel("iac"), "IaC"},
		{TrackLabel("mystery"), "mystery"},
		{LevelLabel("beginner"), "Beginner"}, {LevelLabel("intermediate"), "Intermediate"},
		{LevelLabel("advanced"), "Advanced"}, {LevelLabel("mystery"), "mystery"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("label = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		track, level string
		want         string // error substring, "" means ok
	}{
		{"", "", ""},
		{"cicd", "beginner", ""},
		{" iac ", "", ""},
		{"kube", "", `unknown track "kube": use kubernetes, cicd or iac`},
		{"", "easy", `unknown level "easy": use beginner, intermediate or advanced`},
	}
	for _, tt := range tests {
		f, err := ParseFilter(tt.track, tt.level)
		if tt.want == "" {
			if err != nil {
				t.Errorf("ParseFilter(%q, %q) = %v, want ok", tt.track, tt.level, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ParseFilter(%q, %q) = %+v, %v, want error containing %q", tt.track, tt.level, f, err, tt.want)
		}
	}
}

// An item that never states a track or level is a kubernetes, intermediate
// item, so the filter for those values finds it.
func TestFilter_Match(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		meta   Meta
		want   bool
	}{
		{"empty filter matches all", Filter{}, Meta{Track: "iac"}, true},
		{"track hit", Filter{Track: "cicd"}, Meta{Track: "cicd"}, true},
		{"track miss", Filter{Track: "cicd"}, Meta{Track: "iac"}, false},
		{"level miss", Filter{Level: "beginner"}, Meta{Level: "advanced"}, false},
		{"both must hold", Filter{Track: "cicd", Level: "beginner"}, Meta{Track: "cicd", Level: "advanced"}, false},
		{"missing track is kubernetes", Filter{Track: "kubernetes"}, Meta{}, true},
		{"missing level is intermediate", Filter{Level: "intermediate"}, Meta{}, true},
		{"missing track is not cicd", Filter{Track: "cicd"}, Meta{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Match(tt.meta); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	items := []Scenario{
		{Name: "a", Meta: Meta{Track: "cicd", Level: "beginner"}},
		{Name: "b", Meta: Meta{Track: "kubernetes", Level: "advanced"}},
		{Name: "c"},
	}
	names := func(in []Scenario) string {
		var out []string
		for _, s := range in {
			out = append(out, s.Name)
		}
		return strings.Join(out, ",")
	}
	if got := names(Select(items, Filter{})); got != "a,b,c" {
		t.Errorf("an empty filter should keep every item in order, got %s", got)
	}
	if got := names(Select(items, Filter{Track: "kubernetes"})); got != "b,c" {
		t.Errorf("track kubernetes = %s, want b,c (c has the default)", got)
	}
	if got := Select(items, Filter{Track: "iac"}); len(got) != 0 {
		t.Errorf("track iac = %v, want none", got)
	}
}

// The three fields come from the item's own YAML, side by side with its other
// keys.
func TestMeta_ReadsInlineFromYAML(t *testing.T) {
	var s Scenario
	doc := "name: x\ncategory: delivery\ntrack: cicd\nlevel: beginner\nminutes: 20\n"
	if err := yaml.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatal(err)
	}
	if s.Track != "cicd" || s.Level != "beginner" || s.Minutes != 20 {
		t.Errorf("Meta = %+v, want the values from the YAML", s.Meta)
	}
}

func TestScenarioValidate_RejectsBadMeta(t *testing.T) {
	s := Scenario{Name: "x", Meta: Meta{Track: "k8s"}}
	err := s.Validate()
	if err == nil || !strings.Contains(err.Error(), `track "k8s" is not a known track`) {
		t.Errorf("Validate() = %v, want the track problem", err)
	}
}
