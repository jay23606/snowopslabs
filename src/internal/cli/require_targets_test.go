// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/labtest"
)

// Content starts only when the lab's k8s target is ready. When it is not,
// the command says which target and what to run, and changes nothing.
func TestContentStartsOnlyWhenTargetsAreReady(t *testing.T) {
	const down = "#!/usr/bin/env bash\necho \"the API server of 'clitest' is not answering\" >&2\nexit 1\n"
	tests := []struct {
		name      string
		args      []string
		statusSh  string
		wantErr   []string
		wantSetup bool
	}{
		{name: "scenario up with the cluster ready", args: []string{"scenario", "up", "drill"}, statusSh: "#!/usr/bin/env bash\nexit 0\n", wantSetup: true},
		{
			name: "scenario up with the cluster down", args: []string{"scenario", "up", "drill"}, statusSh: down,
			wantErr: []string{"scenario drill cannot start", "target k8s is not ready: the API server of 'clitest' is not answering", "labctl init"},
		},
		{
			name: "incident inject with the cluster down", args: []string{"incident", "inject", labtest.Fault}, statusSh: down,
			wantErr: []string{"target k8s is not ready", "labctl init"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTerminal(t)
			writeDrill(t, l)
			l.Write(t, "runtimes/k3d/status.sh", tt.statusSh)
			r := l.run(tt.args...)
			if len(tt.wantErr) == 0 && r.err != nil {
				t.Fatalf("labctl %s failed: %v\n%s%s", strings.Join(tt.args, " "), r.err, r.stdout, r.stderr)
			}
			for _, w := range tt.wantErr {
				if r.err == nil || !strings.Contains(r.err.Error(), w) {
					t.Errorf("labctl %s error = %v, want it to contain %q", strings.Join(tt.args, " "), r.err, w)
				}
			}
			_, statErr := os.Stat(filepath.Join(l.Root, "scenarios/drill/BROKEN"))
			if got := statErr == nil; got != tt.wantSetup {
				t.Errorf("setup script ran = %v, want %v", got, tt.wantSetup)
			}
		})
	}
}

// A start checks the lab's targets once: the command's own preflight, and not
// the engine that starts the content after it, runs status.sh.
func TestContentStartRunsStatusOnce(t *testing.T) {
	const counting = "#!/usr/bin/env bash\necho run >>\"$LAB_STATE_DIR/status-runs\"\nexit 0\n"
	for _, args := range [][]string{
		{"scenario", "up", "drill"},
		{"incident", "inject", labtest.Fault},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			l := newTerminal(t)
			writeDrill(t, l)
			l.Write(t, "runtimes/k3d/status.sh", counting)
			if r := l.run(args...); r.err != nil {
				t.Fatalf("labctl %s failed: %v\n%s%s", strings.Join(args, " "), r.err, r.stdout, r.stderr)
			}
			runs, err := os.ReadFile(filepath.Join(l.Home, "state", "clitest", "status-runs"))
			if err != nil {
				t.Fatalf("status.sh did not run: %v", err)
			}
			if got := strings.Count(string(runs), "run"); got != 1 {
				t.Errorf("status.sh ran %d times, want once", got)
			}
		})
	}
}
