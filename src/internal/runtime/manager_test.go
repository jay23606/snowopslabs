// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/executor"
	"github.com/snowops-oss/snowopslabs/internal/labtest"
	"github.com/snowops-oss/snowopslabs/internal/target"
)

// listing is what the injected status and outputs functions answer for one
// runtime.
type listing struct {
	status  target.Status
	err     error
	outputs target.Outputs
	outErr  error
}

// newTestManager builds a Manager over a temp runtimes dir whose status.sh
// answers, outputs and user context are injected, so no script runs.
func newTestManager(t *testing.T, runtimesDir, clusterName, userCtx string, answers map[string]listing) *Manager {
	t.Helper()
	m := &Manager{
		ProjectRoot: filepath.Dir(runtimesDir), // parent of "runtimes"
		ClusterName: clusterName,
		status: func(_ context.Context, name string) (target.Status, error) {
			return answers[name].status, answers[name].err
		},
		outputs: func(name string) (target.Outputs, error) {
			return answers[name].outputs, answers[name].outErr
		},
		userContext: func(_ context.Context) (string, error) { return userCtx, nil },
	}
	m.scan()
	return m
}

// makeRuntime creates a minimal runtime directory with an up.sh.
func makeRuntime(t *testing.T, base, name string) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	// up.sh must exist for scan() to pick it up
	if err := os.WriteFile(filepath.Join(dir, "up.sh"), []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	ready := target.Status{Ready: true}
	tests := []struct {
		name        string
		runtime     string
		userContext string
		answer      listing
		wantActive  bool
		wantCurrent bool
	}{
		{
			name:        "ready, and its recorded context is the one kubectl has selected",
			runtime:     "k3d",
			userContext: "k3d-lab",
			answer:      listing{status: ready, outputs: target.Outputs{KubeContext: "k3d-lab"}},
			wantActive:  true,
			wantCurrent: true,
		},
		{
			name:        "ready, but kubectl has another context selected",
			runtime:     "k3d",
			userContext: "colleague-staging",
			answer:      listing{status: ready, outputs: target.Outputs{KubeContext: "k3d-lab"}},
			wantActive:  true,
		},
		{
			name:        "ready, and kubectl has no context selected",
			runtime:     "kind",
			userContext: "",
			answer:      listing{status: ready, outputs: target.Outputs{KubeContext: "kind-lab"}},
			wantActive:  true,
		},
		{
			name:        "a lab from before outputs existed is matched by the context it expects",
			runtime:     "k3d",
			userContext: "k3d-lab",
			answer:      listing{status: ready, outErr: target.ErrNoOutputs},
			wantActive:  true,
			wantCurrent: true,
		},
		{
			name:        "the recorded context wins over the expected one",
			runtime:     "k3d",
			userContext: "k3d-lab",
			answer:      listing{status: ready, outputs: target.Outputs{KubeContext: "renamed"}},
			wantActive:  true,
		},
		{
			name:        "a provider with no context is active and never current",
			runtime:     "incluster",
			userContext: "anything",
			answer:      listing{status: ready, outErr: target.ErrNoOutputs},
			wantActive:  true,
		},
		{
			name:        "not ready, even though kubectl has its context selected",
			runtime:     "k3d",
			userContext: "k3d-lab",
			answer:      listing{status: target.Status{Reason: "the API server is not answering"}},
		},
		{
			name:        "a status that cannot be read is not active",
			runtime:     "k3d",
			userContext: "k3d-lab",
			answer:      listing{err: errors.New("no status.sh")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rtDir := filepath.Join(t.TempDir(), "runtimes")
			makeRuntime(t, rtDir, tt.runtime)
			m := newTestManager(t, rtDir, "lab", tt.userContext, map[string]listing{tt.runtime: tt.answer})

			list := m.List()
			if len(list) != 1 || list[0].Name != tt.runtime {
				t.Fatalf("List() = %+v, want one entry for %s", list, tt.runtime)
			}
			if list[0].Active != tt.wantActive || list[0].Current != tt.wantCurrent {
				t.Errorf("List() = %+v, want active=%v current=%v", list[0], tt.wantActive, tt.wantCurrent)
			}
		})
	}
}

func TestList_MultipleRuntimes(t *testing.T) {
	rtDir := filepath.Join(t.TempDir(), "runtimes")
	makeRuntime(t, rtDir, "k3d")
	makeRuntime(t, rtDir, "kind")

	m := newTestManager(t, rtDir, "lab", "k3d-lab", map[string]listing{
		"k3d":  {status: target.Status{Ready: true}, outputs: target.Outputs{KubeContext: "k3d-lab"}},
		"kind": {status: target.Status{Reason: "kind has no cluster named 'lab'"}},
	})

	byName := map[string]RuntimeInfo{}
	for _, r := range m.List() {
		byName[r.Name] = r
	}
	if k3d := byName["k3d"]; !k3d.Active || !k3d.Current {
		t.Errorf("k3d: want active and current, got %+v", k3d)
	}
	if kind := byName["kind"]; kind.Active || kind.Current {
		t.Errorf("kind: want neither active nor current, got %+v", kind)
	}
}

// fixtureManager is a Manager over the fixture lab, with the executor that
// runs its scripts and the buffer their output goes to.
func fixtureManager(t *testing.T) (*Manager, *executor.Executor, *labtest.Lab, *bytes.Buffer) {
	t.Helper()
	l := labtest.New(t)
	l.Write(t, "runtimes/k3d/up.sh", "#!/usr/bin/env bash\necho \"up $1\"\n")
	l.Write(t, "runtimes/k3d/down.sh", "#!/usr/bin/env bash\necho \"down $1\"\n")
	ex := executor.New(l.Root)
	var out bytes.Buffer
	ex.Stdout, ex.Stderr = &out, &out
	return NewManager(l.Root, "lab"), ex, l, &out
}

func TestNewManager_FindsRuntimesWithAnUpScript(t *testing.T) {
	m, _, l, _ := fixtureManager(t)
	l.Write(t, "runtimes/_lib/docker.sh", "#!/usr/bin/env bash\n")
	l.Write(t, "runtimes/kind/up.sh", "#!/usr/bin/env bash\n")
	l.Write(t, "runtimes/README.md", "not a runtime\n")
	m = NewManager(m.ProjectRoot, "lab")
	if want := []string{"k3d", "kind"}; !slices.Equal(m.Names(), want) {
		t.Errorf("Names() = %v, want %v", m.Names(), want)
	}
	if got := NewManager(t.TempDir(), "lab").Names(); len(got) != 0 {
		t.Errorf("a project with no runtimes/ lists %v", got)
	}
}

// A runtime is as ready as its own status.sh says, run through the real
// scripts: the manager reads no kubectl context for it.
func TestList_RunsEachRuntimesStatusScript(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		kubectl     string
		wantActive  bool
		wantCurrent bool
	}{
		{"status.sh exits 0 and kubectl has the lab's context selected", "exit 0\n", "echo k3d-lab\n", true, true},
		{"status.sh exits 0 and kubectl has another context selected", "exit 0\n", "echo colleague-staging\n", true, false},
		{"status.sh exits 0 and kubectl has no context", "exit 0\n", "exit 1\n", true, false},
		{"status.sh exits non-zero, though kubectl has the lab's context selected", "echo 'not ready' >&2\nexit 1\n", "echo k3d-lab\n", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, l, _ := fixtureManager(t)
			l.Write(t, "runtimes/k3d/status.sh", "#!/usr/bin/env bash\n"+tt.status)
			l.Stub(t, "kubectl", "#!/bin/sh\n"+tt.kubectl)
			m = NewManager(m.ProjectRoot, "lab")
			got := m.List()
			if len(got) != 1 || got[0].Name != "k3d" || got[0].Current != tt.wantCurrent || got[0].Active != tt.wantActive {
				t.Errorf("List() = %+v, want k3d with current=%v active=%v", got, tt.wantCurrent, tt.wantActive)
			}
		})
	}
}

func TestList_AProviderWithoutStatusScriptIsNotActive(t *testing.T) {
	m, _, l, _ := fixtureManager(t)
	l.Stub(t, "kubectl", "#!/bin/sh\necho k3d-lab\n")
	if err := os.Remove(filepath.Join(l.Root, "runtimes", "k3d", "status.sh")); err != nil {
		t.Fatal(err)
	}
	m = NewManager(m.ProjectRoot, "lab")
	got := m.List()
	if len(got) != 1 || got[0].Active || got[0].Current {
		t.Errorf("List() = %+v, want k3d neither active nor current without a status.sh", got)
	}
}

// The status script gets the lab's own kubeconfig and state directory, so it
// answers for this lab whichever context the user has selected.
func TestList_StatusScriptSeesTheLabsKubeconfig(t *testing.T) {
	m, _, l, _ := fixtureManager(t)
	l.Write(t, "runtimes/k3d/status.sh", "#!/usr/bin/env bash\nprintf '%s|%s|%s' \"$1\" \"$KUBECONFIG\" \"$LAB_STATE_DIR\" >\"$LAB_STATE_DIR/seen\"\n")
	m = NewManager(m.ProjectRoot, "lab")
	stateDir := filepath.Join(l.Home, "state", "lab")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.List()
	seen, err := os.ReadFile(filepath.Join(stateDir, "seen"))
	if err != nil {
		t.Fatalf("status.sh did not run: %v", err)
	}
	want := "lab|" + filepath.Join(stateDir, "kubeconfig") + "|" + stateDir
	if string(seen) != want {
		t.Errorf("status.sh saw %q, want %q", seen, want)
	}
}

func TestActivateAndDeactivate(t *testing.T) {
	steps := []struct {
		name string
		run  func(m *Manager, ex *executor.Executor, runtime string) error
		want string
	}{
		{"Activate", func(m *Manager, ex *executor.Executor, rt string) error { return m.Activate(rt, ex) }, "up lab"},
		{"ActivateWith", func(m *Manager, ex *executor.Executor, rt string) error { return m.ActivateWith("job-1", rt, ex) }, "up lab"},
		{"Deactivate", func(m *Manager, ex *executor.Executor, rt string) error { return m.Deactivate(rt, ex) }, "down lab"},
		{"DeactivateWith", func(m *Manager, ex *executor.Executor, rt string) error { return m.DeactivateWith("job-2", rt, ex) }, "down lab"},
	}
	for _, step := range steps {
		t.Run(step.name+" runs the runtime's script with the cluster name", func(t *testing.T) {
			m, ex, _, out := fixtureManager(t)
			if err := step.run(m, ex, "k3d"); err != nil {
				t.Fatalf("%s() error = %v", step.name, err)
			}
			if !strings.Contains(out.String(), step.want) {
				t.Errorf("%s() output = %q, want it to contain %q", step.name, out.String(), step.want)
			}
		})
		t.Run(step.name+" refuses a runtime that does not exist", func(t *testing.T) {
			m, ex, _, out := fixtureManager(t)
			err := step.run(m, ex, "ghost")
			if err == nil || !strings.Contains(err.Error(), `runtime "ghost" not found`) {
				t.Errorf("%s(ghost) error = %v", step.name, err)
			}
			if out.Len() != 0 {
				t.Errorf("%s(ghost) ran a script: %q", step.name, out.String())
			}
		})
		t.Run(step.name+" reports a script that fails", func(t *testing.T) {
			m, ex, l, _ := fixtureManager(t)
			for _, s := range []string{"up.sh", "down.sh"} {
				l.Write(t, "runtimes/k3d/"+s, "#!/usr/bin/env bash\nexit 4\n")
			}
			if err := step.run(m, ex, "k3d"); err == nil {
				t.Errorf("%s() returned no error for a script that exited 4", step.name)
			}
		})
	}
}
