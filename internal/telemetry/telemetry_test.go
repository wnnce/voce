package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTelemetryPrometheusHandler(t *testing.T) {
	tel, err := New(context.Background(), Config{
		ServiceName: "voce-test",
		Environment: "test",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, tel.Shutdown(context.Background())) }()

	counter, err := tel.Meter("test/module").Int64Counter("packets.received")
	require.NoError(t, err)
	counter.Add(context.Background(), 3)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()
	tel.Handler().ServeHTTP(res, req)

	body, err := io.ReadAll(res.Result().Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, string(body), "voce_packets_received_total{")
	require.Contains(t, string(body), "} 3")
	require.Contains(t, string(body), "service_name=\"voce-test\"")
	require.Contains(t, string(body), "otel_scope_name=\"go.opentelemetry.io/contrib/instrumentation/runtime\"")
	require.Contains(t, res.Header().Get("Content-Type"), "text/plain")
}
