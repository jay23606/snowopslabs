// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/labtest"
	schema "github.com/snowops-oss/snowopslabs/pkg/scenario"
)

// writeTrackedContent adds one beginner CI/CD item of every kind to the
// fixture lab, next to the fixture's own items that set neither field.
func writeTrackedContent(t *testing.T, l *terminal) {
	t.Helper()
	l.Write(t, "scenarios/pipeline-cache/scenario.yaml", `apiVersion: scenario.snowops.net/v2
name: pipeline-cache
displayName: Pipeline cache
description: A cache key that ships the wrong build
category: delivery
track: cicd
level: beginner
minutes: 20
components: []
`)
	l.Write(t, "incidents/stale-cache/fault.yaml", `name: stale-cache
displayName: "Stale cache"
description: "{{.WorkloadName}} ships an old build"
category: workload
severity: low
track: cicd
level: advanced
minutes: 25
target:
  namespace: "{{.WorkloadNamespace}}"
  workload: "{{.WorkloadName}}"
prerequisites:
  apps: ["{{.WorkloadName}}"]
detection:
  name: marker-gone
  type: script
  script: checks/resolved.sh
  timeoutSeconds: 5
`)
	for _, f := range []string{"inject.sh", "resolve.sh", "hints.md", "solution.md", "checks/resolved.sh"} {
		l.Write(t, "incidents/stale-cache/"+f, "#!/bin/sh\nexit 0\n")
	}
	l.Write(t, "challenges/cache-bust/challenge.yaml", `name: cache-bust
displayName: "Bust the cache"
description: "Fix the stale cache"
category: workload
track: cicd
level: beginner
minutes: 12
parTime: "10m"
setup:
  type: incident
  ref: stale-cache
grading:
  useDetectionCheck: true
`)
	l.Write(t, "learn/ci-basics/path.yaml", `name: ci-basics
displayName: "CI basics"
description: "A pipeline path"
track: cicd
level: beginner
minutes: 30
modules:
  - name: step-one
    displayName: "Step one"
    action:
      type: command
      ref: "labctl runtime up"
    check:
      name: cluster-up
      type: script
      script: checks/cluster-up.sh
      timeoutSeconds: 30
`)
	l.Write(t, "learn/ci-basics/checks/cluster-up.sh", "#!/bin/sh\nexit 0\n")
}

// The list commands narrow the catalog by --track and --level, and every
// item without either field shows the defaults.
func TestCatalogFilters_CLI(t *testing.T) {
	cases := []struct {
		name string
		list []string
		// tracked is the item the filters can single out; fixture is the
		// fixture lab's item that sets neither field.
		tracked, fixture string
	}{
		{"scenarios", []string{"scenario", "list"}, "pipeline-cache", labtest.Scenario},
		{"incidents", []string{"incident", "list"}, "stale-cache", labtest.Fault},
		{"challenges", []string{"challenge", "list"}, "cache-bust", labtest.Challenge},
		{"paths", []string{"learn", "list"}, "ci-basics", labtest.Path},
	}
	// level is the tracked item's level for each case, used to build a
	// filter that matches it.
	levels := map[string]string{"scenarios": "beginner", "incidents": "advanced", "challenges": "beginner", "paths": "beginner"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := newTerminal(t)
			writeTrackedContent(t, l)
			run := func(args ...string) cliResult {
				t.Helper()
				return l.run(append(append([]string{}, tc.list...), args...)...)
			}
			listed := func(r cliResult, name string) bool { return strings.Contains(r.stdout, name+" ") }

			all := run()
			if all.err != nil || !listed(all, tc.tracked) || !listed(all, tc.fixture) {
				t.Fatalf("an unfiltered list should show both items, got err=%v\n%s", all.err, all.stdout)
			}
			if !strings.Contains(all.stdout, "TRACK") || !strings.Contains(all.stdout, "LEVEL") {
				t.Errorf("the table has no TRACK and LEVEL columns:\n%s", all.stdout)
			}

			byTrack := run("--track", "cicd")
			if byTrack.err != nil || !listed(byTrack, tc.tracked) || listed(byTrack, tc.fixture) {
				t.Errorf("--track cicd should keep only %s, got err=%v\n%s", tc.tracked, byTrack.err, byTrack.stdout)
			}

			// An item that sets nothing is a kubernetes, intermediate item.
			byDefault := run("--track", "kubernetes", "--level", "intermediate")
			if byDefault.err != nil || listed(byDefault, tc.tracked) || !listed(byDefault, tc.fixture) {
				t.Errorf("the defaults should match %s only, got err=%v\n%s", tc.fixture, byDefault.err, byDefault.stdout)
			}

			byLevel := run("--level", levels[tc.name])
			if byLevel.err != nil || !listed(byLevel, tc.tracked) || listed(byLevel, tc.fixture) {
				t.Errorf("--level %s should keep only %s, got err=%v\n%s", levels[tc.name], tc.tracked, byLevel.err, byLevel.stdout)
			}

			byBoth := run("--track", "cicd", "--level", levels[tc.name])
			if byBoth.err != nil || !listed(byBoth, tc.tracked) {
				t.Errorf("--track cicd --level %s should keep %s, got err=%v\n%s", levels[tc.name], tc.tracked, byBoth.err, byBoth.stdout)
			}

			none := run("--track", "iac")
			if none.err != nil || !strings.Contains(none.stdout, "on the IaC track") || listed(none, tc.tracked) {
				t.Errorf("a filter that matches nothing should say so, got err=%v\n%s", none.err, none.stdout)
			}

			for _, bad := range [][]string{{"--track", "kube"}, {"--level", "easy"}} {
				r := run(bad...)
				if r.err == nil || !strings.Contains(r.err.Error(), "use ") {
					t.Errorf("labctl %s %v should name the valid values, got err=%v", strings.Join(tc.list, " "), bad, r.err)
				}
			}
		})
	}
}

// The JSON for scenarios and incidents carries track, level and minutes, and
// the filter applies to it too.
// The info commands show the track, level and time in words a learner reads,
// and --track k8s is the kubernetes track.
func TestCatalogInfo_ShowsMeta(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"scenario", []string{"scenario", "info", "pipeline-cache"}, []string{"Track:       CI/CD", "Level:       Beginner", "Time:        ~20 min"}},
		{"incident", []string{"incident", "info", "stale-cache"}, []string{"Track:       CI/CD", "Level:       Advanced", "Time:        ~25 min"}},
		{"challenge", []string{"challenge", "info", "cache-bust"}, []string{"Track:       CI/CD", "Level:       Beginner", "Time:        ~12 min"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTerminal(t)
			writeTrackedContent(t, l)
			r := l.run(tt.args...)
			if r.err != nil {
				t.Fatalf("%v: %v", tt.args, r.err)
			}
			for _, w := range tt.want {
				if !strings.Contains(r.stdout, w) {
					t.Errorf("info output lacks %q:\n%s", w, r.stdout)
				}
			}
		})
	}
}

func TestCatalogFilters_K8sAlias(t *testing.T) {
	l := newTerminal(t)
	writeTrackedContent(t, l)
	r := l.run("scenario", "list", "--track", "k8s")
	if r.err != nil || !strings.Contains(r.stdout, labtest.Scenario+" ") || strings.Contains(r.stdout, "pipeline-cache ") {
		t.Errorf("--track k8s should list the kubernetes items only, err=%v\n%s", r.err, r.stdout)
	}
}

func TestNoMatch(t *testing.T) {
	tests := []struct {
		f    schema.Filter
		want string
	}{
		{schema.Filter{Track: "cicd", Level: "advanced"}, "No scenarios on the CI/CD track at Advanced level yet. Drop a filter to see more."},
		{schema.Filter{Track: "iac"}, "No scenarios on the IaC track yet. Drop a filter to see more."},
		{schema.Filter{Level: "beginner"}, "No scenarios at Beginner level yet. Drop a filter to see more."},
	}
	for _, tt := range tests {
		if got := noMatch("scenarios", tt.f); got != tt.want {
			t.Errorf("noMatch(%+v) = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestPrintMeta_OmitsUnstatedTime(t *testing.T) {
	var b strings.Builder
	printMeta(&b, schema.Meta{})
	if got, want := b.String(), "Track:       Kubernetes\nLevel:       Intermediate\n"; got != want {
		t.Errorf("printMeta = %q, want %q", got, want)
	}
}

func TestCatalogFilters_CLIJSON(t *testing.T) {
	for _, tc := range []struct {
		list    []string
		tracked string
	}{
		{[]string{"scenario", "list"}, "pipeline-cache"},
		{[]string{"incident", "list"}, "stale-cache"},
	} {
		t.Run(tc.list[0], func(t *testing.T) {
			l := newTerminal(t)
			writeTrackedContent(t, l)
			r := l.run(append(tc.list, "-o", "json", "--track", "cicd")...)
			if r.err != nil {
				t.Fatalf("list failed: %v\n%s", r.err, r.stderr)
			}
			var items []struct {
				Name    string `json:"name"`
				Track   string `json:"track"`
				Level   string `json:"level"`
				Minutes int    `json:"minutes"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &items); err != nil {
				t.Fatalf("the output is not JSON: %v\n%s", err, r.stdout)
			}
			if len(items) != 1 || items[0].Name != tc.tracked || items[0].Track != "cicd" || items[0].Minutes == 0 || items[0].Level == "" {
				t.Errorf("want only %s with track, level and minutes, got %+v", tc.tracked, items)
			}
		})
	}
}
