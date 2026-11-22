// SPDX-License-Identifier: Apache-2.0

package incident

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setMeta adds lines to a fault's fault.yaml, after its severity.
func setMeta(t *testing.T, root, fault, lines string) {
	t.Helper()
	path := filepath.Join(root, "incidents", fault, "fault.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Replace(string(data), "severity: medium\n", "severity: medium\n"+lines, 1)
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEngine_FaultMeta(t *testing.T) {
	root := t.TempDir()
	createApp(t, root, "go-api")
	for _, f := range []string{"tracked", "plain", "typo"} {
		writeFault(t, root, f)
	}
	setMeta(t, root, "tracked", "track: iac\nlevel: advanced\nminutes: 25\n")
	setMeta(t, root, "typo", "level: expert\n")
	eng := NewEngine(root, "k3d.local")

	tracked, err := eng.Get("tracked")
	if err != nil {
		t.Fatal(err)
	}
	if tracked.Track != "iac" || tracked.Level != "advanced" || tracked.Minutes != 25 {
		t.Errorf("tracked = %+v, want iac, advanced, 25", tracked.Meta)
	}
	plain, _ := eng.Get("plain")
	if plain.Track != "kubernetes" || plain.Level != "intermediate" {
		t.Errorf("plain = %+v, want the defaults", plain.Meta)
	}
	if _, err := eng.Get("typo"); err == nil {
		t.Error("a fault with an unknown level must not load")
	}
	if err := eng.LoadErrors()["typo"]; err == nil || !strings.Contains(err.Error(), `level "expert" is not a known level`) {
		t.Errorf("load error = %v, want it to name the bad level", err)
	}
}
