// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/snowops-oss/snowopslabs/pkg/checks"
)

func TestNeeds_RequiredTargets(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{"nothing listed means the cluster", "name: s\n", []string{"k8s"}},
		{"an empty list means the cluster", "requires: []\n", []string{"k8s"}},
		{"the order written is kept", "requires: [git-forge, k8s]\n", []string{"git-forge", "k8s"}},
		{"blank entries are ignored", "requires: ['  ', k8s]\n", []string{"k8s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Scenario
			if err := yaml.Unmarshal([]byte(tt.yaml), &s); err != nil {
				t.Fatal(err)
			}
			if got := s.RequiredTargets(); !slices.Equal(got, tt.want) {
				t.Errorf("RequiredTargets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeeds_ValidateTargets(t *testing.T) {
	tests := []struct {
		name  string
		needs Needs
		uses  []TargetUse
		want  []string // substrings, one per expected message
	}{
		{"defaults are valid", Needs{}, []TargetUse{{`check "a"`, ""}}, nil},
		{"the default target needs no listing", Needs{}, []TargetUse{{`check "a"`, "k8s"}}, nil},
		{"a listed target is valid", Needs{Requires: []string{"k8s", "git-forge"}}, []TargetUse{{`component "c"`, "git-forge"}}, nil},
		{
			"a target that is not listed names where and what to add",
			Needs{}, []TargetUse{{`component "c"`, "git-forge"}},
			[]string{`component "c": target "git-forge" is not in requires (k8s); add it there`},
		},
		{
			"listing other targets does not list k8s",
			Needs{Requires: []string{"git-forge"}}, []TargetUse{{`check "ready"`, "k8s"}},
			[]string{`check "ready": target "k8s" is not in requires (git-forge)`},
		},
		{"a duplicate entry", Needs{Requires: []string{"k8s", "k8s"}}, nil, []string{`requires: target "k8s" is listed twice`}},
		{"an empty entry", Needs{Requires: []string{""}}, nil, []string{"requires: a target name is empty"}},
		{
			"every problem is reported",
			Needs{Requires: []string{"k8s", "k8s"}},
			[]TargetUse{{`check "a"`, "x"}, {`check "b"`, "y"}},
			[]string{"listed twice", `target "x"`, `target "y"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.needs.ValidateTargets(tt.uses)
			if len(got) != len(tt.want) {
				t.Fatalf("ValidateTargets() = %q, want %d message(s)", got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("message %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

// A scenario's validation covers the target of every component, in stages or
// not, and every check.
func TestScenario_ValidateTargets(t *testing.T) {
	base := func() *Scenario {
		return &Scenario{Name: "s", Components: []Component{{Name: "c", Type: "script", Script: "run.sh"}}}
	}
	tests := []struct {
		name   string
		mutate func(*Scenario)
		want   string
	}{
		{"unchanged scenario is valid", func(*Scenario) {}, ""},
		{"component on the cluster", func(s *Scenario) { s.Components[0].Target = "k8s" }, ""},
		{"component on an unlisted target", func(s *Scenario) { s.Components[0].Target = "git-forge" }, `component "c": target "git-forge" is not in requires`},
		{"component in a stage", func(s *Scenario) {
			s.Stages = []Stage{{Name: "one", Components: s.Components}}
			s.Components = nil
			s.Stages[0].Components[0].Target = "git-forge"
		}, `component "c": target "git-forge" is not in requires`},
		{"check on an unlisted target", func(s *Scenario) {
			s.Checks = []checks.Check{{Name: "ready", Type: "script", Script: "r.sh", Target: "git-forge"}}
		}, `check "ready": target "git-forge" is not in requires`},
		{"listed target is accepted", func(s *Scenario) {
			s.Requires = []string{"k8s", "git-forge"}
			s.Components[0].Target = "git-forge"
		}, ""},
		{"duplicate requires", func(s *Scenario) { s.Requires = []string{"k8s", "k8s"} }, `target "k8s" is listed twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base()
			tt.mutate(s)
			err := s.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want valid", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
