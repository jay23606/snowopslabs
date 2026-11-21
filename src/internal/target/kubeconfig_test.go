// SPDX-License-Identifier: Apache-2.0

package target

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

const sampleKubeconfig = "apiVersion: v1\nkind: Config\ncurrent-context: k3d-lab\n"

func TestExpectedContext(t *testing.T) {
	tests := []struct {
		provider, cluster, override, want string
	}{
		{ProviderK3d, "lab", "", "k3d-lab"},
		{ProviderK3d, "lab", "ignored", "k3d-lab"},
		{ProviderKind, "snowops-kind", "", "kind-snowops-kind"},
		{ProviderIncluster, "lab", "", ""},
		{"cloud", "lab", "prod-eu", "prod-eu"},
		{"cloud", "lab", "", ""},
	}
	for _, tt := range tests {
		if got := ExpectedContext(tt.provider, tt.cluster, tt.override); got != tt.want {
			t.Errorf("ExpectedContext(%q, %q, %q) = %q, want %q", tt.provider, tt.cluster, tt.override, got, tt.want)
		}
	}
}

func TestEnsureKubeconfig(t *testing.T) {
	const kubeContext = "k3d-lab"
	view := "config view --minify --flatten --context " + kubeContext

	t.Run("a lab that has its kubeconfig keeps it and asks kubectl nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), KubeconfigFile)
		if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		fake := toolchain.NewFake()
		if err := EnsureKubeconfig(t.Context(), fake, path, kubeContext, ""); err != nil {
			t.Fatalf("EnsureKubeconfig: %v", err)
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("kubectl was called: %v", fake.CallStrings())
		}
		if got, _ := os.ReadFile(path); string(got) != "existing" {
			t.Errorf("the kubeconfig was rewritten: %q", got)
		}
	})

	t.Run("a lab from before the file existed gets it, private to the user", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state", "lab", KubeconfigFile)
		fake := toolchain.NewFake().WhenArgsContain(view, sampleKubeconfig, 0)
		if err := EnsureKubeconfig(t.Context(), fake, path, kubeContext, "/home/me/.kube/config"); err != nil {
			t.Fatalf("EnsureKubeconfig: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != sampleKubeconfig {
			t.Fatalf("kubeconfig = %q, %v; want the context kubectl printed", got, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("mode = %v, want 0600", mode)
		}
		calls := fake.Calls()
		if len(calls) != 1 || calls[0].Env["KUBECONFIG"] != "/home/me/.kube/config" {
			t.Errorf("kubectl ran as %+v, want one call that reads the user's kubeconfig", calls)
		}
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp-*"))
		if len(left) != 0 {
			t.Errorf("temporary files left behind: %v", left)
		}
	})

	t.Run("without a user kubeconfig kubectl reads its default one", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContain(view, sampleKubeconfig, 0)
		if err := EnsureKubeconfig(t.Context(), fake, filepath.Join(t.TempDir(), KubeconfigFile), kubeContext, ""); err != nil {
			t.Fatalf("EnsureKubeconfig: %v", err)
		}
		if env := fake.Calls()[0].Env; len(env) != 0 {
			t.Errorf("kubectl got env %v, want none", env)
		}
	})

	failures := []struct {
		name        string
		kubeContext string
		fake        func() *toolchain.Fake
		want        []string
	}{
		{
			name:        "a context kubectl does not know says what to run",
			kubeContext: kubeContext,
			fake:        func() *toolchain.Fake { return toolchain.NewFake().WhenArgsContain(view, "", 1) },
			want:        []string{`"k3d-lab"`, "labctl init"},
		},
		{
			name:        "kubectl that prints nothing is the same failure",
			kubeContext: kubeContext,
			fake:        toolchain.NewFake,
			want:        []string{`"k3d-lab"`, "labctl init"},
		},
		{
			name:        "a profile with no known context names the setting to add",
			kubeContext: "",
			fake:        toolchain.NewFake,
			want:        []string{"KUBECONFIG_CONTEXT", "labctl init"},
		},
		{
			name:        "a machine without kubectl is sent to doctor",
			kubeContext: kubeContext,
			fake: func() *toolchain.Fake {
				f := toolchain.NewFake()
				f.LookPathErr = errors.New("not found")
				return f
			},
			want: []string{"kubectl", "labctl doctor"},
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), KubeconfigFile)
			err := EnsureKubeconfig(t.Context(), tt.fake(), path, tt.kubeContext, "")
			if err == nil {
				t.Fatal("EnsureKubeconfig succeeded")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not mention %q", err, w)
				}
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Error("a kubeconfig was written for a failed lookup")
			}
		})
	}

	t.Run("a cancelled context stops before kubectl runs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		fake := toolchain.NewFake().WhenArgsContain(view, sampleKubeconfig, 0)
		path := filepath.Join(t.TempDir(), KubeconfigFile)
		if err := EnsureKubeconfig(ctx, fake, path, kubeContext, ""); err == nil {
			t.Fatal("EnsureKubeconfig succeeded with a cancelled context")
		}
		if _, err := os.Stat(path); err == nil {
			t.Error("a kubeconfig was written after cancellation")
		}
	})

	t.Run("a state directory that cannot be created is reported", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		fake := toolchain.NewFake().WhenArgsContain(view, sampleKubeconfig, 0)
		err := EnsureKubeconfig(t.Context(), fake, filepath.Join(blocker, "state", KubeconfigFile), kubeContext, "")
		if err == nil || !strings.Contains(err.Error(), "creating") {
			t.Errorf("error = %v, want one that says the directory could not be created", err)
		}
	})
}

func TestUserContext(t *testing.T) {
	t.Run("reads the user's kubeconfig when one is named", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContain("config current-context", "colleague-staging\n", 0)
		got, err := UserContext(t.Context(), fake, "/home/me/.kube/config")
		if err != nil || got != "colleague-staging" {
			t.Fatalf("UserContext = %q, %v; want colleague-staging", got, err)
		}
		if env := fake.Calls()[0].Env["KUBECONFIG"]; env != "/home/me/.kube/config" {
			t.Errorf("KUBECONFIG = %q, want the user's kubeconfig", env)
		}
	})

	t.Run("falls back to the default kubeconfig, not the lab's", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		fake := toolchain.NewFake().WhenArgsContain("config current-context", "kind-dev\n", 0)
		got, err := UserContext(t.Context(), fake, "")
		if err != nil || got != "kind-dev" {
			t.Fatalf("UserContext = %q, %v; want kind-dev", got, err)
		}
		if got, want := fake.Calls()[0].Env["KUBECONFIG"], filepath.Join(home, ".kube", "config"); got != want {
			t.Errorf("KUBECONFIG = %q, want the default kubeconfig %q", got, want)
		}
	})

	t.Run("no current context is not an error", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContain("config current-context", "", 1)
		got, err := UserContext(t.Context(), fake, "/x")
		if err != nil || got != "" {
			t.Errorf("UserContext = %q, %v; want empty and no error", got, err)
		}
	})

	t.Run("a kubectl that cannot start is an error", func(t *testing.T) {
		fake := toolchain.NewFake().WhenArgsContainError("config current-context", errors.New("boom"))
		if _, err := UserContext(t.Context(), fake, "/x"); err == nil {
			t.Error("UserContext succeeded though kubectl could not start")
		}
	})

	t.Run("a machine without kubectl is an error", func(t *testing.T) {
		fake := toolchain.NewFake()
		fake.LookPathErr = errors.New("not found")
		if _, err := UserContext(t.Context(), fake, "/x"); err == nil {
			t.Error("UserContext succeeded without kubectl")
		}
	})
}
