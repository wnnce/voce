package realtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/telemetry"
	"go.opentelemetry.io/otel"
)

func TestRealtimeTrafficMetricsByTransport(t *testing.T) {
	tel, err := telemetry.New(context.Background(), telemetry.Config{
		ServiceName: "realtime-test",
		Environment: "test",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, tel.Shutdown(context.Background())) }()
	otel.SetMeterProvider(tel.MeterProvider())

	realtimeMetrics.websocketBytesReceived.Add(realtimeMetricContext, 11)
	realtimeMetrics.websocketBytesSent.Add(realtimeMetricContext, 13)
	realtimeMetrics.grpcBytesReceived.Add(realtimeMetricContext, 17)
	realtimeMetrics.grpcBytesSent.Add(realtimeMetricContext, 19)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()
	tel.Handler().ServeHTTP(res, req)
	body, err := io.ReadAll(res.Result().Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.Code)

	output := string(body)
	require.Contains(t, output, "voce_realtime_websocket_bytes_received")
	require.Contains(t, output, "} 11")
	require.Contains(t, output, "voce_realtime_websocket_bytes_sent")
	require.Contains(t, output, "} 13")
	require.Contains(t, output, "voce_realtime_grpc_bytes_received")
	require.Contains(t, output, "} 17")
	require.Contains(t, output, "voce_realtime_grpc_bytes_sent")
	require.Contains(t, output, "} 19")
}
