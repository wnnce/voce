package schema

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/telemetry"
	"go.opentelemetry.io/otel"
)

func TestSchemaObjectMetrics(t *testing.T) {
	tel, err := telemetry.New(context.Background(), telemetry.Config{
		ServiceName: "schema-test",
		Environment: "test",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, tel.Shutdown(context.Background())) }()
	otel.SetMeterProvider(tel.MeterProvider())

	audio := NewAudio("audio", 16000, 1)
	sd := NewVideo("sd", 480, 640, 0)
	hd := NewVideo("hd", 720, 1280, 0)
	fhd := NewVideo("fhd", 1080, 1920, 0)

	output := scrapeMetrics(t, tel.Handler())
	assertMetricValue(t, output, "voce_schema_audio_objects_active", "1")
	assertMetricValue(t, output, "voce_schema_video_sd_objects_active", "1")
	assertMetricValue(t, output, "voce_schema_video_hd_objects_active", "1")
	assertMetricValue(t, output, "voce_schema_video_fhd_objects_active", "1")

	audio.Release()
	sd.Release()
	hd.Release()
	fhd.Release()

	output = scrapeMetrics(t, tel.Handler())
	assertMetricValue(t, output, "voce_schema_audio_objects_active", "0")
	assertMetricValue(t, output, "voce_schema_video_sd_objects_active", "0")
	assertMetricValue(t, output, "voce_schema_video_hd_objects_active", "0")
	assertMetricValue(t, output, "voce_schema_video_fhd_objects_active", "0")
}

func scrapeMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	body, err := io.ReadAll(response.Result().Body)
	require.NoError(t, err)
	return string(body)
}

func assertMetricValue(t *testing.T, output, name, value string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"{") {
			assert.Equal(t, value, strings.TrimSpace(line[strings.LastIndex(line, " ")+1:]))
			return
		}
	}
	t.Fatalf("metric %s not found in output", name)
}
