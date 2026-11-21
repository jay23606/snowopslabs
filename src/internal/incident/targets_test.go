// SPDX-License-Identifier: Apache-2.0

package incident

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/executor"
	"github.com/snowops-oss/snowopslabs/pkg/checks"
)

func TestFaultValidate_Targets(t *testing.T) {
	base := func() *Fault {
		return &Fault{
			Name: "x", Category: "network", Severity: "high",
			Target:    Target{Namespace: "go-api", Workload: "go-api"},
			Detection: checks.Check{Name: "d", Type: "http", URL: "http://x"},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*Fault)
		wantErr string
	}{
		{"nothing set is valid", func(*Fault) {}, ""},
		{"every part on the cluster", func(f *Fault) {
			f.Detection.Target, f.Inject.Target, f.Resolve.Target = "k8s", "k8s", "k8s"
		}, ""},
		{"detection on an unlisted target", func(f *Fault) { f.Detection.Target = "git-forge" }, `detection: target "git-forge" is not in requires`},
		{"inject on an unlisted target", func(f *Fault) { f.Inject.Target = "git-forge" }, `inject: target "git-forge" is not in requires`},
		{"resolve on an unlisted target", func(f *Fault) { f.Resolve.Target = "git-forge" }, `resolve: target "git-forge" is not in requires`},
		{"listed targets are accepted", func(f *Fault) {
			f.Requires = []string{"k8s", "git-forge"}
			f.Inject.Target = "git-forge"
		}, ""},
		{"duplicate requires", func(f *Fault) { f.Requires = []string{"k8s", "k8s"} }, `target "k8s" is listed twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := base()
			tt.mutate(f)
			err := f.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want valid", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// requires and inject/resolve load from fault.yaml, and a fault that sets
// none needs the cluster.
func TestEngine_FaultRequiresLoadFromYAML(t *testing.T) {
	root := t.TempDir()
	createApp(t, root, "go-api")
	for _, f := range []string{"plain", "forge"} {
		writeFault(t, root, f)
	}
	setMeta(t, root, "forge", "requires: [k8s, git-forge]\ninject:\n  target: git-forge\n")
	eng := NewEngine(root, "k3d.local")

	plain, err := eng.Get("plain")
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.RequiredTargets(); !slices.Equal(got, []string{"k8s"}) {
		t.Errorf("plain requires %v, want [k8s]", got)
	}
	forge, err := eng.Get("forge")
	if err != nil {
		t.Fatal(err)
	}
	if got := forge.RequiredTargets(); !slices.Equal(got, []string{"k8s", "git-forge"}) || forge.Inject.Target != "git-forge" {
		t.Errorf("forge requires %v, inject target %q, want what the file says", got, forge.Inject.Target)
	}
}

// Inject asks whether the fault's targets are ready before it runs anything.
func TestEngine_Inject_RequiresTargets(t *testing.T) {
	tests := []struct {
		name       string
		requires   string
		readyErr   error
		wantAsked  []string
		wantErr    string
		wantBroken bool
	}{
		{name: "default asks for the cluster", wantAsked: []string{"k8s"}, wantBroken: true},
		{name: "listed targets are all asked", requires: "requires: [k8s, git-forge]\n", wantAsked: []string{"k8s", "git-forge"}, wantBroken: true},
		{
			name: "a target that is not ready stops the inject", readyErr: errors.New("target k8s is not ready: no API server\nBring it up with:   labctl init"),
			wantAsked: []string{"k8s"}, wantErr: "incident fault-a cannot start: target k8s is not ready",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, root := testEngine(t, "fault-a")
			if tt.requires != "" {
				setMeta(t, root, "fault-a", tt.requires)
				e = NewEngine(root, "k3d.local")
			}
			var asked []string
			e.RequireTargets = func(_ context.Context, names []string) error {
				asked = names
				return tt.readyErr
			}
			_, err := e.Inject("fault-a", executor.New(root), false, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Inject error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Inject: %v", err)
			}
			if !slices.Equal(asked, tt.wantAsked) {
				t.Errorf("asked about %v, want %v", asked, tt.wantAsked)
			}
			_, statErr := os.Stat(filepath.Join(root, "incidents", "fault-a", "BROKEN"))
			if broken := statErr == nil; broken != tt.wantBroken {
				t.Errorf("inject.sh ran = %v, want %v", broken, tt.wantBroken)
			}
		})
	}
}
