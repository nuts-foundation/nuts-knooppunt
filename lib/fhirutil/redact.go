package fhirutil

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
)

// urlPattern matches the URLs that fhirclient and net/http errors embed, e.g.
// "GET https://nvi.example/List?subject%3Aidentifier=...", capturing scheme and host. User info
// (credentials) is matched but not captured, so it's dropped too.
var urlPattern = regexp.MustCompile(`(?i)(https?://)(?:[^/\s"?#@]*@)?([^/\s"?#]+)[/?#][^\s"]*`)

// RedactClientError returns err with the parts a FHIR client error may carry that can contain
// patient data removed from its message:
//   - the path and query of every request URL (resource ids such as Patient/<id>, search
//     parameters such as identifiers); scheme and host are kept, and
//   - the diagnostics of an upstream OperationOutcome, which the server controls and which may
//     echo request values. Only each issue's code and severity are kept.
//
// The returned error still unwraps to err, so errors.As/errors.Is keep working (e.g. to read the
// upstream HTTP status from a fhirclient.OperationOutcomeError). Its own message must therefore
// be the only one that is ever printed: wrap it with %w as usual, but never log the unwrapped
// original. Returns nil for a nil err.
func RedactClientError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	var outcome fhirclient.OperationOutcomeError
	if errors.As(err, &outcome) {
		message = strings.ReplaceAll(message, outcome.Error(), redactedOperationOutcome(outcome))
	}
	message = urlPattern.ReplaceAllString(message, "$1$2/[redacted]")
	return &redactedError{message: message, cause: err}
}

func redactedOperationOutcome(outcome fhirclient.OperationOutcomeError) string {
	issues := make([]string, 0, len(outcome.Issue))
	for _, issue := range outcome.Issue {
		issues = append(issues, fmt.Sprintf("[%v %v]", issue.Code, issue.Severity))
	}
	return fmt.Sprintf("OperationOutcome (status=%d), issues: %s", outcome.HttpStatusCode, strings.Join(issues, "; "))
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
