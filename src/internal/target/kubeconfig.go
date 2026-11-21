// SPDX-License-Identifier: Apache-2.0

// Package target describes what a lab runs against. The lab's Kubernetes
// cluster has its own kubeconfig in the lab's state directory, and every run
// uses it instead of the kubectl context the user happens to have selected.
package target

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// KubeconfigFile is the name of the lab's own kubeconfig in its state
// directory (<Home>/state/<cluster>). It holds exactly one context, so every
// run can use it without choosing a context.
const KubeconfigFile = "kubeconfig"

// Runtime profiles that create a cluster the lab owns.
const (
	ProviderK3d       = "k3d"
	ProviderKind      = "kind"
	ProviderIncluster = "incluster"
)

// kubectlTimeout bounds each kubectl call made here; none of them needs the
// cluster to answer.
const kubectlTimeout = 20 * time.Second

// ExpectedContext is the kubeconfig context a local provider creates for
// cluster: k3d-<cluster> and kind-<cluster>. Any other provider names its
// context with override (KUBECONFIG_CONTEXT); with none, the result is empty.
func ExpectedContext(provider, cluster, override string) string {
	switch provider {
	case ProviderK3d:
		return "k3d-" + cluster
	case ProviderKind:
		return "kind-" + cluster
	default:
		return override
	}
}

// EnsureKubeconfig makes sure the lab's kubeconfig exists at path. A lab
// created before labctl wrote the file gets it here, cut from the user's
// kubeconfig (userKubeconfig, or kubectl's default when empty) for
// kubeContext. A context that is not in the user's kubeconfig is an error
// that says to run labctl init.
func EnsureKubeconfig(ctx context.Context, runner toolchain.Runner, path, kubeContext, userKubeconfig string) error {
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return nil
	}
	if kubeContext == "" {
		return errors.New("no kubeconfig context is known for this lab; set KUBECONFIG_CONTEXT in .env to the cluster's context, then run: labctl init")
	}
	kubectl, err := runner.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("finding kubectl to create %s: %w; install it, then run: labctl doctor", path, err)
	}

	ctx, cancel := context.WithTimeout(ctx, kubectlTimeout)
	defer cancel()
	cmd := toolchain.Command{
		Path: kubectl,
		Args: []string{"config", "view", "--minify", "--flatten", "--context", kubeContext},
	}
	if userKubeconfig != "" {
		cmd.Env = map[string]string{"KUBECONFIG": userKubeconfig}
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if _, err := runner.Run(ctx, cmd); err != nil || strings.TrimSpace(out.String()) == "" {
		return fmt.Errorf("kubectl has no context %q, so labctl cannot find this lab's cluster; run: labctl init", kubeContext)
	}
	return writePrivate(path, out.Bytes())
}

// writePrivate writes data to path with mode 0600 through a temporary file, so
// a reader never sees half a kubeconfig.
func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, KubeconfigFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating a temporary kubeconfig in %s: %w", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("installing %s: %w", path, err)
	}
	return nil
}

// UserContext is the current kubectl context in the user's own kubeconfig
// (userKubeconfig, or kubectl's default when empty), which is not the lab's.
// An empty result with no error means the user has no current context.
func UserContext(ctx context.Context, runner toolchain.Runner, userKubeconfig string) (string, error) {
	kubectl, err := runner.LookPath("kubectl")
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, kubectlTimeout)
	defer cancel()

	cmd := toolchain.Command{Path: kubectl, Args: []string{"config", "current-context"}}
	if userKubeconfig != "" {
		cmd.Env = map[string]string{"KUBECONFIG": userKubeconfig}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locating the default kubeconfig: %w", err)
		}
		cmd.Args = append([]string{"--kubeconfig", filepath.Join(home, ".kube", "config")}, cmd.Args...)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if _, err := runner.Run(ctx, cmd); err != nil {
		var exit *toolchain.ExitError
		if errors.As(err, &exit) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}
