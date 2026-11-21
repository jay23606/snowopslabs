// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/internal/toolchain"
)

// currentContextRunner answers `kubectl config current-context` with the
// user's selected context and leaves every other command to the fake.
type currentContextRunner struct {
	*toolchain.Fake
	current string
}

func (r currentContextRunner) Run(ctx context.Context, cmd toolchain.Command) (toolchain.Result, error) {
	if slices.Contains(cmd.Args, "current-context") {
		_, _ = cmd.Stdout.Write([]byte(r.current + "\n"))
		return toolchain.Result{}, nil
	}
	return r.Fake.Run(ctx, cmd)
}

// doctor names the cluster the lab targets and says when the user's current
// kubectl context is another one.
func TestRunDoctor_NamesTheLabTarget(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    []string
		absent  []string
	}{
		{
			name:    "another cluster is selected",
			current: "colleague-staging",
			want:    []string{"Lab:      k3d cluster, ", `"colleague-staging"`, "kubectl config use-context k3d-clitest"},
		},
		{
			name:    "the lab's context is selected",
			current: "k3d-clitest",
			want:    []string{"Lab:      k3d cluster, "},
			absent:  []string{"Note:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTerminal(t)
			saved := projectDir
			projectDir = l.Root
			t.Cleanup(func() { projectDir = saved })
			t.Setenv("KUBECONFIG", "/home/me/.kube/config")
			os.Unsetenv(target.UserKubeconfigEnv)

			var out bytes.Buffer
			runner := currentContextRunner{Fake: fakeEnv("16 17179869184"), current: tt.current}
			_ = runDoctor(t.Context(), &out, runner)

			for _, w := range tt.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("doctor output does not contain %q:\n%s", w, out.String())
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(out.String(), a) {
					t.Errorf("doctor output contains %q:\n%s", a, out.String())
				}
			}
		})
	}
}
