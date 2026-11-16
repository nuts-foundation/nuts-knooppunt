package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWrapTransport_RedactsURLInSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	originalProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(originalProvider) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(server.Close)

	// get does one request through client and returns the url.full attribute of its span.
	get := func(t *testing.T, client *http.Client, path string) string {
		t.Helper()
		recorder.Reset()
		response, err := client.Get(server.URL + path)
		require.NoError(t, err)
		_ = response.Body.Close()
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		var urlFull string
		for _, attr := range spans[0].Attributes() {
			assert.NotContains(t, attr.Value.Emit(), "900186021", "attribute %s", attr.Key)
			if attr.Key == "url.full" {
				urlFull = attr.Value.AsString()
			}
		}
		return urlFull
	}

	t.Run("query is redacted", func(t *testing.T) {
		urlFull := get(t, NewHTTPClient(), "/fhir/List?subject:identifier=system|900186021")
		assert.Equal(t, server.URL+"/fhir/List?[redacted]", urlFull)
	})
	t.Run("path is left out with WithoutURLPath", func(t *testing.T) {
		urlFull := get(t, NewHTTPClient(WithoutURLPath()), "/fhir/Patient/900186021?_elements=identifier")
		assert.Equal(t, server.URL, urlFull)
	})
}
