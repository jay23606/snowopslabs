// SPDX-License-Identifier: Apache-2.0

package target

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// statusTimeout bounds one status.sh run. A status check that takes longer is
// broken, and callers poll it.
var statusTimeout = 20 * time.Second

// maxReasonRunes keeps a provider's reason to one line a person can read.
const maxReasonRunes = 300

// ErrNoStatusScript means the provider has no status.sh, so its state is not
// known.
var ErrNoStatusScript = errors.New("the provider has no status.sh")

// Lab is the lab a target belongs to: where its content is, which cluster it
// is, where its state lives, and the runner that executes provider scripts.
type Lab struct {
	// Root is the project root that holds runtimes/.
	Root string
	// Cluster is the lab's cluster name, passed to every provider script.
	Cluster string
	// StateDir is <Home>/state/<cluster>.
	StateDir string
	Runner   toolchain.Runner
	Registry *Registry
	// Providers maps each target the lab has to the provider that supplies
	// it, such as k8s to k3d.
	Providers map[string]string
}

// KubeconfigPath is the lab's own kubeconfig.
func (l Lab) KubeconfigPath() string { return filepath.Join(l.StateDir, KubeconfigFile) }

// OutputsPath is the outputs file the provider's up.sh writes.
func (l Lab) OutputsPath() string { return filepath.Join(l.StateDir, OutputsFile) }

// Outputs reads the outputs the provider recorded. A lab that was never
// brought up has none, which wraps ErrNoOutputs.
func (l Lab) Outputs(provider string) (Outputs, error) {
	return LoadOutputs(l.OutputsPath(), provider)
}

// Status is a provider's answer to status.sh.
type Status struct {
	// Ready is true when status.sh exited 0.
	Ready bool
	// Reason is the one line status.sh printed to stderr when it was not.
	Reason string
}

// Status runs provider's status.sh for the named target and reports whether
// the target is ready. Naming a target the lab does not have, or a provider
// that does not supply it, is an error that lists the ones it does. A provider
// without status.sh wraps ErrNoStatusScript.
func (l Lab) Status(ctx context.Context, targetName, provider string) (Status, error) {
	t, err := l.Registry.Lookup(targetName)
	if err != nil {
		return Status{}, err
	}
	if !t.HasProvider(provider) {
		return Status{}, fmt.Errorf("%q is not a provider of target %s; its providers are: %s",
			provider, t.Name, strings.Join(t.Providers, ", "))
	}
	resolver, err := toolchain.NewResolver(l.Root)
	if err != nil {
		return Status{}, err
	}
	script, err := resolver.Resolve(path.Join("runtimes", provider, StatusScript))
	if err != nil {
		if errors.Is(err, toolchain.ErrScriptNotFound) {
			return Status{}, fmt.Errorf("%w: add runtimes/%s/%s, which exits 0 when the target is ready", ErrNoStatusScript, provider, StatusScript)
		}
		return Status{}, err
	}
	bash, err := l.Runner.LookPath("bash")
	if err != nil {
		return Status{}, fmt.Errorf("finding bash to run %s: %w", script, err)
	}

	parent := ctx
	ctx, cancel := context.WithTimeout(parent, statusTimeout)
	defer cancel()
	cmd := toolchain.Command{
		Path: bash,
		Args: []string{script, l.Cluster},
		Dir:  l.Root,
		Env:  l.scriptEnv(provider),
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if _, err := l.Runner.Run(ctx, cmd); err != nil {
		var exit *toolchain.ExitError
		if errors.As(err, &exit) {
			return Status{Reason: reason(stderr.String(), exit.ExitCode)}, nil
		}
		// The check's own deadline, not the caller's, is a target that did not
		// answer; the caller giving up stays an error.
		if parent.Err() == nil && (ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded)) {
			return Status{Reason: fmt.Sprintf("status check did not answer within %s; is Docker busy?", statusTimeout)}, nil
		}
		return Status{}, fmt.Errorf("running %s: %w", path.Join("runtimes", provider, StatusScript), err)
	}
	return Status{Ready: true}, nil
}

// scriptEnv is what a provider script reads: the cluster and its state, and,
// for a provider that has one, the lab's own kubeconfig.
func (l Lab) scriptEnv(provider string) map[string]string {
	env := map[string]string{
		"CLUSTER_NAME":  l.Cluster,
		"LAB_STATE_DIR": l.StateDir,
	}
	if provider != ProviderIncluster {
		env["KUBECONFIG"] = l.KubeconfigPath()
		env["SNOWOPS_LAB_KUBECONFIG"] = l.KubeconfigPath()
	}
	return env
}

// reason is the first line status.sh printed to stderr, shortened, or a
// statement of its exit code when it printed nothing.
func reason(stderr string, exitCode int) string {
	for line := range strings.Lines(stderr) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > maxReasonRunes {
			line = string([]rune(line)[:maxReasonRunes]) + "..."
		}
		return line
	}
	return fmt.Sprintf("status.sh exited %d without a reason", exitCode)
}

// RequireFunc checks that the named targets are ready and returns an error
// that says what to run when one is not. Lab.Require is the implementation;
// engines take the function so they stay free of the lab's wiring.
type RequireFunc func(ctx context.Context, names []string) error

// Require checks that every named target is ready, so content that needs
// them does not start against one that is missing. An empty list means the
// default target. The error names the first target that is not ready and
// the command that brings it up. A provider without status.sh is not
// blocked, since its state cannot be known.
func (l Lab) Require(ctx context.Context, names []string) error {
	if len(names) == 0 {
		names = []string{DefaultName}
	}
	checked := map[string]bool{}
	for _, name := range names {
		if checked[name] {
			continue
		}
		checked[name] = true
		t, err := l.Registry.Lookup(name)
		if err != nil {
			return err
		}
		provider := l.Providers[t.Name]
		if provider == "" {
			return fmt.Errorf("target %s has no provider in this lab; it can be supplied by: %s",
				t.Name, strings.Join(t.Providers, ", "))
		}
		st, err := l.Status(ctx, t.Name, provider)
		switch {
		case errors.Is(err, ErrNoStatusScript):
			continue
		case err != nil:
			return fmt.Errorf("checking target %s: %w", t.Name, err)
		case !st.Ready:
			return fmt.Errorf("target %s is not ready: %s\n%s", t.Name, st.Reason, upAdvice(t.Name, provider))
		}
	}
	return nil
}

// upAdvice is the command that brings a target up.
func upAdvice(name, provider string) string {
	if name == DefaultName && provider == ProviderIncluster {
		return "Check that the hosting cluster's API server is up and that this pod's ServiceAccount may reach it"
	}
	if name == DefaultName {
		return "Check the lab with: labctl status\nBring it up with:   labctl init"
	}
	return fmt.Sprintf("Bring it up with its provider: runtimes/%s/%s", provider, UpScript)
}
