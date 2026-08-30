package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/telemetry"
	"go.opentelemetry.io/otel"
)

func TestGatewayPoolConnectionMetrics(t *testing.T) {
	tel, err := telemetry.New(context.Background(), telemetry.Config{
		ServiceName: "gateway-pool-test",
		Environment: "test",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, tel.Shutdown(context.Background())) }()
	otel.SetMeterProvider(tel.MeterProvider())

	p := newTestConnectionPool(4, 0)
	key := testSessionKey(1)
	p.Bind(key)

	output := scrapeGatewayPoolMetrics(t, tel.Handler())
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_connections_active", "1")
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_sessions_routed", "1")

	gatewayClientMetrics.connectionsActive.Add(gatewayClientMetricContext, 1)
	gatewayClientMetrics.bytesReceived.Add(gatewayClientMetricContext, 23)
	gatewayClientMetrics.packetsReceived.Add(gatewayClientMetricContext, 1)
	gatewayClientMetrics.bytesSent.Add(gatewayClientMetricContext, 31)
	gatewayClientMetrics.packetsSent.Add(gatewayClientMetricContext, 1)
	gatewayClientMetrics.writeErrors.Add(gatewayClientMetricContext, 2)
	output = scrapeGatewayPoolMetrics(t, tel.Handler())
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_connections_active", "1")
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_bytes_received_total", "23")
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_packets_received_total", "1")
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_bytes_sent_total", "31")
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_packets_sent_total", "1")
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_write_errors_total", "2")

	conn := p.queue[0].conn
	conn.state.Store(int32(protocol.ConnectionConnecting))
	p.OnConnectionClose(conn)
	output = scrapeGatewayPoolMetrics(t, tel.Handler())
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_connections_active", "0")
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_connections_connecting", "1")

	conn.state.Store(int32(protocol.ConnectionActive))
	p.OnConnectionOpen(conn)
	p.Unbind(key)
	p.Shutdown()
	output = scrapeGatewayPoolMetrics(t, tel.Handler())
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_connections_active", "0")
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_connections_connecting", "0")
	assertGatewayPoolMetric(t, output, "voce_gateway_pool_sessions_routed", "0")
	gatewayClientMetrics.connectionsActive.Add(gatewayClientMetricContext, -1)
	output = scrapeGatewayPoolMetrics(t, tel.Handler())
	assertGatewayPoolMetric(t, output, "voce_gateway_client_websocket_connections_active", "0")
}

func scrapeGatewayPoolMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	body, err := io.ReadAll(response.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func assertGatewayPoolMetric(t *testing.T, output, name, value string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{") {
			require.Equal(t, value, strings.TrimSpace(line[strings.LastIndex(line, " ")+1:]))
			return
		}
	}
	t.Fatalf("metric %s not found in output", name)
}
