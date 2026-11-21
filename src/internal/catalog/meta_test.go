// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"
	"testing"
)

// A bad track, level or minutes value is reported against its own line in the
// file:line: [kind/name] message form, for every kind of content.
func TestLoad_BadMetaIsReportedAtItsLine(t *testing.T) {
	cases := []struct {
		name, kind, dir, file, body string
		want                        string
	}{
		{"scenario track", "scenario", "scenarios", "scenario.yaml", validScenario + "track: k8s\n", `track "k8s" is not a known track (use kubernetes, cicd or iac)`},
		{"scenario level", "scenario", "scenarios", "scenario.yaml", validScenario + "level: easy\n", `level "easy" is not a known level (use beginner, intermediate or advanced)`},
		{"scenario minutes", "scenario", "scenarios", "scenario.yaml", validScenario + "minutes: -5\n", "minutes must be a positive number of minutes, got -5"},
		{"incident track", "incident", "incidents", "fault.yaml", validIncident + "track: terraform\n", `track "terraform" is not a known track`},
		{"challenge level", "challenge", "challenges", "challenge.yaml", validChallenge + "level: hard\n", `level "hard" is not a known level`},
		{"path minutes", "path", "learn", "path.yaml", validPath + "minutes: -1\n", "minutes must be a positive number of minutes"},
		{"path alias disagrees", "path", "learn", "path.yaml", validPath + "minutes: 30\nestimatedMinutes: 45\n", "minutes (30) and estimatedMinutes (45) disagree"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			name := map[string]string{"scenario": "s1", "incident": "i1", "path": "p1", "challenge": "ch1"}[tc.kind]
			write(t, root, tc.dir, name, tc.file, tc.body)
			c, _ := Load(root)
			p := findProblem(t, c.Problems(), tc.want)
			if string(p.Kind) != tc.kind || p.Name != name {
				t.Errorf("problem is for %s/%s, want %s/%s", p.Kind, p.Name, tc.kind, name)
			}
			wantLine := 0
			for i, ln := range strings.Split(tc.body, "\n") {
				if strings.HasPrefix(ln, "track:") || strings.HasPrefix(ln, "level:") || strings.HasPrefix(ln, "minutes:") {
					wantLine = i + 1
					break
				}
			}
			if p.Line != wantLine {
				t.Errorf("line = %d, want %d, the line of the bad key", p.Line, wantLine)
			}
			if got := p.String(); !strings.Contains(got, ":") || !strings.Contains(got, "["+tc.kind+"/"+name+"] ") {
				t.Errorf("problem renders as %q, want file:line: [kind/name] message", got)
			}
		})
	}
}

// Valid values load without a problem, and an item that sets none gets the
// defaults.
func TestLoad_MetaDefaultsAndValidValues(t *testing.T) {
	root := t.TempDir()
	write(t, root, "scenarios", "s1", "scenario.yaml", validScenario)
	write(t, root, "incidents", "i1", "fault.yaml", validIncident+"track: cicd\nlevel: advanced\nminutes: 25\n")
	write(t, root, "learn", "p1", "path.yaml", validPath+"estimatedMinutes: 40\n")
	write(t, root, "challenges", "ch1", "challenge.yaml", validChallenge+"level: beginner\n")

	c, _ := Load(root)
	if !c.IsValid() {
		t.Fatalf("expected no problems, got %v", c.Problems())
	}
	s, _ := c.Scenario("s1")
	if s.Track != "kubernetes" || s.Level != "intermediate" || s.Minutes != 0 {
		t.Errorf("scenario meta = %+v, want the defaults and no minutes", s.Meta)
	}
	i, _ := c.Incident("i1")
	if i.Track != "cicd" || i.Level != "advanced" || i.Minutes != 25 {
		t.Errorf("incident meta = %+v, want what the file says", i.Meta)
	}
	p, _ := c.Path("p1")
	if p.Minutes != 40 || p.EstimatedMinutes != 40 {
		t.Errorf("path minutes = %d / %d, want estimatedMinutes to fill minutes", p.Minutes, p.EstimatedMinutes)
	}
	ch, _ := c.Challenge("ch1")
	if ch.Track != "kubernetes" || ch.Level != "beginner" {
		t.Errorf("challenge meta = %+v, want the default track and the stated level", ch.Meta)
	}
}
