// SPDX-License-Identifier: Apache-2.0

// Package target describes what a lab runs against. A lab is a set of named
// targets; the Kubernetes cluster is the default target, "k8s", and the
// runtime profiles under runtimes/ (k3d, kind, incluster) are its providers.
//
// A provider is a directory with up.sh, down.sh and status.sh. Its up.sh
// records what it produced in outputs.env in the lab's state directory, and
// the lab's Kubernetes cluster has its own kubeconfig there too, which every
// run uses instead of the kubectl context the user has selected.
package target

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DefaultName is the target every scenario, incident and challenge uses
// unless it names another.
const DefaultName = "k8s"

// Scripts a provider directory holds. up.sh and down.sh create and remove the
// target; status.sh exits 0 when it is ready.
const (
	UpScript     = "up.sh"
	DownScript   = "down.sh"
	StatusScript = "status.sh"
)

// Target is one named part of a lab and the providers that can supply it.
type Target struct {
	Name      string
	Providers []string
}

// HasProvider reports whether name is one of the target's providers.
func (t Target) HasProvider(name string) bool {
	return slices.Contains(t.Providers, name)
}

// Registry is the set of targets a lab can have.
type Registry struct {
	targets map[string]Target
}

// NewRegistry returns the registry with the targets labctl ships. Only the
// Kubernetes cluster is registered today, with k8sProviders as its providers.
func NewRegistry(k8sProviders []string) *Registry {
	k8s := Target{Name: DefaultName, Providers: slices.Clone(k8sProviders)}
	return &Registry{targets: map[string]Target{k8s.Name: k8s}}
}

// Names lists the registered targets in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.targets))
	for name := range r.targets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Lookup returns the named target. An empty name means the default target. An
// unknown name is an error that lists the known ones.
func (r *Registry) Lookup(name string) (Target, error) {
	if name == "" {
		name = DefaultName
	}
	t, ok := r.targets[name]
	if !ok {
		return Target{}, fmt.Errorf("unknown target %q; this lab has: %s", name, strings.Join(r.Names(), ", "))
	}
	return t, nil
}

// DiscoverProviders lists the runtime profiles under root/runtimes: the
// directories that hold an up.sh, in sorted order. A directory without one,
// such as _lib, is not a provider.
func DiscoverProviders(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "runtimes"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "runtimes", e.Name(), UpScript)); err == nil {
			names = append(names, e.Name())
		}
	}
	return names
}
