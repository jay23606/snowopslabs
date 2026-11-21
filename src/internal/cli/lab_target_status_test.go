// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `lab status --live` asks the runtime's own status.sh first, and reports the
// reason it gives when the cluster is not ready.
func TestLabStatusLive_UsesTheRuntimesStatusScript(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "status.sh says why the cluster is not ready",
			script: "echo \"the API server of 'clitest' is not answering\" >&2\nexit 1\n",
			want:   "Cluster:   unreachable (the API server of 'clitest' is not answering)",
		},
		{
			name:   "status.sh says the cluster is ready, so kubectl adds the details",
			script: "exit 0\n",
			want:   "Cluster:   reachable",
		},
		{
			name:   "a runtime with no status.sh falls back to asking kubectl",
			script: "",
			want:   "Cluster:   reachable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTerminal(t)
			if tt.script != "" {
				l.Write(t, "runtimes/k3d/status.sh", "#!/usr/bin/env bash\n"+tt.script)
			} else if err := os.Remove(filepath.Join(l.Root, "runtimes", "k3d", "status.sh")); err != nil {
				t.Fatal(err)
			}
			r := l.mustRun("lab", "status", "--live")
			if !strings.Contains(r.stdout, tt.want) {
				t.Errorf("lab status --live does not print %q:\n%s", tt.want, r.stdout)
			}
		})
	}
}
