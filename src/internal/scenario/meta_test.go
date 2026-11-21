// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"strings"
	"testing"
)

// A scenario that states its track, level and minutes lists with them, one
// that states nothing lists with the defaults, and one with a typo is not
// loaded and says which value to fix.
func TestEngine_CatalogMeta(t *testing.T) {
	root := t.TempDir()
	writeScenario(t, root, "tracked", `name: tracked
displayName: Tracked
category: delivery
track: cicd
level: beginner
minutes: 20
`)
	writeScenario(t, root, "plain", "name: plain\ndisplayName: Plain\ncategory: delivery\n")
	writeScenario(t, root, "typo", "name: typo\ndisplayName: Typo\ncategory: delivery\ntrack: k8s\n")
	eng := NewEngine(root, "k3d.local", "k3d")

	got := map[string]ScenarioStatus{}
	for _, s := range eng.Status() {
		got[s.Name] = s
	}
	if s := got["tracked"]; s.Track != "cicd" || s.Level != "beginner" || s.Minutes != 20 {
		t.Errorf("tracked lists as %+v, want cicd, beginner, 20", s.Meta)
	}
	if s := got["plain"]; s.Track != "kubernetes" || s.Level != "intermediate" || s.Minutes != 0 {
		t.Errorf("plain lists as %+v, want the defaults and no minutes", s.Meta)
	}
	if _, listed := got["typo"]; listed {
		t.Error("a scenario with an unknown track must not load")
	}
	err := eng.LoadErrors()["typo"]
	if err == nil || !strings.Contains(err.Error(), `track "k8s" is not a known track (use kubernetes, cicd or iac)`) {
		t.Errorf("load error = %v, want it to name the bad track and the valid ones", err)
	}
	for _, s := range eng.List() {
		if s.Track == "" || s.Level == "" {
			t.Errorf("List() returned %s without a track and level", s.Name)
		}
	}
}
