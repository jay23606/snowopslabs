package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// A request here takes well under a millisecond. With 5ms as the first bucket
// every request lands in it, and histogram_quantile can only interpolate inside
// it, so p50 and p99 read 2.5ms and 4.95ms whatever the real latency is.
func TestRequestDuration_ResolvesBelow5ms(t *testing.T) {
	httpRequestDuration.Reset()
	httpRequestDuration.WithLabelValues("GET", "/", "200", serviceName).Observe(0.00015)

	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{`le="0.0001"} 0`, `le="0.00025"} 1`, `le="0.001"} 1`, `le="0.005"} 1`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing bucket %q in /metrics output:\n%s", want, body)
		}
	}
}
