package fhirutil

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestRedactClientError(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.NoError(t, RedactClientError(nil))
	})
	t.Run("request URLs keep their path and query parameter names", func(t *testing.T) {
		err := fmt.Errorf("FHIR request failed (GET https://pip.example/fhir/Patient/abc-1?_elements=identifier, status=500)")
		assert.EqualError(t, RedactClientError(err), "FHIR request failed (GET https://pip.example/fhir/Patient/abc-1?_elements=[redacted], status=500)")
	})
	t.Run("quoted URL of a transport error", func(t *testing.T) {
		err := fmt.Errorf("FHIR request failed (GET http://nvi.example/List?subject%%3Aidentifier=token-abc): %w",
			errors.New(`Get "http://nvi.example/List?subject%3Aidentifier=token-abc": dial tcp: connection refused`))
		assert.EqualError(t, RedactClientError(err), `FHIR request failed (GET http://nvi.example/List?subject%3Aidentifier=[redacted]): `+
			`Get "http://nvi.example/List?subject%3Aidentifier=[redacted]": dial tcp: connection refused`)
	})
	t.Run("upper-case scheme, user info", func(t *testing.T) {
		err := errors.New("FHIR request failed (GET HTTPS://user:secret@pip.example/fhir/Patient/abc-1)")
		redacted := RedactClientError(err).Error()
		assert.NotContains(t, redacted, "secret")
		assert.Contains(t, redacted, "pip.example/fhir/Patient/abc-1)")
	})
	t.Run("a BSN in a path is redacted", func(t *testing.T) {
		err := errors.New("FHIR request failed (GET https://pip.example/fhir/Patient/123456782, status=404)")
		assert.EqualError(t, RedactClientError(err), "FHIR request failed (GET https://pip.example/fhir/Patient/[redacted BSN], status=404)")
	})
	outcome := fhirclient.OperationOutcomeError{
		OperationOutcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{{
			Severity:    fhir.IssueSeverityError,
			Code:        fhir.IssueTypeInvalid,
			Diagnostics: to.Ptr("unknown patient 123456782 in https://nvi.example/List?subject:identifier=x"),
		}}},
		HttpStatusCode: http.StatusBadRequest,
	}
	t.Run("OperationOutcome diagnostics and status are kept, with BSNs and query values redacted", func(t *testing.T) {
		redacted := RedactClientError(fmt.Errorf("register: %w", outcome))

		assert.EqualError(t, redacted, "register: OperationOutcome (status=400), issues: [invalid error] "+
			"unknown patient [redacted BSN] in https://nvi.example/List?subject:identifier=[redacted]")
		var unwrapped fhirclient.OperationOutcomeError
		require.ErrorAs(t, redacted, &unwrapped)
		assert.Equal(t, http.StatusBadRequest, unwrapped.HttpStatusCode)
	})
	t.Run("sent values are redacted wherever they are echoed, as is or URL-encoded", func(t *testing.T) {
		const token = "http://example.org/token|tok-abcdef"
		echo := fhirclient.OperationOutcomeError{
			OperationOutcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{{
				Severity:    fhir.IssueSeverityError,
				Code:        fhir.IssueTypeInvalid,
				Diagnostics: to.Ptr("Invalid search: subject%3Aidentifier=" + url.QueryEscape(token) + " (" + token + "), token tok-abcdef"),
			}}},
			HttpStatusCode: http.StatusBadRequest,
		}

		redacted := RedactClientError(fmt.Errorf("search: %w", echo), token, "tok-abcdef", "short", "")

		assert.EqualError(t, redacted, "search: OperationOutcome (status=400), issues: [invalid error] "+
			"Invalid search: subject%3Aidentifier=[redacted] ([redacted]), token [redacted]")
	})
	t.Run("sent values shorter than the minimum are left alone", func(t *testing.T) {
		assert.EqualError(t, RedactClientError(errors.New("status=400 for ura|1"), "1"), "status=400 for ura|1")
	})
	t.Run("OperationOutcome wrapped in a fhirapi.Error", func(t *testing.T) {
		err := &fhirapi.Error{Message: "Failed to read", Cause: outcome}

		redacted := RedactClientError(err).Error()

		assert.Contains(t, redacted, "Failed to read: OperationOutcome (status=400), issues: [invalid error] unknown patient [redacted BSN]")
		assert.NotContains(t, redacted, "123456782")
	})
}
