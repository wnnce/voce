package gateway

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var gatewaySessionMetrics = newGatewaySessionMetricSet()

type gatewaySessionMetricSet struct {
	active metric.Int64UpDownCounter
}

func newGatewaySessionMetricSet() gatewaySessionMetricSet {
	active, err := otel.Meter("github.com/wnnce/voce/internal/gateway").Int64UpDownCounter(
		"sessions.active",
		metric.WithDescription("Number of active gateway sessions."),
	)
	if err != nil {
		panic(err)
	}
	return gatewaySessionMetricSet{active: active}
}

func (m gatewaySessionMetricSet) addActive(delta int64) {
	m.active.Add(context.Background(), delta)
}

var (
	gatewayClientMetricContext = context.Background()
	gatewayClientMetrics       = newGatewayClientMetricSet()
)

type gatewayClientMetricSet struct {
	connectionsActive metric.Int64UpDownCounter
	bytesReceived     metric.Int64Counter
	bytesSent         metric.Int64Counter
	packetsReceived   metric.Int64Counter
	packetsSent       metric.Int64Counter
	writeErrors       metric.Int64Counter
}

func newGatewayClientMetricSet() gatewayClientMetricSet {
	meter := otel.Meter("github.com/wnnce/voce/internal/gateway/client")
	return gatewayClientMetricSet{
		connectionsActive: mustGatewayClientUpDownCounter(
			meter, "gateway.client.websocket.connections.active",
			"Number of active client WebSocket connections at the gateway.",
		),
		bytesReceived: mustGatewayClientCounter(
			meter, "gateway.client.websocket.bytes.received",
			"Bytes received from client WebSocket connections.",
		),
		bytesSent: mustGatewayClientCounter(
			meter, "gateway.client.websocket.bytes.sent",
			"Bytes sent to client WebSocket connections.",
		),
		packetsReceived: mustGatewayClientCounter(
			meter, "gateway.client.websocket.packets.received",
			"Packets received from client WebSocket connections.",
		),
		packetsSent: mustGatewayClientCounter(
			meter, "gateway.client.websocket.packets.sent",
			"Packets sent to client WebSocket connections.",
		),
		writeErrors: mustGatewayClientCounter(
			meter, "gateway.client.websocket.write.errors",
			"Client WebSocket write errors at the gateway.",
		),
	}
}

func mustGatewayClientUpDownCounter(meter metric.Meter, name, description string) metric.Int64UpDownCounter {
	counter, err := meter.Int64UpDownCounter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}

func mustGatewayClientCounter(meter metric.Meter, name, description string) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}

var (
	gatewayPoolMetricContext = context.Background()
	gatewayPoolMetrics       = newGatewayPoolMetricSet()
)

type gatewayPoolMetricSet struct {
	connectionsActive     metric.Int64UpDownCounter
	connectionsConnecting metric.Int64UpDownCounter
	pendingDials          metric.Int64UpDownCounter
	sessionsRouted        metric.Int64UpDownCounter
	dials                 metric.Int64Counter
	reconnects            metric.Int64Counter
	bytesReceived         metric.Int64Counter
	bytesSent             metric.Int64Counter
	packetsReceived       metric.Int64Counter
	packetsSent           metric.Int64Counter
	writeErrors           metric.Int64Counter
}

func newGatewayPoolMetricSet() gatewayPoolMetricSet {
	meter := otel.Meter("github.com/wnnce/voce/internal/gateway/pool")
	return gatewayPoolMetricSet{
		connectionsActive: mustGatewayPoolUpDownCounter(
			meter, "gateway.pool.connections.active", "Number of active gateway pool connections.",
		),
		connectionsConnecting: mustGatewayPoolUpDownCounter(
			meter, "gateway.pool.connections.connecting", "Number of gateway pool connections connecting or reconnecting.",
		),
		pendingDials: mustGatewayPoolUpDownCounter(
			meter, "gateway.pool.pending.dials", "Number of gateway pool dials currently being created.",
		),
		sessionsRouted: mustGatewayPoolUpDownCounter(
			meter, "gateway.pool.sessions.routed", "Number of sessions currently routed through gateway pool connections.",
		),
		dials: mustGatewayPoolCounter(meter, "gateway.pool.dials", "Number of gateway pool dial attempts."),
		reconnects: mustGatewayPoolCounter(
			meter, "gateway.pool.reconnects", "Number of gateway pool reconnects after an active connection closes.",
		),
		bytesReceived:   mustGatewayPoolCounter(meter, "gateway.pool.bytes.received", "Bytes received from machine pool connections."),
		bytesSent:       mustGatewayPoolCounter(meter, "gateway.pool.bytes.sent", "Bytes sent to machine pool connections."),
		packetsReceived: mustGatewayPoolCounter(meter, "gateway.pool.packets.received", "Packets received from machine pool connections."),
		packetsSent:     mustGatewayPoolCounter(meter, "gateway.pool.packets.sent", "Packets sent to machine pool connections."),
		writeErrors:     mustGatewayPoolCounter(meter, "gateway.pool.write.errors", "Gateway pool connection write errors."),
	}
}

func mustGatewayPoolUpDownCounter(meter metric.Meter, name, description string) metric.Int64UpDownCounter {
	counter, err := meter.Int64UpDownCounter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}

func mustGatewayPoolCounter(meter metric.Meter, name, description string) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}
