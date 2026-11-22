// SPDX-License-Identifier: Apache-2.0

package learn

import (
	"strings"
	"testing"
)

// minutes and estimatedMinutes are two names for one number: either fills
// the other, both may be set when they agree, and a disagreement is an error.
func TestLoadPath_MinutesAlias(t *testing.T) {
	tests := []struct {
		name        string
		extra       string
		wantMinutes int
		wantAlias   int
		wantErr     string
	}{
		{"neither", "", 0, 0, ""},
		{"minutes only", "minutes: 30\n", 30, 30, ""},
		{"estimatedMinutes only", "estimatedMinutes: 45\n", 45, 45, ""},
		{"both agree", "minutes: 20\nestimatedMinutes: 20\n", 20, 20, ""},
		{"both disagree", "minutes: 20\nestimatedMinutes: 25\n", 0, 0, "minutes (20) and estimatedMinutes (25) disagree"},
		{"negative alias", "estimatedMinutes: -2\n", 0, 0, "estimatedMinutes must be a positive number of minutes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, dir := makeEngine(t)
			writePathYAML(t, dir, "test-path", validPathYAML+tt.extra)
			p, err := e.LoadPath("test-path")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadPath() error = %v, want it to say %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Minutes != tt.wantMinutes || p.EstimatedMinutes != tt.wantAlias {
				t.Errorf("minutes = %d, estimatedMinutes = %d, want %d and %d", p.Minutes, p.EstimatedMinutes, tt.wantMinutes, tt.wantAlias)
			}
		})
	}
}

func TestLoadPath_Meta(t *testing.T) {
	e, dir := makeEngine(t)
	writePathYAML(t, dir, "plain", strings.Replace(validPathYAML, "test-path", "plain", 1))
	writePathYAML(t, dir, "tracked", strings.Replace(validPathYAML, "test-path", "tracked", 1)+"track: iac\nlevel: advanced\n")
	writePathYAML(t, dir, "typo", strings.Replace(validPathYAML, "test-path", "typo", 1)+"track: terraform\n")

	if p, err := e.LoadPath("plain"); err != nil || p.Track != "kubernetes" || p.Level != "intermediate" {
		t.Errorf("plain = %+v, %v, want the defaults", p, err)
	}
	if p, err := e.LoadPath("tracked"); err != nil || p.Track != "iac" || p.Level != "advanced" {
		t.Errorf("tracked = %+v, %v, want iac and advanced", p, err)
	}
	if _, err := e.LoadPath("typo"); err == nil || !strings.Contains(err.Error(), `track "terraform" is not a known track`) {
		t.Errorf("typo error = %v, want it to name the bad track", err)
	}
}
