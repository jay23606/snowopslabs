// SPDX-License-Identifier: Apache-2.0

package checks

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// A check names the target it runs against with target:, which every check
// type accepts and which stays empty (the k8s default) when absent.
func TestCheck_TargetFromYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"absent means the default", "name: a\ntype: script\nscript: a.sh\n", ""},
		{"a script check", "name: a\ntype: script\nscript: a.sh\ntarget: k8s\n", "k8s"},
		{"an http check", "name: a\ntype: http\nurl: http://x\ntarget: git-forge\n", "git-forge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Check
			if err := yaml.Unmarshal([]byte(tt.yaml), &c); err != nil {
				t.Fatal(err)
			}
			if c.Target != tt.want {
				t.Errorf("Target = %q, want %q", c.Target, tt.want)
			}
			if err := c.Validate(); err != nil {
				t.Errorf("Validate() = %v, want a target to be accepted on any check", err)
			}
		})
	}
}
