package logging

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// Init initializes the global slog logger with JSON output and trace context injection.
// Default log level is Debug to match previous zerolog configuration.
func Init() {
	handler := &ContextHandler{
		Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	}
	slog.SetDefault(slog.New(handler))
}

// ContextHandler wraps a slog.Handler and automatically adds trace context to log records.
// When a valid OpenTelemetry span is present in the context, trace_id and span_id are added.
type ContextHandler struct {
	slog.Handler
}

// Handle adds trace context attributes to the record before delegating to the wrapped handler.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new handler with the given group name.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}
