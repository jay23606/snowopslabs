// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"fmt"
	"sync"
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		in   string
		want string
	}{
		{"password", map[string]string{"GRAFANA_ADMIN_PASSWORD": "hunter2hunter2"}, "login hunter2hunter2 ok", "login [redacted:GRAFANA_ADMIN_PASSWORD] ok"},
		{"all suffixes", map[string]string{"A_PASSWORD": "aaaaaaaa1", "B_TOKEN": "bbbbbbbb2", "C_KEY": "cccccccc3", "D_SECRET": "dddddddd4"},
			"aaaaaaaa1 bbbbbbbb2 cccccccc3 dddddddd4",
			"[redacted:A_PASSWORD] [redacted:B_TOKEN] [redacted:C_KEY] [redacted:D_SECRET]"},
		{"repeated", map[string]string{"X_TOKEN": "tok-12345678"}, "tok-12345678 tok-12345678", "[redacted:X_TOKEN] [redacted:X_TOKEN]"},
		{"short ignored", map[string]string{"X_PASSWORD": "short12"}, "short12", "short12"},
		{"exactly min length", map[string]string{"X_PASSWORD": "12345678"}, "12345678", "[redacted:X_PASSWORD]"},
		{"non secret key ignored", map[string]string{"CLUSTER_NAME": "snowops-lab-1"}, "snowops-lab-1", "snowops-lab-1"},
		{"overlap longest wins", map[string]string{"A_TOKEN": "abcdefgh", "B_TOKEN": "abcdefghij"}, "abcdefghij", "[redacted:B_TOKEN]"},
		{"overlap shorter alone", map[string]string{"A_TOKEN": "abcdefgh", "B_TOKEN": "abcdefghij"}, "abcdefgh", "[redacted:A_TOKEN]"},
		{"lowercase key", map[string]string{"api_key": "lowerkey123"}, "lowerkey123", "[redacted:api_key]"},
		{"no secrets", nil, "plain line", "plain line"},
		{"empty input", map[string]string{"X_KEY": "keykeykey1"}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(tc.env).Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRegisterLater(t *testing.T) {
	r := New(nil)
	line := "token=late-secret-9"
	if got := r.Redact(line); got != line {
		t.Fatalf("before Register got %q", got)
	}
	r.Register("CONNECT_TOKEN", "late-secret-9")
	if got, want := r.Redact(line), "token=[redacted:CONNECT_TOKEN]"; got != want {
		t.Errorf("after Register got %q, want %q", got, want)
	}
	r.Register("OTHER", "short")
	r.Register("DUP", "late-secret-9")
	if got, want := r.Redact(line), "token=[redacted:CONNECT_TOKEN]"; got != want {
		t.Errorf("duplicate value renamed the mask: %q", got)
	}
}

func TestNilRedactor(t *testing.T) {
	var r *Redactor
	if got := r.Redact("x"); got != "x" {
		t.Errorf("nil Redactor changed input: %q", got)
	}
}

func TestIsSecretKey(t *testing.T) {
	cases := map[string]bool{"A_PASSWORD": true, "a_token": true, "X_KEY": true, "X_SECRET": true, "PASSWORD": false, "KEYS": false, "X_PASSWORD_FILE": false,
		"DB_PASS": true, "DB_PASSWD": true, "GCP_CREDENTIALS": true, "SENTRY_DSN": true, "SLACK_WEBHOOK": true,
		"ALERT_WEBHOOK_URL": true, "WEBHOOK_TARGET": true, "alert_webhook_url": true, "PASS": false, "BYPASS": false}
	for k, want := range cases {
		if got := IsSecretKey(k); got != want {
			t.Errorf("IsSecretKey(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	r := New(map[string]string{"BASE_TOKEN": "base-secret-1"})
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.Register(fmt.Sprintf("N%d_TOKEN", i), fmt.Sprintf("secret-value-%d", i))
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				if got := r.Redact("base-secret-1"); got != "[redacted:BASE_TOKEN]" {
					t.Errorf("got %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestProcessEnvMasksExportedSecrets(t *testing.T) {
	t.Setenv("PROBE_SHELL_TOKEN", "exported-secret-1")
	r := New(ProcessEnv())
	if got := r.Redact("x exported-secret-1 y"); got != "x [redacted:PROBE_SHELL_TOKEN] y" {
		t.Errorf("Redact = %q", got)
	}
}

func TestRedactMultiLineSecret(t *testing.T) {
	pem := "-----BEGIN KEY-----\nMIIEvQIBADANBgkq\nshort\nh7Jd92kLmN3pQrSt\n-----END KEY-----"
	r := New(map[string]string{"TLS_KEY": pem})
	tests := []struct{ name, in, want string }{
		{"whole value", pem, "[redacted:TLS_KEY]"},
		{"one body line", "x MIIEvQIBADANBgkq y", "x [redacted:TLS_KEY] y"},
		{"another body line", "h7Jd92kLmN3pQrSt", "[redacted:TLS_KEY]"},
		{"short line left alone", "short", "short"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	r2 := New(nil)
	r2.Register("SSH_KEY", "aaaaaaaa1\r\nbbbbbbbb2\r\n")
	if got := r2.Redact("bbbbbbbb2"); got != "[redacted:SSH_KEY]" {
		t.Errorf("CRLF line not masked: %q", got)
	}
}
