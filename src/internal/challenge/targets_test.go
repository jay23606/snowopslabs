// SPDX-License-Identifier: Apache-2.0

package challenge

import (
	"slices"
	"strings"
	"testing"
)

func TestChallengeValidate_Targets(t *testing.T) {
	base := func() *Challenge {
		return &Challenge{
			Name: "c", DisplayName: "C",
			Setup:   SetupSpec{Type: "scenario", Ref: "s"},
			Grading: GradingSpec{Checks: []GCheck{{Name: "ready", Type: "script", Script: "r.sh"}}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*Challenge)
		wantErr string
	}{
		{"nothing set is valid", func(*Challenge) {}, ""},
		{"a check on the cluster", func(c *Challenge) { c.Grading.Checks[0].Target = "k8s" }, ""},
		{"a check on an unlisted target", func(c *Challenge) { c.Grading.Checks[0].Target = "git-forge" }, `check "ready": target "git-forge" is not in requires`},
		{"a listed target is accepted", func(c *Challenge) {
			c.Requires = []string{"k8s", "git-forge"}
			c.Grading.Checks[0].Target = "git-forge"
		}, ""},
		{"duplicate requires", func(c *Challenge) { c.Requires = []string{"k8s", "k8s"} }, `target "k8s" is listed twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(c)
			errs := c.Validate()
			if tt.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("Validate() = %v, want valid", errs)
				}
				return
			}
			if !strings.Contains(strings.Join(errs, "\n"), tt.wantErr) {
				t.Fatalf("Validate() = %v, want it to contain %q", errs, tt.wantErr)
			}
		})
	}
}

func TestLoad_RequiresFromYAML(t *testing.T) {
	tests := []struct {
		name  string
		extra string
		want  []string
	}{
		{"default is the cluster", "", []string{"k8s"}},
		{"listed targets", "requires: [k8s, git-forge]\n", []string{"k8s", "git-forge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, cDir := makeEngine(t)
			writeChallengeYAML(t, cDir, "test-challenge", validChallengeYAML+tt.extra)
			c, err := e.Load("test-challenge")
			if err != nil {
				t.Fatal(err)
			}
			if got := c.RequiredTargets(); !slices.Equal(got, tt.want) {
				t.Errorf("RequiredTargets() = %v, want %v", got, tt.want)
			}
		})
	}
}
