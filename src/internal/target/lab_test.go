// SPDX-License-Identifier: Apache-2.0

package target

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// newStatusLab is a lab whose k3d, kind and incluster providers each have a
// status.sh, run through fake.
func newStatusLab(t *testing.T, fake *toolchain.Fake) Lab {
	t.Helper()
	root := t.TempDir()
	for _, p := range []string{ProviderK3d, ProviderKind, ProviderIncluster} {
		dir := filepath.Join(root, "runtimes", p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{UpScript, StatusScript} {
			if err := os.WriteFile(filepath.Join(dir, s), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return Lab{
		Root:     root,
		Cluster:  "lab",
		StateDir: filepath.Join(t.TempDir(), "state", "lab"),
		Runner:   fake,
		Registry: NewRegistry([]string{ProviderK3d, ProviderKind, ProviderIncluster}),
	}
}

func TestLab_Status(t *testing.T) {
	tests := []struct {
		name       string
		fake       func() *toolchain.Fake
		provider   string
		want       Status
		wantInArgs string
	}{
		{
			name:     "status.sh exits 0",
			fake:     toolchain.NewFake,
			provider: ProviderK3d,
			want:     Status{Ready: true},
		},
		{
			name: "status.sh exits non-zero with a reason",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContainStderr("status.sh", "", "the API server is not answering\n", 1)
			},
			provider: ProviderKind,
			want:     Status{Reason: "the API server is not answering"},
		},
		{
			name: "only the first line of the reason is kept",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContainStderr("status.sh", "", "\n  first line  \nsecond line\n", 1)
			},
			provider: ProviderK3d,
			want:     Status{Reason: "first line"},
		},
		{
			name: "a script that gives no reason is still explained",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContain("status.sh", "", 3)
			},
			provider: ProviderK3d,
			want:     Status{Reason: "status.sh exited 3 without a reason"},
		},
		{
			name: "a very long reason is cut",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContainStderr("status.sh", "", strings.Repeat("x", 500)+"\n", 1)
			},
			provider: ProviderK3d,
			want:     Status{Reason: strings.Repeat("x", maxReasonRunes) + "..."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lab := newStatusLab(t, tt.fake())
			got, err := lab.Status(t.Context(), DefaultName, tt.provider)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if got != tt.want {
				t.Errorf("Status = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLab_Status_RunsTheScriptWithTheLabsState(t *testing.T) {
	fake := toolchain.NewFake()
	lab := newStatusLab(t, fake)
	if _, err := lab.Status(t.Context(), "", ProviderK3d); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one", fake.CallStrings())
	}
	c := calls[0]
	if !strings.HasSuffix(c.Args[0], filepath.Join("runtimes", "k3d", "status.sh")) || c.Args[1] != "lab" {
		t.Errorf("ran %v, want runtimes/k3d/status.sh with the cluster name", c.Args)
	}
	if c.Dir != lab.Root {
		t.Errorf("Dir = %q, want the project root %q", c.Dir, lab.Root)
	}
	want := map[string]string{
		"CLUSTER_NAME":           "lab",
		"LAB_STATE_DIR":          lab.StateDir,
		"KUBECONFIG":             lab.KubeconfigPath(),
		"SNOWOPS_LAB_KUBECONFIG": lab.KubeconfigPath(),
	}
	for k, v := range want {
		if c.Env[k] != v {
			t.Errorf("env %s = %q, want %q", k, c.Env[k], v)
		}
	}
}

func TestLab_Status_InclusterHasNoKubeconfig(t *testing.T) {
	fake := toolchain.NewFake()
	lab := newStatusLab(t, fake)
	if _, err := lab.Status(t.Context(), DefaultName, ProviderIncluster); err != nil {
		t.Fatal(err)
	}
	env := fake.Calls()[0].Env
	for _, k := range []string{"KUBECONFIG", "SNOWOPS_LAB_KUBECONFIG"} {
		if _, set := env[k]; set {
			t.Errorf("incluster's status.sh was given %s", k)
		}
	}
	if env["CLUSTER_NAME"] != "lab" {
		t.Errorf("CLUSTER_NAME = %q, want lab", env["CLUSTER_NAME"])
	}
}

func TestLab_Status_Errors(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		provider string
		setup    func(t *testing.T, lab *Lab, fake *toolchain.Fake)
		want     []string
		wantIs   error
	}{
		{
			name:     "an unknown target is rejected and the known ones named",
			target:   "git-forge",
			provider: ProviderK3d,
			want:     []string{`unknown target "git-forge"`, "k8s"},
		},
		{
			name:     "a provider the target does not have is rejected and the real ones named",
			target:   DefaultName,
			provider: "aks",
			want:     []string{`"aks" is not a provider of target k8s`, "k3d", "kind", "incluster"},
		},
		{
			name:     "a provider with no status.sh says what to add",
			target:   DefaultName,
			provider: ProviderKind,
			setup: func(t *testing.T, lab *Lab, _ *toolchain.Fake) {
				if err := os.Remove(filepath.Join(lab.Root, "runtimes", "kind", StatusScript)); err != nil {
					t.Fatal(err)
				}
			},
			want:   []string{"runtimes/kind/status.sh", "exits 0 when the target is ready"},
			wantIs: ErrNoStatusScript,
		},
		{
			name:     "a machine without bash",
			target:   DefaultName,
			provider: ProviderK3d,
			setup: func(_ *testing.T, _ *Lab, fake *toolchain.Fake) {
				fake.LookPathErr = errors.New("not found")
			},
			want: []string{"bash"},
		},
		{
			name:     "a script that cannot start is not reported as not ready",
			target:   DefaultName,
			provider: ProviderK3d,
			setup: func(_ *testing.T, _ *Lab, fake *toolchain.Fake) {
				fake.WhenArgsContainError("status.sh", errors.New("exec format error"))
			},
			want: []string{"runtimes/k3d/status.sh", "exec format error"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := toolchain.NewFake()
			lab := newStatusLab(t, fake)
			if tt.setup != nil {
				tt.setup(t, &lab, fake)
			}
			got, err := lab.Status(t.Context(), tt.target, tt.provider)
			if err == nil {
				t.Fatalf("Status = %+v, want an error", got)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("error %v is not %v", err, tt.wantIs)
			}
			if len(fake.Calls()) != 0 && tt.name != "a script that cannot start is not reported as not ready" {
				t.Errorf("a script ran: %v", fake.CallStrings())
			}
		})
	}
}

func TestLab_Status_Context(t *testing.T) {
	t.Run("a cancelled context runs nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		fake := toolchain.NewFake()
		lab := newStatusLab(t, fake)
		if _, err := lab.Status(ctx, DefaultName, ProviderK3d); err == nil {
			t.Fatal("Status succeeded with a cancelled context")
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("a script ran: %v", fake.CallStrings())
		}
	})

	t.Run("a status check that outlives its own deadline is not ready, with a reason", func(t *testing.T) {
		old := statusTimeout
		statusTimeout = 20 * time.Millisecond
		t.Cleanup(func() { statusTimeout = old })
		fake := toolchain.NewFake().WhenArgsContainBlock("status.sh", time.Minute)
		lab := newStatusLab(t, fake)
		got, err := lab.Status(t.Context(), DefaultName, ProviderK3d)
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if got.Ready || !strings.Contains(got.Reason, "did not answer within 20ms") || !strings.Contains(got.Reason, "Docker") {
			t.Errorf("Status = %+v, want not ready with a reason that names the wait and Docker", got)
		}
	})

	t.Run("a deadline error from the runner is the same answer", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContainError("status.sh", context.DeadlineExceeded)
		lab := newStatusLab(t, fake)
		got, err := lab.Status(t.Context(), DefaultName, ProviderK3d)
		if err != nil || got.Ready || !strings.Contains(got.Reason, "did not answer within 20s") {
			t.Errorf("Status = %+v, %v; want not ready with the timeout reason", got, err)
		}
	})

	t.Run("a script that hangs is stopped by the deadline, not reported as not ready", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContainBlock("status.sh", time.Minute)
		lab := newStatusLab(t, fake)
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		got, err := lab.Status(ctx, DefaultName, ProviderK3d)
		if err == nil {
			t.Fatalf("Status = %+v, want a deadline error", got)
		}
	})
}
