// Package telemetry provides the process-level OpenTelemetry metrics setup.
package telemetry

import (
	"context"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Config identifies the process that owns the exported metrics.
type Config struct {
	ServiceName string
	Environment string
}

// Telemetry owns the process-level meter provider and Prometheus endpoint.
//
// The registry is intentionally instance-local. This keeps multiple application
// instances and tests isolated from the process-global Prometheus registry.
type Telemetry struct {
	provider *sdkmetric.MeterProvider
	handler  http.Handler

	shutdownOnce sync.Once
	shutdownErr  error
}

// New initializes an OpenTelemetry meter provider backed by a pull-based
// Prometheus exporter.
func New(ctx context.Context, cfg Config) (*Telemetry, error) {
	registry := prometheus.NewRegistry()
	exporter, err := otelprom.New(
		otelprom.WithRegisterer(registry),
		otelprom.WithNamespace("voce"),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("deployment.environment.name", cfg.Environment),
	))
	if err != nil {
		return nil, err
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)
	if err := otelruntime.Start(otelruntime.WithMeterProvider(provider)); err != nil {
		_ = provider.Shutdown(ctx)
		return nil, err
	}

	return &Telemetry{
		provider: provider,
		handler:  promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}, nil
}

// Meter returns a meter for a module or instrumentation scope.
func (t *Telemetry) Meter(name string, options ...metric.MeterOption) metric.Meter {
	return t.provider.Meter(name, options...)
}

// MeterProvider returns the configured provider for application wiring.
func (t *Telemetry) MeterProvider() metric.MeterProvider {
	return t.provider
}

// Handler returns the standard Prometheus exposition handler.
func (t *Telemetry) Handler() http.Handler {
	return t.handler
}

// Shutdown releases the meter provider. It is safe to call more than once.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	t.shutdownOnce.Do(func() {
		t.shutdownErr = t.provider.Shutdown(ctx)
	})
	return t.shutdownErr
}
