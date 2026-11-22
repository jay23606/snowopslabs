// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"
	"testing"
)

// A target name that is not registered, or one used but not listed, is
// reported against its own line in the file:line: [kind/name] message form.
func TestLoad_BadTargetsAreReportedAtTheirLine(t *testing.T) {
	cases := []struct {
		name, kind, dir, file, body, want string
		wantLineOf                        string // text of the line the problem points at
	}{
		{"scenario requires unknown", "scenario", "scenarios", "scenario.yaml", validScenario + "requires: [k8s, git-forge]\n",
			`requires: unknown target "git-forge" (use k8s)`, "requires: [k8s, git-forge]"},
		{"scenario requires unknown, block list", "scenario", "scenarios", "scenario.yaml", validScenario + "requires:\n  - tofu-state\n",
			`unknown target "tofu-state"`, "  - tofu-state"},
		{"scenario component on an unlisted target", "scenario", "scenarios", "scenario.yaml", strings.Replace(validScenario, "    script: run.sh\n", "    script: run.sh\n    target: git-forge\n", 1),
			`component "c1": target "git-forge" is not in requires (k8s)`, "    target: git-forge"},
		{"incident requires unknown", "incident", "incidents", "fault.yaml", validIncident + "requires: [github]\n",
			`unknown target "github"`, "requires: [github]"},
		{"incident inject on an unlisted target", "incident", "incidents", "fault.yaml", validIncident + "inject:\n  target: git-forge\n",
			`inject: target "git-forge" is not in requires`, "  target: git-forge"},
		{"challenge requires unknown", "challenge", "challenges", "challenge.yaml", validChallenge + "requires: [github]\n",
			`unknown target "github"`, "requires: [github]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			name := map[string]string{"scenario": "s1", "incident": "i1", "challenge": "ch1"}[tc.kind]
			write(t, root, tc.dir, name, tc.file, tc.body)
			c, _ := Load(root)
			p := findProblem(t, c.Problems(), tc.want)
			if string(p.Kind) != tc.kind || p.Name != name {
				t.Errorf("problem is for %s/%s, want %s/%s", p.Kind, p.Name, tc.kind, name)
			}
			lines := strings.Split(tc.body, "\n")
			if p.Line < 1 || p.Line > len(lines) || lines[p.Line-1] != tc.wantLineOf {
				t.Errorf("line = %d, want the line %q", p.Line, tc.wantLineOf)
			}
			if got := p.String(); !strings.Contains(got, "["+tc.kind+"/"+name+"] ") {
				t.Errorf("problem renders as %q, want file:line: [kind/name] message", got)
			}
		})
	}
}

// Content that lists the cluster, alone or with its own target, and content
// that lists nothing, loads without a problem.
func TestLoad_ValidTargetsLoadClean(t *testing.T) {
	root := t.TempDir()
	write(t, root, "scenarios", "s1", "scenario.yaml",
		strings.Replace(validScenario, "    script: run.sh\n", "    script: run.sh\n    target: k8s\n", 1)+"requires: [k8s]\n")
	write(t, root, "incidents", "i1", "fault.yaml", validIncident+"requires: [k8s]\ninject:\n  target: k8s\nresolve:\n  target: k8s\n")
	write(t, root, "challenges", "ch1", "challenge.yaml", validChallenge)
	c, _ := Load(root)
	if !c.IsValid() {
		t.Fatalf("expected no problems, got %v", c.Problems())
	}
	s, _ := c.Scenario("s1")
	if got := s.RequiredTargets(); len(got) != 1 || got[0] != "k8s" {
		t.Errorf("scenario requires %v, want [k8s]", got)
	}
	ch, _ := c.Challenge("ch1")
	if got := ch.RequiredTargets(); len(got) != 1 || got[0] != "k8s" {
		t.Errorf("challenge with no requires needs %v, want the default [k8s]", got)
	}
}

// A learning path cannot declare requires, so a module check may only name the
// default target; any other name is reported at its own line.
func TestLoad_PathCheckTargets(t *testing.T) {
	cases := []struct {
		name, target, want string
	}{
		{name: "the default target loads clean", target: "k8s"},
		{name: "no target loads clean"},
		{name: "an unknown target", target: "git-forge", want: `check target "git-forge" is unknown (use k8s)`},
		{name: "a known target other than the default", target: "k8s-extra", want: `check target "k8s-extra" is unknown`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			body := validPath
			if tc.target != "" {
				body = strings.Replace(validPath, "      script: x.sh\n", "      script: x.sh\n      target: "+tc.target+"\n", 1)
			}
			write(t, root, "learn", "p1", "path.yaml", body)
			write(t, root, "scenarios", "s1", "scenario.yaml", validScenario)
			c, _ := Load(root)
			if tc.want == "" {
				if !c.IsValid() {
					t.Fatalf("expected no problems, got %v", c.Problems())
				}
				return
			}
			p := findProblem(t, c.Problems(), tc.want)
			lines := strings.Split(body, "\n")
			if p.Line < 1 || lines[p.Line-1] != "      target: "+tc.target {
				t.Errorf("line = %d, want the target line", p.Line)
			}
			if got := p.String(); !strings.Contains(got, "[path/p1] ") {
				t.Errorf("problem renders as %q, want file:line: [kind/name] message", got)
			}
		})
	}
}
