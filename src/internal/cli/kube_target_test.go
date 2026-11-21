// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/snowops-oss/snowopslabs/internal/config"
	"github.com/snowops-oss/snowopslabs/internal/k8s"
	"github.com/snowops-oss/snowopslabs/internal/labcheck"
	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/internal/toolchain"
	"github.com/snowops-oss/snowopslabs/internal/workload"
)

// colleagueKubectl is a kubectl whose user has another cluster selected. It
// can cut the lab's context out of the user's kubeconfig, and it reports the
// KUBECONFIG it was started with for anything else.
const colleagueKubectl = `#!/bin/sh
case "$*" in
  "config view --minify --flatten --context k3d-clitest") echo "apiVersion: v1" ;;
  "config current-context") echo colleague-staging ;;
  *) echo "kubectl saw KUBECONFIG=$KUBECONFIG" ;;
esac
`

// Whatever kubectl context the user has selected, every way labctl runs
// something reaches the lab's own kubeconfig: scripts through the run engine,
// the web UI's executor, check scripts, and Go's own kubectl calls.
func TestLabKubeconfig_ReachesEveryRunPath(t *testing.T) {
	l := newTerminal(t)
	t.Setenv("KUBECONFIG", "/home/me/.kube/config")
	os.Unsetenv(target.UserKubeconfigEnv)
	l.Stub(t, "kubectl", colleagueKubectl)
	writeDrill(t, l)
	l.Write(t, "scenarios/drill/setup.sh", "#!/usr/bin/env bash\nprintf '%s' \"$KUBECONFIG\" >\"$(dirname \"$0\")/SEEN\"\n")

	l.mustRun("scenario", "up", "drill")

	labFile := filepath.Join(l.Home, "state", "clitest", "kubeconfig")
	t.Run("the run engine's scripts", func(t *testing.T) {
		seen, err := os.ReadFile(filepath.Join(l.Root, "scenarios/drill/SEEN"))
		if err != nil {
			t.Fatalf("the setup script did not run: %v", err)
		}
		if string(seen) != labFile {
			t.Errorf("setup script saw KUBECONFIG=%q, want the lab's %q", seen, labFile)
		}
	})
	t.Run("the lab file is created from the user's context, private", func(t *testing.T) {
		data, err := os.ReadFile(labFile)
		if err != nil || !strings.Contains(string(data), "apiVersion: v1") {
			t.Fatalf("lab kubeconfig = %q, %v", data, err)
		}
		if info, _ := os.Stat(labFile); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	})
	t.Run("the web UI's executor", func(t *testing.T) {
		if got := scriptExec.GetEnv("KUBECONFIG"); got != labFile {
			t.Errorf("executor KUBECONFIG = %q, want %q", got, labFile)
		}
	})
	t.Run("the env the run engine's services pass", func(t *testing.T) {
		if got := scriptEnv()["KUBECONFIG"]; got != labFile {
			t.Errorf("scriptEnv KUBECONFIG = %q, want %q", got, labFile)
		}
		if got := scriptEnv()["SNOWOPS_LAB_KUBECONFIG"]; got != labFile {
			t.Errorf("scriptEnv SNOWOPS_LAB_KUBECONFIG = %q, want %q", got, labFile)
		}
	})
	t.Run("check scripts", func(t *testing.T) {
		if env := strings.Join(labcheck.ScriptEnv(cfg, workload.Workload{}), "\n"); !strings.Contains(env, "KUBECONFIG="+labFile) {
			t.Errorf("check env does not carry the lab kubeconfig:\n%s", env)
		}
	})
	t.Run("Go's own kubectl", func(t *testing.T) {
		out, err := k8s.RunKubectl(t.Context(), "get", "pods")
		if err != nil || out != "kubectl saw KUBECONFIG="+labFile {
			t.Errorf("RunKubectl = %q, %v; want kubectl to read the lab's kubeconfig", out, err)
		}
	})
	t.Run("the user's kubeconfig is kept for the runtime scripts", func(t *testing.T) {
		if got := os.Getenv(target.UserKubeconfigEnv); got != "/home/me/.kube/config" {
			t.Errorf("%s = %q, want the user's own kubeconfig", target.UserKubeconfigEnv, got)
		}
	})
}

// A lab with no context to cut a kubeconfig from still never falls back to the
// user's current context, and a command that needs the cluster says what to run.
func TestLabKubeconfig_MissingContext(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantWarning bool
	}{
		{"a command that needs the cluster", []string{"scenario", "list"}, true},
		{"status has its own message", []string{"status"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTerminal(t)
			os.Unsetenv(target.UserKubeconfigEnv)
			l.Stub(t, "kubectl", "#!/bin/sh\nexit 1\n")

			r := l.run(tt.args...)

			warned := strings.Contains(r.stderr, `kubectl has no context "k3d-clitest"`) && strings.Contains(r.stderr, "labctl init")
			if warned != tt.wantWarning {
				t.Errorf("warned = %v, want %v\nstderr:\n%s", warned, tt.wantWarning, r.stderr)
			}
			want := filepath.Join(l.Home, "state", "clitest", "kubeconfig")
			if got := os.Getenv("KUBECONFIG"); got != want {
				t.Errorf("KUBECONFIG = %q, want the lab's %q even though it cannot be created", got, want)
			}
		})
	}
}

// The in-cluster profile has no kubeconfig; KUBECONFIG stays as it was.
func TestLabKubeconfig_InclusterIsLeftAlone(t *testing.T) {
	l := newTerminal(t)
	l.Write(t, ".env", "APP_NAME=demo-app\nPROFILE=incluster\n")
	l.Write(t, "runtimes/incluster/runtime.env", "REGISTRY_TYPE=none\n")

	l.mustRun("scenario", "list")

	if got := os.Getenv("KUBECONFIG"); got != "" {
		t.Errorf("KUBECONFIG = %q, want it untouched for incluster", got)
	}
	if _, set := scriptExec.Env["KUBECONFIG"]; set {
		t.Error("the executor carries a KUBECONFIG for incluster")
	}
	if _, set := scriptEnv()["KUBECONFIG"]; set {
		t.Error("scriptEnv carries a KUBECONFIG for incluster")
	}
}

func TestUseLabCluster(t *testing.T) {
	const kubeContext = "k3d-lab"
	view := "config view --minify --flatten --context " + kubeContext
	newLab := func(t *testing.T) *config.Config {
		t.Helper()
		return &config.Config{
			Profile:     "k3d",
			ClusterName: "lab",
			KubeContext: kubeContext,
			Kubeconfig:  filepath.Join(t.TempDir(), "kubeconfig"),
		}
	}
	reset := func(t *testing.T, kubeconfig string) {
		t.Helper()
		t.Setenv("KUBECONFIG", kubeconfig)
		t.Setenv(target.UserKubeconfigEnv, "")
		os.Unsetenv(target.UserKubeconfigEnv)
	}

	t.Run("the user's kubeconfig is recorded before KUBECONFIG moves to the lab's", func(t *testing.T) {
		reset(t, "/home/me/.kube/config")
		lab := newLab(t)
		fake := toolchain.NewFake().WhenArgsContain(view, "apiVersion: v1\n", 0)
		if err := useLabCluster(t.Context(), &cobra.Command{Use: "status"}, fake, lab); err != nil {
			t.Fatalf("useLabCluster: %v", err)
		}
		if got := os.Getenv("KUBECONFIG"); got != lab.Kubeconfig {
			t.Errorf("KUBECONFIG = %q, want %q", got, lab.Kubeconfig)
		}
		if got := os.Getenv(target.UserKubeconfigEnv); got != "/home/me/.kube/config" {
			t.Errorf("%s = %q, want the user's kubeconfig", target.UserKubeconfigEnv, got)
		}
		if got := fake.Calls()[0].Env["KUBECONFIG"]; got != "/home/me/.kube/config" {
			t.Errorf("kubectl cut the context from %q, want the user's kubeconfig", got)
		}
	})

	t.Run("a second command in the same process keeps the first one's record", func(t *testing.T) {
		reset(t, "/home/me/.kube/config")
		lab := newLab(t)
		fake := toolchain.NewFake().WhenArgsContain(view, "apiVersion: v1\n", 0)
		for range 2 {
			if err := useLabCluster(t.Context(), &cobra.Command{Use: "status"}, fake, lab); err != nil {
				t.Fatalf("useLabCluster: %v", err)
			}
		}
		if got := os.Getenv(target.UserKubeconfigEnv); got != "/home/me/.kube/config" {
			t.Errorf("%s = %q after two runs, want the user's kubeconfig", target.UserKubeconfigEnv, got)
		}
	})

	t.Run("a KUBECONFIG that already is the lab's is not read as the user's", func(t *testing.T) {
		lab := newLab(t)
		reset(t, lab.Kubeconfig)
		fake := toolchain.NewFake().WhenArgsContain(view, "apiVersion: v1\n", 0)
		if err := useLabCluster(t.Context(), &cobra.Command{Use: "status"}, fake, lab); err != nil {
			t.Fatalf("useLabCluster: %v", err)
		}
		if env := fake.Calls()[0].Env; len(env) != 0 {
			t.Errorf("kubectl got env %v, want it to use its default kubeconfig", env)
		}
	})

	t.Run("a lab that has its kubeconfig runs no kubectl", func(t *testing.T) {
		reset(t, "")
		lab := newLab(t)
		if err := os.WriteFile(lab.Kubeconfig, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		fake := toolchain.NewFake()
		if err := useLabCluster(t.Context(), &cobra.Command{Use: "scenario"}, fake, lab); err != nil {
			t.Fatalf("useLabCluster: %v", err)
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("kubectl ran: %v", fake.CallStrings())
		}
	})

	t.Run("incluster does nothing", func(t *testing.T) {
		reset(t, "/home/me/.kube/config")
		fake := toolchain.NewFake()
		if err := useLabCluster(t.Context(), &cobra.Command{Use: "scenario"}, fake, &config.Config{Profile: "incluster"}); err != nil {
			t.Fatalf("useLabCluster: %v", err)
		}
		if got := os.Getenv("KUBECONFIG"); got != "/home/me/.kube/config" {
			t.Errorf("KUBECONFIG = %q, want it untouched", got)
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("kubectl ran: %v", fake.CallStrings())
		}
	})

	warnings := []struct {
		name string
		cmd  func() *cobra.Command
		want bool
	}{
		{"a command that needs the cluster is warned", func() *cobra.Command { return &cobra.Command{Use: "incident"} }, true},
		{"init is not", func() *cobra.Command { return &cobra.Command{Use: "init"} }, false},
		{"reset is not", func() *cobra.Command { return &cobra.Command{Use: "reset"} }, false},
		{"teardown is not", func() *cobra.Command { return &cobra.Command{Use: "teardown"} }, false},
		{"setup-tools is not", func() *cobra.Command { return &cobra.Command{Use: "setup-tools"} }, false},
		{"status is not", func() *cobra.Command { return &cobra.Command{Use: "status"} }, false},
		{"lab up is not", func() *cobra.Command { return childOf("lab", "up") }, false},
		{"runtime down is not", func() *cobra.Command { return childOf("runtime", "down") }, false},
		{"up under another group is", func() *cobra.Command { return childOf("scenario", "up") }, true},
		{"a bare up is", func() *cobra.Command { return &cobra.Command{Use: "up"} }, true},
	}
	for _, tt := range warnings {
		t.Run(tt.name, func(t *testing.T) {
			reset(t, "")
			var stderr bytes.Buffer
			cmd := tt.cmd()
			cmd.SetErr(&stderr)
			fake := toolchain.NewFake().WhenArgsContain(view, "", 1)
			if err := useLabCluster(t.Context(), cmd, fake, newLab(t)); err != nil {
				t.Fatalf("useLabCluster: %v", err)
			}
			if got := strings.Contains(stderr.String(), `no context "k3d-lab"`); got != tt.want {
				t.Errorf("warned = %v, want %v (stderr %q)", got, tt.want, stderr.String())
			}
		})
	}
}

func childOf(parent, name string) *cobra.Command {
	p := &cobra.Command{Use: parent}
	c := &cobra.Command{Use: name}
	p.AddCommand(c)
	return c
}

func TestCurrentContextNote(t *testing.T) {
	lab := &config.Config{Profile: "k3d", KubeContext: "k3d-lab"}
	tests := []struct {
		name    string
		lab     *config.Config
		kubectl func(*toolchain.Fake) *toolchain.Fake
		want    []string
	}{
		{
			name: "another cluster is selected",
			lab:  lab,
			kubectl: func(f *toolchain.Fake) *toolchain.Fake {
				return f.WhenArgsContain("current-context", "colleague-staging\n", 0)
			},
			want: []string{`"colleague-staging"`, `"k3d-lab"`, "labctl runs only against the lab", "kubectl config use-context k3d-lab"},
		},
		{
			name:    "the lab's own context is selected",
			lab:     lab,
			kubectl: func(f *toolchain.Fake) *toolchain.Fake { return f.WhenArgsContain("current-context", "k3d-lab\n", 0) },
		},
		{
			name:    "no context is selected",
			lab:     lab,
			kubectl: func(f *toolchain.Fake) *toolchain.Fake { return f.WhenArgsContain("current-context", "", 1) },
		},
		{
			name: "kubectl cannot start",
			lab:  lab,
			kubectl: func(f *toolchain.Fake) *toolchain.Fake {
				return f.WhenArgsContainError("current-context", errors.New("boom"))
			},
		},
		{
			name:    "a lab with no context of its own",
			lab:     &config.Config{Profile: "incluster"},
			kubectl: func(f *toolchain.Fake) *toolchain.Fake { return f.WhenArgsContain("current-context", "other\n", 0) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KUBECONFIG", "/home/me/.kube/config")
			os.Unsetenv(target.UserKubeconfigEnv)
			got := currentContextNote(t.Context(), tt.kubectl(toolchain.NewFake()), tt.lab)
			if len(tt.want) == 0 && got != "" {
				t.Errorf("note = %q, want none", got)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("note %q does not contain %q", got, w)
				}
			}
		})
	}
}

func TestPrintTarget(t *testing.T) {
	t.Setenv("KUBECONFIG", "/home/me/.kube/config")
	os.Unsetenv(target.UserKubeconfigEnv)
	lab := &config.Config{Profile: "kind", KubeContext: "kind-lab"}

	var out bytes.Buffer
	printTarget(t.Context(), &out, toolchain.NewFake().WhenArgsContain("current-context", "colleague-staging\n", 0), lab)
	for _, want := range []string{"Lab:      kind cluster, context kind-lab", "Note:", `"colleague-staging"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q does not contain %q", out.String(), want)
		}
	}

	out.Reset()
	printTarget(t.Context(), &out, toolchain.NewFake().WhenArgsContain("current-context", "kind-lab\n", 0), lab)
	if strings.Contains(out.String(), "Note:") || !strings.Contains(out.String(), "Lab:") {
		t.Errorf("output %q should name the lab and nothing else", out.String())
	}

	out.Reset()
	withFile := &config.Config{Profile: "k3d", KubeContext: "k3d-lab", Kubeconfig: "/tmp/lab/kubeconfig"}
	printTarget(t.Context(), &out, toolchain.NewFake().WhenArgsContain("current-context", "k3d-lab\n", 0), withFile)
	if !strings.Contains(out.String(), "Lab:      k3d cluster, kubeconfig /tmp/lab/kubeconfig") {
		t.Errorf("output %q should name the lab's kubeconfig file", out.String())
	}

	out.Reset()
	printTarget(t.Context(), &out, toolchain.NewFake(), &config.Config{Profile: "incluster"})
	if out.Len() != 0 {
		t.Errorf("a lab with no context printed %q", out.String())
	}
}
