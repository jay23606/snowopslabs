// SPDX-License-Identifier: Apache-2.0

package target

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOutputs(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		content   string
		want      Outputs
		wantError []string
	}{
		{
			name:     "everything k3d records",
			provider: ProviderK3d,
			content: "KUBECONFIG=/state/lab/kubeconfig\nKUBE_CONTEXT=k3d-lab\n" +
				"HTTP_PORT=8080\nHTTPS_PORT=8443\nDOMAIN_SUFFIX=lab.localhost\n",
			want: Outputs{
				Kubeconfig: "/state/lab/kubeconfig", KubeContext: "k3d-lab",
				HTTPPort: "8080", HTTPSPort: "8443", DomainSuffix: "lab.localhost",
			},
		},
		{
			name:     "comments, blank lines and spaces are ignored",
			provider: ProviderKind,
			content:  "# written by up.sh\n\n  KUBECONFIG = /k  \nKUBE_CONTEXT=kind-lab\n",
			want:     Outputs{Kubeconfig: "/k", KubeContext: "kind-lab"},
		},
		{
			name:     "a key can appear twice and the last one wins",
			provider: ProviderK3d,
			content:  "KUBECONFIG=/old\nKUBECONFIG=/new\nKUBE_CONTEXT=k3d-lab\n",
			want:     Outputs{Kubeconfig: "/new", KubeContext: "k3d-lab"},
		},
		{
			name:     "ports and suffix are optional",
			provider: ProviderK3d,
			content:  "KUBECONFIG=/k\nKUBE_CONTEXT=k3d-lab\n",
			want:     Outputs{Kubeconfig: "/k", KubeContext: "k3d-lab"},
		},
		{
			name:     "incluster has no kubeconfig or context",
			provider: ProviderIncluster,
			content:  "DOMAIN_SUFFIX=cluster.local\n",
			want:     Outputs{DomainSuffix: "cluster.local"},
		},
		{
			name:      "a provider that makes a cluster must name its kubeconfig",
			provider:  ProviderK3d,
			content:   "KUBE_CONTEXT=k3d-lab\n",
			wantError: []string{"KUBECONFIG is missing", "labctl init"},
		},
		{
			name:      "and its context",
			provider:  ProviderKind,
			content:   "KUBECONFIG=/k\n",
			wantError: []string{"KUBE_CONTEXT is missing", "labctl init"},
		},
		{
			name:      "a value left empty counts as missing",
			provider:  ProviderK3d,
			content:   "KUBECONFIG=\nKUBE_CONTEXT=k3d-lab\n",
			wantError: []string{"KUBECONFIG is missing"},
		},
		{
			name:      "a port that is not a number",
			provider:  ProviderK3d,
			content:   "KUBECONFIG=/k\nKUBE_CONTEXT=c\nHTTP_PORT=http\n",
			wantError: []string{`HTTP_PORT is "http"`, "not a port number"},
		},
		{
			name:      "a port of zero",
			provider:  ProviderK3d,
			content:   "KUBECONFIG=/k\nKUBE_CONTEXT=c\nHTTPS_PORT=0\n",
			wantError: []string{`HTTPS_PORT is "0"`},
		},
		{
			name:      "a port above 65535",
			provider:  ProviderIncluster,
			content:   "HTTP_PORT=70000\n",
			wantError: []string{`HTTP_PORT is "70000"`},
		},
		{
			name:      "a line that is not KEY=value names its line number",
			provider:  ProviderK3d,
			content:   "KUBECONFIG=/k\nthis is not a setting\n",
			wantError: []string{"line 2", "not KEY=value", "labctl init"},
		},
		{
			name:      "a line with no key",
			provider:  ProviderK3d,
			content:   "=value\n",
			wantError: []string{"line 1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), OutputsFile)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadOutputs(path, tt.provider)
			if len(tt.wantError) > 0 {
				if err == nil {
					t.Fatalf("LoadOutputs = %+v, want an error", got)
				}
				for _, w := range tt.wantError {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not contain %q", err, w)
					}
				}
				if !strings.Contains(err.Error(), path) {
					t.Errorf("error %q does not name the file", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadOutputs: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadOutputs = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadOutputs_FileProblems(t *testing.T) {
	t.Run("a lab that was never brought up has no outputs", func(t *testing.T) {
		_, err := LoadOutputs(filepath.Join(t.TempDir(), OutputsFile), ProviderK3d)
		if !errors.Is(err, ErrNoOutputs) {
			t.Fatalf("error = %v, want ErrNoOutputs", err)
		}
		if !strings.Contains(err.Error(), "labctl init") {
			t.Errorf("error %q does not say what to run", err)
		}
	})
	t.Run("a path that cannot be read as a file is not reported as missing", func(t *testing.T) {
		_, err := LoadOutputs(t.TempDir(), ProviderK3d)
		if err == nil || errors.Is(err, ErrNoOutputs) {
			t.Errorf("error = %v, want a read error", err)
		}
	})
}

func TestLab_Outputs(t *testing.T) {
	dir := t.TempDir()
	lab := Lab{StateDir: dir}
	if got, want := lab.OutputsPath(), filepath.Join(dir, "outputs.env"); got != want {
		t.Errorf("OutputsPath = %q, want %q", got, want)
	}
	if got, want := lab.KubeconfigPath(), filepath.Join(dir, "kubeconfig"); got != want {
		t.Errorf("KubeconfigPath = %q, want %q", got, want)
	}
	if _, err := lab.Outputs(ProviderK3d); !errors.Is(err, ErrNoOutputs) {
		t.Errorf("Outputs before up = %v, want ErrNoOutputs", err)
	}
	if err := os.WriteFile(lab.OutputsPath(), []byte("KUBECONFIG=/k\nKUBE_CONTEXT=k3d-lab\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := lab.Outputs(ProviderK3d)
	if err != nil || got.KubeContext != "k3d-lab" {
		t.Errorf("Outputs = %+v, %v; want the recorded context", got, err)
	}
}
