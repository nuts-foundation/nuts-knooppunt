package tracing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
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
	tracerProvider *sdktrace.TracerProvider
	loggerProvider *log.LoggerProvider
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
		slog.Info("No OTLP endpoint configured, tracing disabled")
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
	c.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	c.shutdownFuncs = append(c.shutdownFuncs, c.tracerProvider.Shutdown)
	otel.SetTracerProvider(c.tracerProvider)

	// Set up OTLP log exporter
	logOpts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(c.config.OTLPEndpoint),
	}
	if c.config.Insecure {
		logOpts = append(logOpts, otlploghttp.WithInsecure())
	}
	logExporter, err := otlploghttp.New(ctx, logOpts...)
	if err != nil {
		return err
	}
	c.shutdownFuncs = append(c.shutdownFuncs, logExporter.Shutdown)

	// Set up log provider with batch processor
	c.loggerProvider = log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(res),
	)
	c.shutdownFuncs = append(c.shutdownFuncs, c.loggerProvider.Shutdown)

	// Configure slog to use otelslog bridge, sending logs via OTLP.
	// When OTLP is enabled, all logs go to the collector for centralized viewing.
	slogHandler := otelslog.NewHandler(c.config.ServiceName,
		otelslog.WithLoggerProvider(c.loggerProvider),
	)
	slog.SetDefault(slog.New(slogHandler))

	// Print a message to stdout to inform users where to find logs.
	fmt.Printf("OpenTelemetry enabled: logs and traces are sent to %s\n", c.config.OTLPEndpoint)
	fmt.Println("View application logs and traces in your OTLP collector (e.g. at http://localhost:18888)")

	slog.Info("OpenTelemetry tracing and logging initialized",
		slog.String("endpoint", c.config.OTLPEndpoint),
		slog.String("service", c.config.ServiceName))

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	if len(c.shutdownFuncs) == 0 {
		return nil
	}

	slog.Info("Shutting down OpenTelemetry tracing")

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

// TransportOption configures the instrumentation added by WrapTransport and NewHTTPClient.
type TransportOption func(*urlRedactingTransport)

// WithoutURLPath makes client spans record only the scheme and host of the request URL, for
// clients whose request paths identify a patient (e.g. Patient/<id>).
func WithoutURLPath() TransportOption {
	return func(t *urlRedactingTransport) {
		t.dropPath = true
	}
}

// WrapTransport wraps an http.RoundTripper with OpenTelemetry instrumentation.
// If transport is nil, http.DefaultTransport is used.
// This wrapper centralizes tracing configuration for outbound HTTP calls. The client spans it
// creates record the request URL without its query (see urlRedactingTransport).
func WrapTransport(transport http.RoundTripper, opts ...TransportOption) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	redacting := &urlRedactingTransport{next: transport}
	for _, opt := range opts {
		opt(redacting)
	}
	return otelhttp.NewTransport(redacting)
}

// NewHTTPClient creates an http.Client with OpenTelemetry instrumentation, see WrapTransport.
func NewHTTPClient(opts ...TransportOption) *http.Client {
	return &http.Client{
		Transport: WrapTransport(nil, opts...),
	}
}

// urlRedactingTransport overwrites the url.full attribute that otelhttp puts on the client span
// with the URL without its query: query parameters carry search values such as patient
// identifiers. otelhttp sets the attribute before calling this transport with the span's
// context, and setting an attribute again replaces it.
type urlRedactingTransport struct {
	next     http.RoundTripper
	dropPath bool
}

func (t *urlRedactingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	fullURL := logging.RedactURL(request.URL)
	if t.dropPath {
		fullURL = (&url.URL{Scheme: request.URL.Scheme, Host: request.URL.Host}).String()
	}
	trace.SpanFromContext(request.Context()).SetAttributes(attribute.String("url.full", fullURL))
	return t.next.RoundTrip(request)
}
