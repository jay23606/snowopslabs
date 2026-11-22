// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"slices"
	"strings"
)

// DefaultTarget is the target content runs against unless it names another:
// the lab's Kubernetes cluster.
const DefaultTarget = "k8s"

// Needs is the list of targets a scenario, incident or challenge needs, so
// activation can check each one is ready first. It is embedded inline in each
// item's YAML.
type Needs struct {
	// Requires names the targets the item needs. Empty means [k8s]. List k8s
	// as well as any other target when the item uses the cluster too.
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
}

// RequiredTargets returns the targets the item needs, in the order listed,
// or [k8s] when it lists none.
func (n Needs) RequiredTargets() []string {
	var out []string
	for _, name := range n.Requires {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return []string{DefaultTarget}
	}
	return out
}

// TargetUse is one place content names a target with a target: field.
type TargetUse struct {
	// Where says which part of the item it is, such as `check "ready"`.
	Where string
	// Target is the name the field holds. Empty means the default and is not
	// checked.
	Target string
}

// ValidateTargets reports a requires entry that is empty or listed twice and
// every use whose target the item does not list in requires. Whether a name
// is one this lab can supply is checked where the target registry is
// available (labctl validate).
func (n Needs) ValidateTargets(uses []TargetUse) []string {
	var errs []string
	seen := map[string]bool{}
	for _, raw := range n.Requires {
		name := strings.TrimSpace(raw)
		switch {
		case name == "":
			errs = append(errs, "requires: a target name is empty")
		case seen[name]:
			errs = append(errs, fmt.Sprintf("requires: target %q is listed twice", name))
		}
		seen[name] = true
	}
	listed := n.RequiredTargets()
	for _, u := range uses {
		name := strings.TrimSpace(u.Target)
		if name == "" || slices.Contains(listed, name) {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: target %q is not in requires (%s); add it there",
			u.Where, name, strings.Join(listed, ", ")))
	}
	return errs
}
