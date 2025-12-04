package logging

import (
	"fmt"
	"log/slog"
)

// Error returns a slog attribute for errors.
func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

// FHIRServer returns a slog attribute for FHIR server URLs.
func FHIRServer(url string) slog.Attr {
	return slog.String("fhir_server", url)
}

// TypeOf returns a slog attribute with the type name of the given value.
func TypeOf(key string, v any) slog.Attr {
	return slog.String(key, fmt.Sprintf("%T", v))
}

// Component returns a slog attribute for a component type.
func Component(v any) slog.Attr {
	return TypeOf("component", v)
}
