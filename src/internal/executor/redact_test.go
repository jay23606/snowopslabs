// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// drainEvents collects events until action_end arrives.
func drainEvents(t *testing.T, ch chan ActionEvent) []ActionEvent {
	t.Helper()
	var evs []ActionEvent
	for {
		select {
		case ev := <-ch:
			evs = append(evs, ev)
			if ev.Type == "action_end" {
				return evs
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no action_end event")
		}
	}
}

func assertNoSecret(t *testing.T, evs []ActionEvent, secret string) {
	t.Helper()
	for _, ev := range evs {
		for _, f := range []string{ev.Command, ev.Error, ev.Output} {
			if strings.Contains(f, secret) {
				t.Errorf("%s event leaks the secret: %+v", ev.Type, ev)
			}
		}
	}
}

func TestActionEventMetadataIsRedacted(t *testing.T) {
	const secret = "s3cr3t-admin-pw"
	t.Run("command in start and end, env set directly", func(t *testing.T) {
		e := New(t.TempDir())
		e.Env["API_TOKEN"] = secret
		ch := e.Broadcast.Subscribe()
		if _, err := e.RunCommandStreamed("x", "echo", secret); err != nil {
			t.Fatal(err)
		}
		evs := drainEvents(t, ch)
		assertNoSecret(t, evs, secret)
		if evs[0].Type != "action_start" || !strings.Contains(evs[0].Command, "[redacted:API_TOKEN]") {
			t.Errorf("start event = %+v", evs[0])
		}
	})
	t.Run("missing script", func(t *testing.T) {
		e := New(t.TempDir())
		e.SetEnv("API_TOKEN", secret)
		ch := e.Broadcast.Subscribe()
		if _, err := e.RunScriptStreamed("x", "no-such-"+secret+".sh"); err == nil {
			t.Fatal("expected error")
		}
		assertNoSecret(t, drainEvents(t, ch), secret)
	})
	t.Run("start failure", func(t *testing.T) {
		e := New(t.TempDir())
		e.SetEnv("API_TOKEN", secret)
		ch := e.Broadcast.Subscribe()
		if _, err := e.RunCommandStreamed("x", "no-such-binary-"+secret); err == nil {
			t.Fatal("expected error")
		}
		assertNoSecret(t, drainEvents(t, ch), secret)
	})
	t.Run("BroadcastEnd", func(t *testing.T) {
		e := New(t.TempDir())
		e.SetEnv("API_TOKEN", secret)
		ch := e.Broadcast.Subscribe()
		e.BroadcastEnd("1", "x", errors.New("failed with "+secret))
		assertNoSecret(t, drainEvents(t, ch), secret)
	})
}

// streamedOutput runs sh -c script through the broadcast path and returns the
// action_output lines it sent.
func streamedOutput(t *testing.T, e *Executor, script string) []string {
	t.Helper()
	ch := e.Broadcast.Subscribe()
	defer e.Broadcast.Unsubscribe(ch)

	if _, err := e.RunCommandStreamed("leak", "sh", "-c", script); err != nil {
		t.Fatalf("RunCommandStreamed: %v", err)
	}
	var out []string
	for {
		select {
		case ev := <-ch:
			if ev.Type == "action_output" {
				out = append(out, ev.Output)
			}
			if ev.Type == "action_end" {
				return out
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no action_end event")
		}
	}
}

func TestStreamedOutputIsRedacted(t *testing.T) {
	const secret = "s3cr3t-admin-pw"
	tests := []struct {
		name   string
		setup  func(e *Executor)
		script string
		want   string
	}{
		{"set with SetEnv", func(e *Executor) { e.SetEnv("GRAFANA_ADMIN_PASSWORD", secret) },
			`echo "pw=$GRAFANA_ADMIN_PASSWORD"`, "pw=[redacted:GRAFANA_ADMIN_PASSWORD]"},
		{"written to Env directly", func(e *Executor) { e.Env["API_TOKEN"] = secret },
			`echo "t=$API_TOKEN" >&2`, "t=[redacted:API_TOKEN]"},
		{"registered later", func(e *Executor) { e.Redactor.Register("CONNECT_TOKEN", secret) },
			"echo " + secret, "[redacted:CONNECT_TOKEN]"},
		{"short value left alone", func(e *Executor) { e.SetEnv("X_PASSWORD", "short") },
			`echo "$X_PASSWORD"`, "short"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := New(t.TempDir())
			var stdout, stderr bytes.Buffer
			e.Stdout, e.Stderr = &stdout, &stderr
			tc.setup(e)

			got := streamedOutput(t, e, tc.script)
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("broadcast lines = %q, want [%q]", got, tc.want)
			}
			if strings.Contains(stdout.String()+stderr.String(), secret) {
				t.Errorf("the terminal copy holds the secret: %q %q", stdout.String(), stderr.String())
			}
		})
	}
}
