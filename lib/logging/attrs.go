package logging

import (
	"fmt"
	"log/slog"
	"net/url"
)

// Error returns a slog attribute for errors.
func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

// FHIRServer returns a slog attribute for FHIR server URLs.
func FHIRServer(url string) slog.Attr {
	return slog.String("fhir_server", url)
}

// RedactURL returns u without its user info, query and fragment, for logging: queries of FHIR
// requests carry search values such as patient identifiers.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	redacted := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}
	if u.RawQuery != "" {
		redacted.RawQuery = "[redacted]"
	}
	return redacted.String()
}

// TypeOf returns a slog attribute with the type name of the given value.
func TypeOf(key string, v any) slog.Attr {
	return slog.String(key, fmt.Sprintf("%T", v))
}

// Component returns a slog attribute for a component type.
func Component(v any) slog.Attr {
	return TypeOf("component", v)
}
