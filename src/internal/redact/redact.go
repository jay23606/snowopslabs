// SPDX-License-Identifier: Apache-2.0

// Package redact masks secret values in text before it is stored or streamed.
//
// The lab copies every .env key into script environments, so a script that
// echoes one (or runs under set -x) would otherwise write it into run logs.
package redact

import (
	"os"
	"sort"
	"strings"
	"sync"
)

// MinSecretLen is the shortest value that is redacted. Shorter values match
// too much ordinary text to mask safely.
const MinSecretLen = 8

// secretSuffixes mark an env key whose value is a secret.
var secretSuffixes = []string{"_PASSWORD", "_TOKEN", "_KEY", "_SECRET"}

// IsSecretKey reports whether an env key names a secret by its suffix.
func IsSecretKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, s := range secretSuffixes {
		if strings.HasSuffix(upper, s) {
			return true
		}
	}
	return false
}

// Redactor replaces registered secret values with [redacted:NAME]. Build one
// with New. It is safe for concurrent use, and a nil Redactor masks nothing.
type Redactor struct {
	mu       sync.RWMutex
	secrets  map[string]string // value -> name
	replacer *strings.Replacer // nil when nothing is registered
}

// New returns a Redactor holding the secrets found in env.
func New(env map[string]string) *Redactor {
	r := &Redactor{secrets: make(map[string]string)}
	r.RegisterEnv(env)
	return r
}

// ProcessEnv returns labctl's own environment as a map. Child processes
// inherit it, so its secret-named keys need masking as much as a run's env.
func ProcessEnv() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

// RegisterEnv registers every secret-named key in env.
func (r *Redactor) RegisterEnv(env map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	for k, v := range env {
		if IsSecretKey(k) && r.addLocked(k, v) {
			changed = true
		}
	}
	if changed {
		r.rebuildLocked()
	}
}

// Register adds one secret under name. Values shorter than MinSecretLen are
// ignored, and a value already registered keeps its first name.
func (r *Redactor) Register(name, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.addLocked(name, value) {
		r.rebuildLocked()
	}
}

func (r *Redactor) addLocked(name, value string) bool {
	if len(value) < MinSecretLen {
		return false
	}
	if _, ok := r.secrets[value]; ok {
		return false
	}
	r.secrets[value] = name
	return true
}

// rebuildLocked orders values longest first, so a secret that contains
// another is masked whole rather than leaving a fragment behind.
func (r *Redactor) rebuildLocked() {
	values := make([]string, 0, len(r.secrets))
	for v := range r.secrets {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}
		return values[i] < values[j]
	})
	pairs := make([]string, 0, len(values)*2)
	for _, v := range values {
		pairs = append(pairs, v, "[redacted:"+r.secrets[v]+"]")
	}
	r.replacer = strings.NewReplacer(pairs...)
}

// Redact returns s with every registered secret masked.
func (r *Redactor) Redact(s string) string {
	if r == nil {
		return s
	}
	r.mu.RLock()
	rep := r.replacer
	r.mu.RUnlock()
	if rep == nil {
		return s
	}
	return rep.Replace(s)
}
