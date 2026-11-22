// SPDX-License-Identifier: Apache-2.0

package target

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

func TestOutputs_Env(t *testing.T) {
	tests := []struct {
		name string
		out  Outputs
		env  map[string]string
		kube map[string]string
	}{
		{
			name: "k3d",
			out:  Outputs{Kubeconfig: "/state/kubeconfig", KubeContext: "k3d-lab", HTTPPort: "8080", HTTPSPort: "8443", DomainSuffix: "lab.localhost"},
			env: map[string]string{
				"KUBECONFIG": "/state/kubeconfig", "SNOWOPS_LAB_KUBECONFIG": "/state/kubeconfig",
				"HTTP_PORT": "8080", "HTTPS_PORT": "8443", "DOMAIN_SUFFIX": "lab.localhost",
			},
			kube: map[string]string{"KUBECONFIG": "/state/kubeconfig", "SNOWOPS_LAB_KUBECONFIG": "/state/kubeconfig"},
		},
		{name: "incluster has no kubeconfig", out: Outputs{DomainSuffix: "lab.local"}, env: map[string]string{"DOMAIN_SUFFIX": "lab.local"}, kube: map[string]string{}},
		{name: "nothing recorded", out: Outputs{}, env: map[string]string{}, kube: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.out.Env(); !maps.Equal(got, tt.env) {
				t.Errorf("Env() = %v, want %v", got, tt.env)
			}
			if got := tt.out.KubeEnv(); !maps.Equal(got, tt.kube) {
				t.Errorf("KubeEnv() = %v, want %v", got, tt.kube)
			}
		})
	}
}

func TestLab_Require(t *testing.T) {
	tests := []struct {
		name      string
		names     []string
		fake      func() *toolchain.Fake
		setup     func(t *testing.T, lab *Lab)
		wantErr   []string
		wantCalls int
	}{
		{name: "the cluster is ready", names: []string{"k8s"}, fake: toolchain.NewFake, wantCalls: 1},
		{name: "no names means the cluster", names: nil, fake: toolchain.NewFake, wantCalls: 1},
		{name: "a name listed twice is checked once", names: []string{"k8s", "k8s"}, fake: toolchain.NewFake, wantCalls: 1},
		{
			name:  "a cluster that is down says why and what to run",
			names: []string{"k8s"},
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContainStderr("status.sh", "", "the API server is not answering\n", 1)
			},
			wantErr:   []string{"target k8s is not ready: the API server is not answering", "labctl init"},
			wantCalls: 1,
		},
		{
			name:    "an unknown target is rejected and the known ones named",
			names:   []string{"k8s", "git-forge"},
			fake:    toolchain.NewFake,
			wantErr: []string{`unknown target "git-forge"`, "k8s"},
			// k8s is checked before the unknown name is reached.
			wantCalls: 1,
		},
		{
			name:    "a lab with no provider for the target says which could supply it",
			names:   []string{"k8s"},
			fake:    toolchain.NewFake,
			setup:   func(_ *testing.T, lab *Lab) { lab.Providers = nil },
			wantErr: []string{"target k8s has no provider in this lab", "k3d, kind, incluster"},
		},
		{
			name:  "a provider with no status.sh is not blocked",
			names: []string{"k8s"},
			fake:  toolchain.NewFake,
			setup: func(t *testing.T, lab *Lab) {
				if err := os.Remove(filepath.Join(lab.Root, "runtimes", "k3d", StatusScript)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "a status check that cannot run is reported",
			names: []string{"k8s"},
			fake: func() *toolchain.Fake {
				f := toolchain.NewFake()
				f.LookPathErr = os.ErrNotExist
				return f
			},
			wantErr: []string{"checking target k8s", "bash"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := tt.fake()
			lab := newStatusLab(t, fake)
			lab.Providers = map[string]string{DefaultName: ProviderK3d}
			if tt.setup != nil {
				tt.setup(t, &lab)
			}
			err := lab.Require(t.Context(), tt.names)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("Require() = %v, want ready", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Require() = nil, want an error with %q", tt.wantErr)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not contain %q", err, w)
					}
				}
			}
			if got := len(fake.Calls()); got != tt.wantCalls {
				t.Errorf("status.sh ran %d times, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestLab_Require_AProviderThatTimesOutSaysSo(t *testing.T) {
	fake := toolchain.NewFake().WhenArgsContainError("status.sh", context.DeadlineExceeded)
	lab := newStatusLab(t, fake)
	lab.Providers = map[string]string{DefaultName: ProviderK3d}
	err := lab.Require(t.Context(), []string{DefaultName})
	if err == nil {
		t.Fatal("Require succeeded")
	}
	for _, w := range []string{"target k8s is not ready", "did not answer within 20s", "Docker"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error %q does not contain %q", err, w)
		}
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("error %q shows a raw context error", err)
	}
}

func TestUpAdvice(t *testing.T) {
	if got := upAdvice(DefaultName, ProviderK3d); !strings.Contains(got, "labctl init") {
		t.Errorf("k8s advice = %q, want labctl init", got)
	}
	got := upAdvice(DefaultName, ProviderIncluster)
	if strings.Contains(got, "labctl init") || !strings.Contains(got, "ServiceAccount") || !strings.Contains(got, "API server") {
		t.Errorf("incluster advice = %q, want the API server and ServiceAccount, not labctl init", got)
	}
	if got := upAdvice("git-forge", "gitea"); !strings.Contains(got, "runtimes/gitea/up.sh") {
		t.Errorf("other advice = %q, want the provider's up.sh", got)
	}
}
