// SPDX-License-Identifier: Apache-2.0

package challenge

import (
	"strings"
	"testing"
)

func TestLoad_Meta(t *testing.T) {
	tests := []struct {
		name      string
		extra     string
		wantTrack string
		wantLevel string
		wantMin   int
		wantErr   string
	}{
		{"defaults", "", "kubernetes", "intermediate", 0, ""},
		{"stated", "track: cicd\nlevel: beginner\nminutes: 12\n", "cicd", "beginner", 12, ""},
		{"unknown track", "track: k8s\n", "", "", 0, `track "k8s" is not a known track (use kubernetes, cicd or iac)`},
		{"unknown level", "level: easy\n", "", "", 0, `level "easy" is not a known level`},
		{"negative minutes", "minutes: -4\n", "", "", 0, "minutes must be a positive number of minutes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, cDir := makeEngine(t)
			writeChallengeYAML(t, cDir, "test-challenge", validChallengeYAML+tt.extra)
			c, err := e.Load("test-challenge")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %v, want it to say %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Track != tt.wantTrack || c.Level != tt.wantLevel || c.Minutes != tt.wantMin {
				t.Errorf("Meta = %+v, want %s, %s, %d", c.Meta, tt.wantTrack, tt.wantLevel, tt.wantMin)
			}
		})
	}
}
