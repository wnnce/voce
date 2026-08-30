package realtime

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	realtimeMetricContext = context.Background()
	realtimeMetrics       = newRealtimeMetricSet()
)

type realtimeMetricSet struct {
	websocketBytesReceived metric.Int64Counter
	websocketBytesSent     metric.Int64Counter
	grpcBytesReceived      metric.Int64Counter
	grpcBytesSent          metric.Int64Counter
}

func newRealtimeMetricSet() realtimeMetricSet {
	meter := otel.Meter("github.com/wnnce/voce/biz/realtime")
	return realtimeMetricSet{
		websocketBytesReceived: mustCounter(
			meter,
			"realtime.websocket.bytes.received",
			"Bytes received from WebSocket clients.",
		),
		websocketBytesSent: mustCounter(
			meter,
			"realtime.websocket.bytes.sent",
			"Bytes accounted for WebSocket client writes.",
		),
		grpcBytesReceived: mustCounter(
			meter,
			"realtime.grpc.bytes.received",
			"Audio payload bytes received from gRPC clients.",
		),
		grpcBytesSent: mustCounter(
			meter,
			"realtime.grpc.bytes.sent",
			"Payload bytes sent to gRPC clients.",
		),
	}
}

func mustCounter(meter metric.Meter, name, description string) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}
