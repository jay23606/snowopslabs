// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/snowops-oss/snowopslabs/internal/redact"
	"github.com/snowops-oss/snowopslabs/internal/store"
)

const testSecret = "s3cr3t-admin-pw"

func TestRedaction_ScriptOutputIsMaskedInStoreAndStream(t *testing.T) {
	h := newHarness(t)
	h.fake.WhenArgsContainStderr("leak.sh", "password is "+testSecret+"\n", "+ echo "+testSecret+"\n", 0)

	events, unsubscribe := h.engine.Subscribe("")
	defer unsubscribe()

	id, err := h.engine.Submit(context.Background(), Spec{
		Kind: "test", Script: h.script("leak.sh"),
		Env: map[string]string{"GRAFANA_ADMIN_PASSWORD": testSecret, "CLUSTER_NAME": "lab-one-name"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	h.waitForStatus(id, store.StatusSucceeded)

	log := h.logText(id)
	if strings.Contains(log, testSecret) {
		t.Fatalf("stored log holds the secret:\n%s", log)
	}
	if got := strings.Count(log, "[redacted:GRAFANA_ADMIN_PASSWORD]"); got != 2 {
		t.Errorf("mask appears %d times, want 2 (stdout and stderr):\n%s", got, log)
	}

	// A watcher learns about lines by cursor and reads them back from the
	// store, so what it reads is the redacted text.
	select {
	case ev := <-events:
		if ev.Type == EventLog {
			lines, err := h.store.ReadLogs(context.Background(), id, 0, 100)
			if err != nil {
				t.Fatalf("ReadLogs: %v", err)
			}
			for _, l := range lines {
				if strings.Contains(l.Text, testSecret) {
					t.Errorf("watcher read the secret: %q", l.Text)
				}
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered")
	}
}

func TestRedaction_SecretSplitAcrossWrites(t *testing.T) {
	h := newHarness(t)
	id, err := h.engine.Submit(context.Background(), Spec{
		Kind: "test",
		Env:  map[string]string{"X_TOKEN": testSecret},
		Func: func(_ context.Context, out io.Writer) error {
			_, _ = io.WriteString(out, "token="+testSecret[:6])
			_, _ = io.WriteString(out, testSecret[6:]+"\n")
			_, _ = io.WriteString(out, "tail "+testSecret) // no newline: flushed at exit
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	h.waitForStatus(id, store.StatusSucceeded)
	log := h.logText(id)
	if strings.Contains(log, testSecret) {
		t.Fatalf("stored log holds the secret:\n%s", log)
	}
	if got := strings.Count(log, "[redacted:X_TOKEN]"); got != 2 {
		t.Errorf("mask appears %d times, want 2:\n%s", got, log)
	}
}

func TestRedaction_RegisteredValueAndFailureMessage(t *testing.T) {
	r := redact.New(nil)
	h := newHarness(t, WithRedactor(r))
	r.Register("CONNECT_TOKEN", testSecret)
	h.fake.WhenArgsContainStderr("fail.sh", "", "denied for "+testSecret, 1)

	id, err := h.engine.Submit(context.Background(), Spec{Kind: "test", Script: h.script("fail.sh")})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	rec := h.waitForStatus(id, store.StatusFailed)

	if strings.Contains(rec.Error, testSecret) || strings.Contains(h.logText(id), testSecret) {
		t.Errorf("secret survived: error=%q log=%q", rec.Error, h.logText(id))
	}
	if !strings.Contains(rec.Error, "[redacted:CONNECT_TOKEN]") {
		t.Errorf("failure message = %q, want the mask", rec.Error)
	}
}

func TestRedaction_NoSecretsLeavesOutputAlone(t *testing.T) {
	h := newHarness(t)
	h.fake.WhenArgsContain("ok.sh", "plain "+testSecret+"\n", 0)
	id, err := h.engine.Submit(context.Background(), Spec{
		Kind: "test", Script: h.script("ok.sh"),
		Env: map[string]string{"SHORT_PASSWORD": "short"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	h.waitForStatus(id, store.StatusSucceeded)
	if !strings.Contains(h.logText(id), "plain "+testSecret) {
		t.Errorf("unregistered text was changed: %q", h.logText(id))
	}
}

// A sink holding a registered secret never stores it, whichever way the line
// arrives.
func TestLogSink_NeverStoresRegisteredSecret(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/sink.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const id = "sink-run"
	if err := st.CreateRun(ctx, store.Run{ID: id, Kind: "test"}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	r := redact.New(nil)
	r.Register("A_KEY", testSecret)
	sink := newLogSink(ctx, st, newSubscribers(), id, time.Now, r)
	sink.system("system " + testSecret)
	w := sink.writer(store.StreamStderr)
	_, _ = io.WriteString(w, "err "+testSecret+"\n")
	sink.close()

	lines, err := st.ReadLogs(ctx, id, 0, 100)
	if err != nil {
		t.Fatalf("ReadLogs: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for _, l := range lines {
		if strings.Contains(l.Text, testSecret) {
			t.Errorf("sink stored the secret: %q", l.Text)
		}
	}
}

func TestRedaction_InheritedProcessEnvIsMasked(t *testing.T) {
	// Scripts inherit labctl's own environment, so a secret exported in the
	// shell that started labctl reaches them without being in Spec.Env.
	t.Setenv("PROBE_API_TOKEN", testSecret)
	h := newHarness(t)
	h.fake.WhenArgsContainStderr("inherit.sh", "token="+testSecret+"\n", "", 0)

	id, err := h.engine.Submit(context.Background(), Spec{Kind: "test", Script: h.script("inherit.sh")})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	h.waitForStatus(id, store.StatusSucceeded)

	log := h.logText(id)
	if strings.Contains(log, testSecret) {
		t.Fatalf("stored log holds an inherited secret:\n%s", log)
	}
	if !strings.Contains(log, "[redacted:PROBE_API_TOKEN]") {
		t.Errorf("inherited secret not masked by name:\n%s", log)
	}
}
