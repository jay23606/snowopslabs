// SPDX-License-Identifier: Apache-2.0

package schemas

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/snowops-oss/snowopslabs/internal/target"
	"github.com/snowops-oss/snowopslabs/pkg/scenario"
)

// TestSchemasAreWellFormed asserts every embedded schema parses as JSON and
// carries the metadata external tooling relies on. A malformed schema would ship
// silently otherwise - editors would just stop offering completion.
func TestSchemasAreWellFormed(t *testing.T) {
	for kind, raw := range ByKind {
		t.Run(kind, func(t *testing.T) {
			if len(raw) == 0 {
				t.Fatalf("schema %q is empty (embed failed)", kind)
			}
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("schema %q is not valid JSON: %v", kind, err)
			}
			for _, key := range []string{"$schema", "$id", "title", "type"} {
				if _, ok := doc[key]; !ok {
					t.Errorf("schema %q missing %q", kind, key)
				}
			}
			if doc["type"] != "object" {
				t.Errorf("schema %q: top-level type = %v, want object", kind, doc["type"])
			}
			id, _ := doc["$id"].(string)
			if !strings.HasSuffix(id, kind+".schema.json") {
				t.Errorf("schema %q: $id = %q, want a %s.schema.json suffix", kind, id, kind)
			}
		})
	}
}

// TestContentKindsHaveSchema guards against adding a content kind without an
// authoring schema (or vice versa).
func TestContentKindsHaveSchema(t *testing.T) {
	want := []string{"check", "scenario", "incident", "path", "challenge"}
	if len(ByKind) != len(want) {
		t.Fatalf("ByKind has %d entries, want %d", len(ByKind), len(want))
	}
	for _, k := range want {
		if _, ok := ByKind[k]; !ok {
			t.Errorf("no schema registered for kind %q", k)
		}
	}
}

// TestNoCloudRuntimesInSchemas enforces ADR-0001: the cloud runtimes were cut,
// so no schema may still offer them as options.
func TestNoCloudRuntimesInSchemas(t *testing.T) {
	for kind, raw := range ByKind {
		for _, banned := range []string{"\"aks\"", "\"eks\"", "\"gke\""} {
			if strings.Contains(string(raw), banned) {
				t.Errorf("schema %q references cut cloud runtime %s (ADR-0001)", kind, banned)
			}
		}
	}
}

// TestMetaEnumsMatchTheCode keeps the track and level lists in the schemas
// equal to the ones the loaders validate against, so an editor never offers a
// value the loader rejects or hides one it accepts.
func TestMetaEnumsMatchTheCode(t *testing.T) {
	for _, kind := range []string{"scenario", "incident", "path", "challenge"} {
		var doc struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(ByKind[kind], &doc); err != nil {
			t.Fatal(err)
		}
		for field, want := range map[string][]string{"track": scenario.Tracks, "level": scenario.Levels} {
			if got := doc.Properties[field].Enum; !slices.Equal(got, want) {
				t.Errorf("%s schema: %s enum = %v, want %v", kind, field, got, want)
			}
		}
		if _, ok := doc.Properties["minutes"]; !ok {
			t.Errorf("%s schema has no minutes property", kind)
		}
	}
}

// TestTargetEnumsMatchTheRegistry keeps every target list in the schemas
// equal to the registry's, so an editor offers exactly the targets labctl
// validate accepts, on every field that takes one.
func TestTargetEnumsMatchTheRegistry(t *testing.T) {
	want := target.NewRegistry(nil).Names()
	type prop struct {
		Type  string   `json:"type"`
		Enum  []string `json:"enum"`
		Items *prop    `json:"items"`
	}
	type object struct {
		Properties map[string]prop `json:"properties"`
	}
	var doc struct {
		object
		Defs map[string]object `json:"$defs"`
	}
	// Each entry is a schema kind and the place in it that holds a target.
	places := []struct{ kind, def, field string }{
		{"scenario", "", "requires"},
		{"scenario", "component", "target"},
		{"scenario", "check", "target"},
		{"incident", "", "requires"},
		{"challenge", "", "requires"},
		{"check", "", "target"},
	}
	for _, p := range places {
		if err := json.Unmarshal(ByKind[p.kind], &doc); err != nil {
			t.Fatal(err)
		}
		props := doc.Properties
		if p.def != "" {
			props = doc.Defs[p.def].Properties
		}
		got, ok := props[p.field]
		if !ok {
			t.Errorf("%s schema %s has no %s property", p.kind, p.def, p.field)
			continue
		}
		enum := got.Enum
		if got.Items != nil {
			enum = got.Items.Enum
		}
		if !slices.Equal(enum, want) {
			t.Errorf("%s schema %s %s enum = %v, want %v", p.kind, p.def, p.field, enum, want)
		}
	}

	// The incident's inject and resolve each take a target.
	var inc struct {
		Properties map[string]object `json:"properties"`
	}
	if err := json.Unmarshal(ByKind["incident"], &inc); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"inject", "resolve"} {
		got := inc.Properties[step].Properties["target"].Enum
		if !slices.Equal(got, want) {
			t.Errorf("incident schema %s.target enum = %v, want %v", step, got, want)
		}
	}
}
