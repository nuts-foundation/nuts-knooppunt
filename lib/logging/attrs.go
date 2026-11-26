package logging

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
)

// Error returns a slog attribute for errors.
func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

// FHIRServer returns a slog attribute for FHIR server URLs.
func FHIRServer(url string) slog.Attr {
	return slog.String("fhir_server", url)
}

// RedactedValue replaces each query parameter value that RedactURL removes.
const RedactedValue = "[redacted]"

// RedactURL returns u for logging, without user info and fragment, with every query parameter
// value replaced by RedactedValue and with any BSN in the path redacted (see bsnutil.Redact). The
// path, e.g. a resource type and id, is kept.
// Query values of FHIR requests carry search values such as patient identifiers. The parameter
// names are kept, so the log still shows which search was made.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	withoutQuery := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}
	// Redacted after encoding, so the placeholder isn't percent-encoded.
	redacted := bsnutil.Redact(withoutQuery.String())
	if u.RawQuery != "" {
		redacted += "?" + redactQueryValues(u.RawQuery)
	}
	return redacted
}

// redactQueryValues replaces the value of every parameter in rawQuery with RedactedValue, keeping
// the (still encoded) names in their order. A parameter without '=' may itself be a value, so it
// is replaced as a whole.
func redactQueryValues(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	params := strings.Split(rawQuery, "&")
	for i, param := range params {
		if name, _, hasValue := strings.Cut(param, "="); hasValue {
			params[i] = name + "=" + RedactedValue
		} else if param != "" {
			params[i] = RedactedValue
		}
	}
	return strings.Join(params, "&")
}

// TypeOf returns a slog attribute with the type name of the given value.
func TypeOf(key string, v any) slog.Attr {
	return slog.String(key, fmt.Sprintf("%T", v))
}

// Component returns a slog attribute for a component type.
func Component(v any) slog.Attr {
	return TypeOf("component", v)
}
