// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/snowops-oss/snowopslabs/internal/config"
	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// userKubeconfigEnv holds the KUBECONFIG the user started labctl with, so the
// runtime scripts can still merge the lab's context into the user's own
// kubeconfig once KUBECONFIG itself points at the lab.
const userKubeconfigEnv = "SNOWOPS_USER_KUBECONFIG"

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
	user := userKubeconfig()
	if err := os.Setenv(userKubeconfigEnv, user); err != nil {
		return fmt.Errorf("setting %s: %w", userKubeconfigEnv, err)
	}
	if user == lab.Kubeconfig {
		user = ""
	}
	if err := target.EnsureKubeconfig(ctx, runner, lab.Kubeconfig, lab.KubeContext, user); err != nil && !expectsNoCluster(cmd) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\n", err)
	}
	if err := os.Setenv("KUBECONFIG", lab.Kubeconfig); err != nil {
		return fmt.Errorf("setting KUBECONFIG to the lab's kubeconfig: %w", err)
	}
	return nil
}

// userKubeconfig is the KUBECONFIG the user started labctl with: the one
// recorded before labctl pointed KUBECONFIG at the lab, else the current one.
func userKubeconfig() string {
	if v, ok := os.LookupEnv(userKubeconfigEnv); ok {
		return v
	}
	return os.Getenv("KUBECONFIG")
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

// printTarget says which cluster the lab runs against and, when the user's
// current kubectl context is another one, that plain kubectl is aimed
// elsewhere. labctl itself never follows that context.
func printTarget(ctx context.Context, out io.Writer, runner toolchain.Runner, lab *config.Config) {
	if lab.KubeContext == "" {
		return
	}
	fmt.Fprintf(out, "  Target:   k8s (%s), context %s\n", lab.Profile, lab.KubeContext)
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
	current, err := target.UserContext(ctx, runner, userKubeconfig())
	if err != nil || current == "" || current == lab.KubeContext {
		return ""
	}
	return fmt.Sprintf("your current kubectl context is %q, not the lab's %q. labctl runs only against the lab; "+
		"plain kubectl uses %q until you run: kubectl config use-context %s",
		current, lab.KubeContext, current, lab.KubeContext)
}
