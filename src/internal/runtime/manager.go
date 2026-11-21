// SPDX-License-Identifier: Apache-2.0

// Package runtime discovers the cluster runtimes under runtimes/ (k3d, kind,
// incluster), reports which are ready from their status.sh, and runs their up
// and down scripts.
package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/snowops-oss/snowopslabs/internal/config"
	"github.com/snowops-oss/snowopslabs/internal/executor"
	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// RuntimeInfo describes a discovered runtime and its status.
type RuntimeInfo struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`  // status.sh reports the cluster ready
	Current bool   `json:"current"` // the cluster's context is the user's current kubectl context
}

// Manager discovers and manages cluster runtimes (k3d, kind, incluster).
type Manager struct {
	ProjectRoot string
	ClusterName string
	runtimes    []string

	// injectable for tests
	status      func(ctx context.Context, runtime string) (target.Status, error)
	outputs     func(runtime string) (target.Outputs, error)
	userContext func(ctx context.Context) (string, error)
}

// NewManager scans the runtimes/ directory for available runtimes. A runtime's
// state comes from its status.sh and the outputs its up.sh recorded, never from
// kubectl's current context.
func NewManager(projectRoot, clusterName string) *Manager {
	runner := toolchain.NewExec()
	// Without a state directory there are no outputs to read, and the status
	// scripts fall back to their own default location.
	stateDir, _ := config.StateDir(clusterName)
	m := &Manager{ProjectRoot: projectRoot, ClusterName: clusterName}
	m.scan()
	lab := target.Lab{
		Root:     projectRoot,
		Cluster:  clusterName,
		StateDir: stateDir,
		Runner:   runner,
		Registry: target.NewRegistry(m.runtimes),
	}
	m.status = func(ctx context.Context, name string) (target.Status, error) {
		return lab.Status(ctx, target.DefaultName, name)
	}
	m.outputs = lab.Outputs
	m.userContext = func(ctx context.Context) (string, error) {
		return target.UserContext(ctx, runner, target.UserKubeconfig())
	}
	return m
}

// listTimeout bounds the whole of List; every runtime is checked at once.
const listTimeout = 8 * time.Second

// List returns all discovered runtimes with their current status. A runtime is
// active when its status.sh reports the cluster ready, and current when its
// recorded context is the one the user's kubectl has selected. A runtime whose
// state cannot be read is reported as not active.
func (m *Manager) List() []RuntimeInfo {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()

	userCtx, _ := m.userContext(ctx)

	infos := make([]RuntimeInfo, len(m.runtimes))
	var wg sync.WaitGroup
	for i, name := range m.runtimes {
		wg.Go(func() {
			infos[i] = m.info(ctx, name, userCtx)
		})
	}
	wg.Wait()
	return infos
}

func (m *Manager) info(ctx context.Context, name, userCtx string) RuntimeInfo {
	info := RuntimeInfo{Name: name}
	st, err := m.status(ctx, name)
	if err != nil || !st.Ready {
		return info
	}
	info.Active = true
	kubeContext := target.ExpectedContext(name, m.ClusterName, "")
	if out, err := m.outputs(name); err == nil && out.KubeContext != "" {
		kubeContext = out.KubeContext
	}
	info.Current = kubeContext != "" && kubeContext == userCtx
	return info
}

// Names returns the names of all discovered runtimes.
func (m *Manager) Names() []string {
	return m.runtimes
}

// Activate provisions a runtime by running its up.sh script.
func (m *Manager) Activate(name string, exec *executor.Executor) error {
	if !m.exists(name) {
		return fmt.Errorf("runtime %q not found", name)
	}
	scriptPath := filepath.Join("runtimes", name, target.UpScript)
	_, err := exec.RunScriptStreamed(fmt.Sprintf("Activate %s", name), scriptPath, m.ClusterName)
	return err
}

// ActivateWith provisions a runtime using a pre-allocated action ID so the
// HTTP 202 response and WebSocket action_start event share the same ID.
func (m *Manager) ActivateWith(actionID, name string, exec *executor.Executor) error {
	if !m.exists(name) {
		return fmt.Errorf("runtime %q not found", name)
	}
	scriptPath := filepath.Join("runtimes", name, target.UpScript)
	return exec.RunScriptStreamedWith(actionID, fmt.Sprintf("Activate %s", name), scriptPath, m.ClusterName)
}

// Deactivate tears down a runtime by running its down.sh script.
func (m *Manager) Deactivate(name string, exec *executor.Executor) error {
	if !m.exists(name) {
		return fmt.Errorf("runtime %q not found", name)
	}
	scriptPath := filepath.Join("runtimes", name, target.DownScript)
	_, err := exec.RunScriptStreamed(fmt.Sprintf("Deactivate %s", name), scriptPath, m.ClusterName)
	return err
}

// DeactivateWith tears down a runtime using a pre-allocated action ID.
func (m *Manager) DeactivateWith(actionID, name string, exec *executor.Executor) error {
	if !m.exists(name) {
		return fmt.Errorf("runtime %q not found", name)
	}
	scriptPath := filepath.Join("runtimes", name, target.DownScript)
	return exec.RunScriptStreamedWith(actionID, fmt.Sprintf("Deactivate %s", name), scriptPath, m.ClusterName)
}

func (m *Manager) scan() {
	m.runtimes = target.DiscoverProviders(m.ProjectRoot)
}

func (m *Manager) exists(name string) bool {
	return slices.Contains(m.runtimes, name)
}
