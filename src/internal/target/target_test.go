// SPDX-License-Identifier: Apache-2.0

package target

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRegistry_Lookup(t *testing.T) {
	r := NewRegistry([]string{"k3d", "kind"})

	tests := []struct {
		name      string
		target    string
		wantName  string
		wantError []string
	}{
		{name: "the default target by name", target: "k8s", wantName: "k8s"},
		{name: "no name means the default target", target: "", wantName: "k8s"},
		{name: "an unknown target lists the ones the lab has", target: "git-forge", wantError: []string{`unknown target "git-forge"`, "k8s"}},
		{name: "names are exact", target: "K8S", wantError: []string{`unknown target "K8S"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Lookup(tt.target)
			if len(tt.wantError) > 0 {
				if err == nil {
					t.Fatalf("Lookup(%q) = %+v, want an error", tt.target, got)
				}
				for _, w := range tt.wantError {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q does not contain %q", err, w)
					}
				}
				return
			}
			if err != nil || got.Name != tt.wantName {
				t.Fatalf("Lookup(%q) = %+v, %v; want %s", tt.target, got, err, tt.wantName)
			}
		})
	}
}

func TestRegistry_ProvidersOfTheDefaultTarget(t *testing.T) {
	providers := []string{"k3d", "kind", "incluster"}
	r := NewRegistry(providers)
	k8s, err := r.Lookup(DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range providers {
		if !k8s.HasProvider(p) {
			t.Errorf("k8s does not list provider %s", p)
		}
	}
	if k8s.HasProvider("aks") {
		t.Error("k8s lists a provider that was not registered")
	}
	if got := r.Names(); !slices.Equal(got, []string{"k8s"}) {
		t.Errorf("Names() = %v, want only the default target", got)
	}

	// The registry keeps its own copy, so a caller changing its slice later
	// does not change what the lab supports.
	providers[0] = "changed"
	if k8s, _ = r.Lookup(DefaultName); !k8s.HasProvider("k3d") {
		t.Error("the registry shares the caller's slice")
	}
}

func TestDiscoverProviders(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("runtimes/kind/up.sh")
	write("runtimes/k3d/up.sh")
	write("runtimes/_lib/docker.sh")  // helpers, not a provider
	write("runtimes/README.md")       // a file, not a provider
	write("runtimes/notarun/down.sh") // no up.sh
	write("runtimes/incluster/up.sh")

	want := []string{"incluster", "k3d", "kind"}
	if got := DiscoverProviders(root); !slices.Equal(got, want) {
		t.Errorf("DiscoverProviders = %v, want %v", got, want)
	}
	if got := DiscoverProviders(t.TempDir()); len(got) != 0 {
		t.Errorf("a project with no runtimes/ has providers %v", got)
	}
}
