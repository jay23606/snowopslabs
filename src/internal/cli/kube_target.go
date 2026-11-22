// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/snowops-oss/snowopslabs/internal/config"
	"github.com/snowops-oss/snowopslabs/internal/display"
	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// useLabCluster points this process, and so every child it starts (run engine,
// web UI executor, check scripts and Go's own kubectl calls), at the lab's own
// kubeconfig instead of the user's current kubectl context. It first creates
// that file for a lab that predates it. A lab with no usable context is only
// reported for commands that need the cluster; the ones that create or report
// on it (init, reset, lab up, status) have their own message.
func useLabCluster(ctx context.Context, cmd *cobra.Command, runner toolchain.Runner, lab *config.Config) error {
	if lab.Kubeconfig == "" {
		return nil
	}
	user := target.UserKubeconfig()
	// A KUBECONFIG that already is the lab's file is not the user's own: the
	// runtime scripts merge into and clean up kubectl's default file instead.
	if sameFile(user, lab.Kubeconfig) {
		user = ""
	}
	if err := os.Setenv(target.UserKubeconfigEnv, user); err != nil {
		return fmt.Errorf("setting %s: %w", target.UserKubeconfigEnv, err)
	}
	if err := target.EnsureKubeconfig(ctx, runner, lab.Profile, lab.Kubeconfig, lab.KubeContext, user); err != nil && !expectsNoCluster(cmd) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", err)
	}
	if err := os.Setenv("KUBECONFIG", lab.Kubeconfig); err != nil {
		return fmt.Errorf("setting KUBECONFIG to the lab's kubeconfig: %w", err)
	}
	return nil
}

// sameFile reports whether a and b name the same file, following symlinks. An
// empty name is no file.
func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// expectsNoCluster reports whether cmd builds, rebuilds or reports on the lab's
// cluster, so a missing kubectl context is no surprise when it starts.
func expectsNoCluster(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "init", "reset", "teardown", "setup-tools", "status":
		return true
	case "up", "down":
		p := cmd.Parent()
		return p != nil && (p.Name() == "lab" || p.Name() == "runtime")
	}
	return false
}

// labTarget is the lab the CLI is running, for the target package.
func labTarget() target.Lab {
	return target.Lab{
		Root:     cfg.ProjectRoot,
		Cluster:  cfg.ClusterName,
		StateDir: cfg.StateDir,
		Runner:   toolchain.NewExec(),
		Registry: target.NewRegistry(target.DiscoverProviders(cfg.ProjectRoot)),
		// The k8s target is supplied by the lab's runtime profile.
		Providers: map[string]string{target.DefaultName: cfg.Profile},
	}
}

// preflight remembers the target lists a command already found ready, so the
// engine that starts the content next does not run each status.sh again.
var preflight = struct {
	sync.Mutex
	ready map[string]bool
}{ready: map[string]bool{}}

// targetsKey names a list of targets; no names is the default target.
func targetsKey(names []string) string {
	if len(names) == 0 {
		return target.DefaultName
	}
	return strings.Join(names, ",")
}

// preflightTargets is what the commands call first, so nothing is installed
// for content that cannot run. A list that passes is not checked again by the
// engine that starts the content in the same command.
func preflightTargets(ctx context.Context, names []string) error {
	if err := labTarget().Require(ctx, names); err != nil {
		return err
	}
	preflight.Lock()
	defer preflight.Unlock()
	preflight.ready[targetsKey(names)] = true
	return nil
}

// requireTargets stops content from starting against a target that is not
// ready. It is what the scenario and incident engines call before they begin.
// A list the command's preflight already passed is let through once.
func requireTargets(ctx context.Context, names []string) error {
	preflight.Lock()
	key := targetsKey(names)
	passed := preflight.ready[key]
	delete(preflight.ready, key)
	preflight.Unlock()
	if passed {
		return nil
	}
	return labTarget().Require(ctx, names)
}

// printTarget says which cluster the lab runs against and, when the user's
// current kubectl context is another one, that plain kubectl is aimed
// elsewhere. labctl itself never follows that context.
func printTarget(ctx context.Context, out io.Writer, runner toolchain.Runner, lab *config.Config) {
	if lab.KubeContext == "" {
		return
	}
	// Learners never need the word "target"; the useful fact is which
	// kubeconfig labctl uses, so they can point plain kubectl at it too.
	where := "context " + lab.KubeContext
	if lab.Kubeconfig != "" {
		where = "kubeconfig " + display.Path(lab.Kubeconfig, "", "")
	}
	fmt.Fprintf(out, "  Lab:      %s cluster, %s\n", lab.Profile, where)
	if note := currentContextNote(ctx, runner, lab); note != "" {
		fmt.Fprintf(out, "  Note:     %s\n", note)
	}
}

// currentContextNote warns that the user's current kubectl context is not the
// lab's. It is empty when they match, when the lab has no context of its own,
// or when the current context cannot be read.
func currentContextNote(ctx context.Context, runner toolchain.Runner, lab *config.Config) string {
	if lab.KubeContext == "" {
		return ""
	}
	current, err := target.UserContext(ctx, runner, target.UserKubeconfig())
	if err != nil || current == "" || current == lab.KubeContext {
		return ""
	}
	return fmt.Sprintf("your current kubectl context is %q, not the lab's %q. labctl runs only against the lab; "+
		"plain kubectl uses %q until you run: kubectl config use-context %s",
		current, lab.KubeContext, current, lab.KubeContext)
}
