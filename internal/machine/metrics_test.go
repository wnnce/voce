package machine

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/lxzan/gws"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/protocol"
	"github.com/wnnce/voce/internal/telemetry"
	"go.opentelemetry.io/otel"
)

func TestMachinePoolMetrics(t *testing.T) {
	tel, err := telemetry.New(context.Background(), telemetry.Config{
		ServiceName: "machine-pool-test",
		Environment: "test",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, tel.Shutdown(context.Background())) }()
	otel.SetMeterProvider(tel.MeterProvider())

	manager := newTestConnectionManager()
	conn := activeTestConnection()
	key := testSessionKey(1)
	before := scrapeMachinePoolMetrics(t, tel.Handler())
	activeBefore := machinePoolMetricValue(t, before, "voce_machine_pool_connections_active")
	routedBefore := machinePoolMetricValue(t, before, "voce_machine_pool_sessions_routed")
	bytesBefore := machinePoolMetricValue(t, before, "voce_machine_pool_bytes_received_total")
	packetsBefore := machinePoolMetricValue(t, before, "voce_machine_pool_packets_received_total")
	writeErrorsBefore := machinePoolMetricValue(t, before, "voce_machine_pool_write_errors_total")
	manager.Store(conn)
	manager.Select(key)
	packet := protocol.AcquirePacket()
	require.Error(t, conn.Write(key, packet))
	protocol.ReleasePacket(packet)
	afterWriteError := scrapeMachinePoolMetrics(t, tel.Handler())
	require.Equal(t, writeErrorsBefore+1, machinePoolMetricValue(t, afterWriteError, "voce_machine_pool_write_errors_total"))

	packet = protocol.AcquirePacket()
	packet.Type = protocol.TypeAudio
	packet.SetPayload([]byte("audio"))
	body := append(append([]byte{}, key[:]...), packet.Marshal()...)
	protocol.ReleasePacket(packet)
	conn.handle = func(protocol.SessionKey, *protocol.Packet) {}
	message := &gws.Message{Opcode: gws.OpcodeBinary, Data: bytes.NewBuffer(body)}
	conn.OnMessage(nil, message)

	afterMessage := scrapeMachinePoolMetrics(t, tel.Handler())
	require.Equal(t, bytesBefore+int64(len(body)), machinePoolMetricValue(t, afterMessage, "voce_machine_pool_bytes_received_total"))
	require.Equal(t, packetsBefore+1, machinePoolMetricValue(t, afterMessage, "voce_machine_pool_packets_received_total"))

	manager.Release(key)
	manager.Remove(conn)
	after := scrapeMachinePoolMetrics(t, tel.Handler())
	require.Equal(t, activeBefore, machinePoolMetricValue(t, after, "voce_machine_pool_connections_active"))
	require.Equal(t, routedBefore, machinePoolMetricValue(t, after, "voce_machine_pool_sessions_routed"))
}

func scrapeMachinePoolMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	body, err := io.ReadAll(response.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func machinePoolMetricValue(t *testing.T, output, name string) int64 {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{") {
			value := strings.TrimSpace(line[strings.LastIndex(line, " ")+1:])
			parsed, err := strconv.ParseInt(value, 10, 64)
			require.NoError(t, err)
			return parsed
		}
	}
	return 0
}
