// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The lab's kubeconfig lives in its state directory and reaches every child
// process through ScriptEnv; the context it is cut from follows the profile.
func TestLoad_LabKubeconfig(t *testing.T) {
	tests := []struct {
		name           string
		dotEnv         string
		wantContext    string
		wantKubeconfig bool
	}{
		{name: "k3d", dotEnv: "CLUSTER_NAME=lab1\n", wantContext: "k3d-lab1", wantKubeconfig: true},
		{name: "kind", dotEnv: "PROFILE=kind\nCLUSTER_NAME=lab2\n", wantContext: "kind-lab2", wantKubeconfig: true},
		{name: "an explicit context for another profile", dotEnv: "PROFILE=cloud\nKUBECONFIG_CONTEXT=my-ctx\n", wantContext: "my-ctx", wantKubeconfig: true},
		{name: "a profile with no context", dotEnv: "PROFILE=cloud\n", wantContext: "", wantKubeconfig: true},
		{name: "incluster keeps its ServiceAccount", dotEnv: "PROFILE=incluster\n", wantContext: "", wantKubeconfig: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			root := t.TempDir()
			for _, p := range []string{"k3d", "kind", "cloud", "incluster"} {
				if err := os.MkdirAll(filepath.Join(root, "runtimes", p), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(tt.dotEnv), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(root)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.KubeContext != tt.wantContext {
				t.Errorf("KubeContext = %q, want %q", cfg.KubeContext, tt.wantContext)
			}
			want := ""
			if tt.wantKubeconfig {
				want = filepath.Join(cfg.StateDir, "kubeconfig")
			}
			if cfg.Kubeconfig != want {
				t.Errorf("Kubeconfig = %q, want %q", cfg.Kubeconfig, want)
			}
			for _, key := range []string{"KUBECONFIG", "SNOWOPS_LAB_KUBECONFIG"} {
				if got := cfg.ScriptEnv[key]; got != want {
					t.Errorf("ScriptEnv[%s] = %q, want %q", key, got, want)
				}
			}
		})
	}
}

// The user's own KUBECONFIG never replaces the lab's in what children see.
func TestLoad_LabKubeconfigBeatsTheUsersKubeconfig(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("KUBECONFIG", "/home/me/.kube/other")
	root := makeLab(t)
	if err := os.MkdirAll(filepath.Join(root, "runtimes", "k3d"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.ScriptEnv["KUBECONFIG"], filepath.Join(cfg.StateDir, "kubeconfig"); got != want {
		t.Errorf("ScriptEnv[KUBECONFIG] = %q, want the lab's %q", got, want)
	}
}

// A run that uses the k8s target gets what the lab recorded about it, and no
// other target has outputs yet.
func TestConfig_TargetOutputs(t *testing.T) {
	cfg := &Config{
		Kubeconfig: "/state/lab/kubeconfig", KubeContext: "k3d-lab",
		HTTPPort: "8080", HTTPSPort: "8443", DomainSuffix: "lab.localhost",
	}
	for _, name := range []string{"", "k8s"} {
		out, err := cfg.TargetOutputs(name)
		if err != nil {
			t.Fatalf("TargetOutputs(%q): %v", name, err)
		}
		if out.Kubeconfig != "/state/lab/kubeconfig" || out.KubeContext != "k3d-lab" ||
			out.HTTPPort != "8080" || out.HTTPSPort != "8443" || out.DomainSuffix != "lab.localhost" {
			t.Errorf("TargetOutputs(%q) = %+v, want the lab's own values", name, out)
		}
	}
	if _, err := cfg.TargetOutputs("git-forge"); err == nil || !strings.Contains(err.Error(), `"git-forge"`) {
		t.Errorf("TargetOutputs(git-forge) error = %v, want it to name the target", err)
	}
}

// outputs.env, once the provider has written a valid one, is where a run's
// cluster access comes from; a lab with none, a broken one or one written by
// another runtime profile keeps the computed values.
func TestLoad_RunEnvComesFromTheOutputsFile(t *testing.T) {
	const profileK3d = "PROFILE=k3d\n"
	tests := []struct {
		name        string
		dotEnv      string
		outputs     string // empty: no file
		wantKubecfg string // relative to the state dir unless absolute
		wantContext string
	}{
		{name: "no outputs yet", dotEnv: "CLUSTER_NAME=lab1\n", wantKubecfg: "kubeconfig", wantContext: "k3d-lab1"},
		{
			name: "a valid file wins", dotEnv: "CLUSTER_NAME=lab1\n",
			outputs:     profileK3d + "KUBECONFIG=/elsewhere/lab.kubeconfig\nKUBE_CONTEXT=custom-ctx\nHTTP_PORT=8080\n",
			wantKubecfg: "/elsewhere/lab.kubeconfig", wantContext: "custom-ctx",
		},
		{
			name: "a file without a profile still counts", dotEnv: "CLUSTER_NAME=lab1\n",
			outputs:     "KUBECONFIG=/elsewhere/lab.kubeconfig\nKUBE_CONTEXT=custom-ctx\n",
			wantKubecfg: "/elsewhere/lab.kubeconfig", wantContext: "custom-ctx",
		},
		{
			name: "a file with no context is invalid", dotEnv: "CLUSTER_NAME=lab1\n",
			outputs:     profileK3d + "KUBECONFIG=/elsewhere/lab.kubeconfig\n",
			wantKubecfg: "kubeconfig", wantContext: "k3d-lab1",
		},
		{
			name: "a file that is not KEY=value is invalid", dotEnv: "CLUSTER_NAME=lab1\n",
			outputs:     "this is not an env file\n",
			wantKubecfg: "kubeconfig", wantContext: "k3d-lab1",
		},
		{
			name: "a file from another runtime profile is ignored", dotEnv: "CLUSTER_NAME=lab1\n",
			outputs:     "PROFILE=kind\nKUBECONFIG=/kind/kubeconfig\nKUBE_CONTEXT=kind-lab1\n",
			wantKubecfg: "kubeconfig", wantContext: "k3d-lab1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			root := t.TempDir()
			for _, p := range []string{"k3d", "kind"} {
				if err := os.MkdirAll(filepath.Join(root, "runtimes", p), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(tt.dotEnv), 0o644); err != nil {
				t.Fatal(err)
			}
			stateDir, err := StateDir("lab1")
			if err != nil {
				t.Fatal(err)
			}
			if tt.outputs != "" {
				if err := os.MkdirAll(stateDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stateDir, "outputs.env"), []byte(tt.outputs), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(root)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := tt.wantKubecfg
			if !filepath.IsAbs(want) {
				want = filepath.Join(stateDir, want)
			}
			if cfg.Kubeconfig != want || cfg.KubeContext != tt.wantContext {
				t.Errorf("Kubeconfig, KubeContext = %q, %q; want %q, %q", cfg.Kubeconfig, cfg.KubeContext, want, tt.wantContext)
			}
			for _, key := range []string{"KUBECONFIG", "SNOWOPS_LAB_KUBECONFIG"} {
				if got := cfg.ScriptEnv[key]; got != want {
					t.Errorf("ScriptEnv[%s] = %q, want %q", key, got, want)
				}
			}
			if got := cfg.K8sOutputs().KubeEnv()["KUBECONFIG"]; got != want {
				t.Errorf("K8sOutputs().KubeEnv()[KUBECONFIG] = %q, want %q", got, want)
			}
		})
	}
}
