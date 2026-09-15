package machine

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	machinePoolMetricContext = context.Background()
	machinePoolMetrics       = newMachinePoolMetricSet()
)

type machinePoolMetricSet struct {
	connectionsActive metric.Int64UpDownCounter
	sessionsRouted    metric.Int64UpDownCounter
	bytesReceived     metric.Int64Counter
	bytesSent         metric.Int64Counter
	packetsReceived   metric.Int64Counter
	packetsSent       metric.Int64Counter
	writeErrors       metric.Int64Counter
}

func newMachinePoolMetricSet() machinePoolMetricSet {
	meter := otel.Meter("github.com/wnnce/voce/internal/machine/pool")
	return machinePoolMetricSet{
		connectionsActive: mustMachinePoolUpDownCounter(meter, "machine.pool.connections.active", "Number of active machine pool connections."),
		sessionsRouted: mustMachinePoolUpDownCounter(
			meter,
			"machine.pool.sessions.routed",
			"Number of sessions routed through machine pool connections.",
		),
		bytesReceived:   mustMachinePoolCounter(meter, "machine.pool.bytes.received", "Bytes received from gateway pool connections."),
		bytesSent:       mustMachinePoolCounter(meter, "machine.pool.bytes.sent", "Bytes sent to gateway pool connections."),
		packetsReceived: mustMachinePoolCounter(meter, "machine.pool.packets.received", "Packets received from gateway pool connections."),
		packetsSent:     mustMachinePoolCounter(meter, "machine.pool.packets.sent", "Packets sent to gateway pool connections."),
		writeErrors:     mustMachinePoolCounter(meter, "machine.pool.write.errors", "Machine pool connection write errors."),
	}
}

func mustMachinePoolUpDownCounter(meter metric.Meter, name, description string) metric.Int64UpDownCounter {
	counter, err := meter.Int64UpDownCounter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}

func mustMachinePoolCounter(meter metric.Meter, name, description string) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}
