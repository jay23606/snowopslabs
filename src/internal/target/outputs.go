// SPDX-License-Identifier: Apache-2.0

package target

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// OutputsFile is the name of the file a provider's up.sh writes in the lab's
// state directory (<Home>/state/<cluster>).
const OutputsFile = "outputs.env"

// Keys of the outputs file.
const (
	KeyKubeconfig   = "KUBECONFIG"
	KeyKubeContext  = "KUBE_CONTEXT"
	KeyHTTPPort     = "HTTP_PORT"
	KeyHTTPSPort    = "HTTPS_PORT"
	KeyDomainSuffix = "DOMAIN_SUFFIX"
)

// Outputs is what a provider's up.sh recorded about the target it brought up.
type Outputs struct {
	// Kubeconfig is the path of the file that reaches the cluster. Empty for
	// incluster, which uses the pod's ServiceAccount.
	Kubeconfig string
	// KubeContext is the one context in that file. Empty for incluster.
	KubeContext string
	// HTTPPort and HTTPSPort are the host ports of the cluster's ingress.
	HTTPPort, HTTPSPort string
	// DomainSuffix is the suffix of the cluster's ingress hostnames.
	DomainSuffix string
}

// ErrNoOutputs means the provider has not recorded an outputs file, which is
// the state of a lab that was never brought up.
var ErrNoOutputs = errors.New("the lab has no outputs file")

// LoadOutputs reads and validates the outputs file at path. provider is the
// provider that wrote it: every provider but incluster must name a kubeconfig
// and a context. A missing file wraps ErrNoOutputs.
func LoadOutputs(path, provider string) (Outputs, error) {
	f, err := os.Open(path) //nolint:gosec // path is the lab's own state file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Outputs{}, fmt.Errorf("%w (%s); bring the lab up with: labctl init", ErrNoOutputs, path)
		}
		return Outputs{}, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	vals := map[string]string{}
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return Outputs{}, fmt.Errorf("%s line %d is %q, not KEY=value; rerun labctl init to write it again", path, n, line)
		}
		vals[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Outputs{}, fmt.Errorf("reading %s: %w", path, err)
	}

	out := Outputs{
		Kubeconfig:   vals[KeyKubeconfig],
		KubeContext:  vals[KeyKubeContext],
		HTTPPort:     vals[KeyHTTPPort],
		HTTPSPort:    vals[KeyHTTPSPort],
		DomainSuffix: vals[KeyDomainSuffix],
	}
	if err := out.validate(provider); err != nil {
		return Outputs{}, fmt.Errorf("%s: %w; rerun labctl init to write it again", path, err)
	}
	return out, nil
}

func (o Outputs) validate(provider string) error {
	if provider != ProviderIncluster {
		if o.Kubeconfig == "" {
			return fmt.Errorf("%s is missing", KeyKubeconfig)
		}
		if o.KubeContext == "" {
			return fmt.Errorf("%s is missing", KeyKubeContext)
		}
	}
	for _, p := range []struct{ key, port string }{{KeyHTTPPort, o.HTTPPort}, {KeyHTTPSPort, o.HTTPSPort}} {
		if p.port == "" {
			continue
		}
		if n, err := strconv.Atoi(p.port); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s is %q, not a port number", p.key, p.port)
		}
	}
	return nil
}
