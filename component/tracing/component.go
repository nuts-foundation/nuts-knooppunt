package tracing

import (
	"context"
	"errors"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var _ component.Lifecycle = (*Component)(nil)

type Config struct {
	OTLPEndpoint   string `koanf:"otlpendpoint"`
	Insecure       bool   `koanf:"insecure"`
	ServiceName    string `koanf:"servicename"`
	ServiceVersion string
}

func DefaultConfig() Config {
	return Config{
		Insecure:    true,
		ServiceName: "nuts-knooppunt",
	}
}

type Component struct {
	config         Config
	tracerProvider *trace.TracerProvider
	shutdownFuncs  []func(context.Context) error
}

func New(cfg Config) *Component {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "nuts-knooppunt"
	}
	return &Component{config: cfg}
}

func (c *Component) Start() error {
	if c.config.OTLPEndpoint == "" {
		log.Info().Msg("No OTLP endpoint configured, tracing disabled")
		return nil
	}

	ctx := context.Background()

	// Set up propagator (W3C Trace Context + Baggage)
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(prop)

	// Set up resource with service info
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(c.config.ServiceName),
			semconv.ServiceVersionKey.String(c.config.ServiceVersion),
		),
	)
	if err != nil {
		return err
	}

	// Set up OTLP HTTP exporter
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(c.config.OTLPEndpoint),
	}
	if c.config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	traceExporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return err
	}
	c.shutdownFuncs = append(c.shutdownFuncs, traceExporter.Shutdown)

	// Set up trace provider with batch exporter
	c.tracerProvider = trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)
	c.shutdownFuncs = append(c.shutdownFuncs, c.tracerProvider.Shutdown)
	otel.SetTracerProvider(c.tracerProvider)

	log.Info().
		Str("endpoint", c.config.OTLPEndpoint).
		Str("service", c.config.ServiceName).
		Msg("OpenTelemetry tracing initialized")

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	if len(c.shutdownFuncs) == 0 {
		return nil
	}

	log.Info().Msg("Shutting down OpenTelemetry tracing")

	var errs error
	for _, fn := range c.shutdownFuncs {
		if err := fn(ctx); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	c.shutdownFuncs = nil
	return errs
}

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	// Tracing component doesn't expose HTTP endpoints
}

// WrapTransport wraps an http.RoundTripper with OpenTelemetry instrumentation.
// If transport is nil, http.DefaultTransport is used.
// This wrapper centralizes tracing configuration for outbound HTTP calls,
// allowing future additions like custom options or sampling without changing callers.
func WrapTransport(transport http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(transport)
}

// NewHTTPClient creates an http.Client with OpenTelemetry instrumentation.
// This wrapper centralizes tracing configuration for outbound HTTP calls,
// allowing future additions like custom options or sampling without changing callers.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(nil),
	}
}
