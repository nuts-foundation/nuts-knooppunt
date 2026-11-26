package fhirutil

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

// urlPattern matches the URLs that fhirclient and net/http errors embed, e.g.
// "GET https://nvi.example/List?subject%3Aidentifier=..." or `Get "https://...": dial tcp`.
var urlPattern = regexp.MustCompile(`(?i)https?://[^\s"]+`)

// RedactClientError returns err with the patient data a FHIR client error may carry removed from
// its message, while keeping what an operator needs to find out what went wrong:
//   - every request URL is redacted with logging.RedactURL: the user info and the query values
//     are removed, and the path (resource type and id) and the query parameter names are kept;
//   - the diagnostics of an upstream OperationOutcome are kept, together with the HTTP status;
//   - every occurrence of sentValues, as is or URL-encoded, is replaced by logging.RedactedValue.
//     These are patient identifiers the request carried that the server may echo in its
//     diagnostics, such as a transport token. Values shorter than minSentValueLength are skipped,
//     so a short value can't mangle the rest of the message;
//   - every BSN left anywhere in the message, e.g. echoed in those diagnostics, is redacted with
//     bsnutil.Redact.
//
// The returned error still unwraps to err, so errors.As/errors.Is keep working (e.g. to read the
// upstream HTTP status from a fhirclient.OperationOutcomeError). Its own message must therefore
// be the only one that is ever printed: wrap it with %w as usual, but never log the unwrapped
// original. Returns nil for a nil err.
func RedactClientError(err error, sentValues ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	var outcome fhirclient.OperationOutcomeError
	if errors.As(err, &outcome) {
		message = strings.ReplaceAll(message, outcome.Error(), operationOutcomeWithStatus(outcome))
	}
	message = redactSentValues(message, sentValues)
	message = urlPattern.ReplaceAllStringFunc(message, redactURL)
	message = bsnutil.Redact(message)
	return &redactedError{message: message, cause: err}
}

// minSentValueLength is the length below which RedactClientError doesn't redact a sent value.
const minSentValueLength = 6

func redactSentValues(message string, sentValues []string) string {
	for _, value := range sentValues {
		if len(value) < minSentValueLength {
			continue
		}
		for _, form := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
			message = strings.ReplaceAll(message, form, logging.RedactedValue)
		}
	}
	return message
}

// operationOutcomeWithStatus is the message of outcome, which fhirclient formats without the HTTP
// status, with the status added.
func operationOutcomeWithStatus(outcome fhirclient.OperationOutcomeError) string {
	return strings.Replace(outcome.Error(), "OperationOutcome,", fmt.Sprintf("OperationOutcome (status=%d),", outcome.HttpStatusCode), 1)
}

// redactURL redacts a URL urlPattern matched. Punctuation that ends the surrounding message, such
// as the "," in "(GET <url>, status=500)", is kept outside the URL.
func redactURL(match string) string {
	trimmed := strings.TrimRight(match, ",;:)")
	suffix := match[len(trimmed):]
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "[redacted URL]" + suffix
	}
	return logging.RedactURL(parsed) + suffix
}

type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string {
	return e.message
}

func (e *redactedError) Unwrap() error {
	return e.cause
}
