package schema

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	metricsContext = context.Background()
	schemaMetrics  = newMetricSet()
)

type metricSet struct {
	audioActive    metric.Int64UpDownCounter
	videoSDActive  metric.Int64UpDownCounter
	videoHDActive  metric.Int64UpDownCounter
	videoFHDActive metric.Int64UpDownCounter
}

func newMetricSet() metricSet {
	meter := otel.Meter("github.com/wnnce/voce/internal/schema")
	return metricSet{
		audioActive: mustUpDownCounter(
			meter,
			"schema.audio.objects.active",
			"Number of active audio objects.",
		),
		videoSDActive: mustUpDownCounter(
			meter,
			"schema.video.sd.objects.active",
			"Number of active SD video objects.",
		),
		videoHDActive: mustUpDownCounter(
			meter,
			"schema.video.hd.objects.active",
			"Number of active HD video objects.",
		),
		videoFHDActive: mustUpDownCounter(
			meter,
			"schema.video.fhd.objects.active",
			"Number of active FHD video objects.",
		),
	}
}

func mustUpDownCounter(meter metric.Meter, name, description string) metric.Int64UpDownCounter {
	counter, err := meter.Int64UpDownCounter(name, metric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return counter
}
