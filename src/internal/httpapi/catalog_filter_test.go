// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/labtest"
)

// trackedLab is the fixture lab plus one beginner CI/CD item of each kind,
// next to the fixture's own items that set neither field.
func trackedLab(t *testing.T) *Server {
	t.Helper()
	lab := labtest.New(t)
	lab.Write(t, "scenarios/pipeline-cache/scenario.yaml", "apiVersion: scenario.snowops.net/v2\nname: pipeline-cache\ndisplayName: Pipeline cache\ncategory: delivery\ntrack: cicd\nlevel: beginner\nminutes: 20\ncomponents: []\n")
	lab.Write(t, "incidents/stale-cache/fault.yaml", `name: stale-cache
displayName: "Stale cache"
description: "{{.WorkloadName}} ships an old build"
category: workload
severity: low
track: cicd
level: beginner
minutes: 25
target:
  namespace: "{{.WorkloadNamespace}}"
  workload: "{{.WorkloadName}}"
detection:
  name: marker-gone
  type: script
  script: checks/resolved.sh
  timeoutSeconds: 5
`)
	for _, f := range []string{"inject.sh", "resolve.sh", "hints.md", "solution.md", "checks/resolved.sh"} {
		lab.Write(t, "incidents/stale-cache/"+f, "#!/bin/sh\nexit 0\n")
	}
	lab.Write(t, "challenges/cache-bust/challenge.yaml", "name: cache-bust\ndisplayName: Bust the cache\ndescription: Fix it\ncategory: workload\ntrack: cicd\nlevel: beginner\nminutes: 12\nparTime: 10m\nsetup:\n  type: incident\n  ref: stale-cache\ngrading:\n  useDetectionCheck: true\n")
	lab.Write(t, "learn/ci-basics/path.yaml", "name: ci-basics\ndisplayName: CI basics\ndescription: A pipeline path\ntrack: cicd\nlevel: beginner\nestimatedMinutes: 30\nmodules:\n  - name: step-one\n    action:\n      type: command\n      ref: labctl runtime up\n    check:\n      name: cluster-up\n      type: script\n      script: checks/cluster-up.sh\n")
	lab.Write(t, "learn/ci-basics/checks/cluster-up.sh", "#!/bin/sh\nexit 0\n")
	return contractServer(t, lab)
}

// itemNames lists the names a catalog route answers, in either envelope: a
// paginated {items} or the incident list's {faults}.
func itemNames(t *testing.T, body []byte) []string {
	t.Helper()
	var doc struct {
		Items  []struct{ Name string } `json:"items"`
		Faults []struct{ Name string } `json:"faults"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, body)
	}
	var names []string
	for _, it := range append(doc.Items, doc.Faults...) {
		names = append(names, it.Name)
	}
	return names
}

func has(names []string, want string) bool { return slices.Contains(names, want) }

// Each list route narrows by ?track and ?level before it paginates, answers an
// unknown value with a 400 that names the valid ones, and reports every item's
// track and level.
func TestCatalogFilters_API(t *testing.T) {
	routes := []struct {
		route, tracked, fixture string
	}{
		{"/scenarios", "pipeline-cache", labScenario},
		{"/incidents", "stale-cache", labFault},
		{"/challenges", "cache-bust", labChallenge},
		{"/learn/paths", "ci-basics", labPath},
	}
	for _, rt := range routes {
		t.Run(rt.route, func(t *testing.T) {
			s := trackedLab(t)
			get := func(query string) []string {
				t.Helper()
				w := do(s, http.MethodGet, rt.route+query, "", nil)
				if w.Code != http.StatusOK {
					t.Fatalf("GET %s%s = %d\n%s", rt.route, query, w.Code, w.Body)
				}
				return itemNames(t, w.Body.Bytes())
			}

			if all := get(""); !has(all, rt.tracked) || !has(all, rt.fixture) {
				t.Fatalf("an unfiltered list = %v, want both %s and %s", all, rt.tracked, rt.fixture)
			}
			if got := get("?track=cicd"); !has(got, rt.tracked) || has(got, rt.fixture) {
				t.Errorf("?track=cicd = %v, want only %s", got, rt.tracked)
			}
			if got := get("?track=kubernetes&level=intermediate"); has(got, rt.tracked) || !has(got, rt.fixture) {
				t.Errorf("the defaults = %v, want %s and not %s", got, rt.fixture, rt.tracked)
			}
			if got := get("?track=k8s&level=intermediate"); has(got, rt.tracked) || !has(got, rt.fixture) {
				t.Errorf("?track=k8s = %v, want the kubernetes item %s", got, rt.fixture)
			}
			if got := get("?track=cicd&level=advanced"); has(got, rt.tracked) {
				t.Errorf("?track=cicd&level=advanced = %v, must not include the beginner item", got)
			}
			if got := get("?track=iac"); len(got) != 0 {
				t.Errorf("?track=iac = %v, want an empty list", got)
			}

			for _, bad := range []string{"track=kube", "level=easy"} {
				w := do(s, http.MethodGet, rt.route+"?"+bad, "", nil)
				assertProblem(t, w, http.StatusBadRequest, "invalid_request", instanceOf(t, rt.route+"?"+bad))
				if !strings.Contains(w.Body.String(), "use ") {
					t.Errorf("?%s: the error does not name the valid values: %s", bad, w.Body)
				}
			}
		})
	}
}

// The filter applies before the page is cut, so a filtered list pages through
// only its own items.
func TestCatalogFilters_APIFilterBeforePagination(t *testing.T) {
	s := trackedLab(t)
	w := do(s, http.MethodGet, "/scenarios?"+url.Values{"track": {"kubernetes"}, "limit": {"1"}}.Encode(), "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", w.Code, w.Body)
	}
	var page struct {
		Items      []struct{ Name string } `json:"items"`
		NextCursor string                  `json:"nextCursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != labScenario {
		t.Errorf("first page = %+v, want the one kubernetes scenario", page.Items)
	}
	if page.NextCursor != "" {
		t.Errorf("nextCursor = %q, want none: pipeline-cache is not in the kubernetes track", page.NextCursor)
	}
}

// The list answers carry the three fields, and a path keeps estimatedMinutes
// beside minutes.
func TestCatalogFilters_APIShape(t *testing.T) {
	s := trackedLab(t)
	w := do(s, http.MethodGet, "/learn/paths?track=cicd", "", nil)
	var page struct {
		Items []struct {
			Track            string `json:"track"`
			Level            string `json:"level"`
			Minutes          int    `json:"minutes"`
			EstimatedMinutes int    `json:"estimatedMinutes"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("paths = %s, %v", w.Body, err)
	}
	if p := page.Items[0]; p.Track != "cicd" || p.Level != "beginner" || p.Minutes != 30 || p.EstimatedMinutes != 30 {
		t.Errorf("path = %+v, want cicd, beginner and 30 under both names", p)
	}
}
