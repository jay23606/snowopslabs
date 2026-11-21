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
const statusTimeout = 20 * time.Second

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

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
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
