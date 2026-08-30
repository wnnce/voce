package engine

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var engineSessionMetrics = newSessionMetricSet()

type sessionMetricSet struct {
	active metric.Int64UpDownCounter
}

func newSessionMetricSet() sessionMetricSet {
	active, err := otel.Meter("github.com/wnnce/voce/internal/engine").Int64UpDownCounter(
		"sessions.active",
		metric.WithDescription("Number of active workflow sessions."),
	)
	if err != nil {
		panic(err)
	}
	return sessionMetricSet{active: active}
}

func (m sessionMetricSet) addActive(delta int64) {
	m.active.Add(context.Background(), delta)
}
