package fhirutil

import (
	"errors"
	"fmt"
	"net/http"
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
	t.Run("request URLs keep only scheme and host", func(t *testing.T) {
		err := fmt.Errorf("FHIR request failed (GET https://pip.example/fhir/Patient/123?_elements=identifier, status=500)")
		assert.EqualError(t, RedactClientError(err), "FHIR request failed (GET https://pip.example/[redacted] status=500)")
	})
	t.Run("quoted URL of a transport error", func(t *testing.T) {
		err := fmt.Errorf("FHIR request failed (GET http://nvi.example/List?subject%%3Aidentifier=token): %w",
			errors.New(`Get "http://nvi.example/List?subject%3Aidentifier=token": dial tcp: connection refused`))
		redacted := RedactClientError(err).Error()
		assert.NotContains(t, redacted, "token")
		assert.NotContains(t, redacted, "List")
		assert.Contains(t, redacted, "connection refused")
	})
	t.Run("upper-case scheme, user info", func(t *testing.T) {
		err := errors.New("FHIR request failed (GET HTTPS://user:secret@pip.example/fhir/Patient/123)")
		assert.EqualError(t, RedactClientError(err), "FHIR request failed (GET HTTPS://pip.example/[redacted]")
	})
	t.Run("OperationOutcome diagnostics are dropped, codes and status kept", func(t *testing.T) {
		outcome := fhirclient.OperationOutcomeError{
			OperationOutcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{{
				Severity:    fhir.IssueSeverityError,
				Code:        fhir.IssueTypeInvalid,
				Diagnostics: to.Ptr("unknown patient 123456782"),
			}}},
			HttpStatusCode: http.StatusBadRequest,
		}
		redacted := RedactClientError(fmt.Errorf("register: %w", outcome))

		assert.EqualError(t, redacted, "register: OperationOutcome (status=400), issues: [invalid error]")
		var unwrapped fhirclient.OperationOutcomeError
		require.ErrorAs(t, redacted, &unwrapped)
		assert.Equal(t, http.StatusBadRequest, unwrapped.HttpStatusCode)
	})
	t.Run("OperationOutcome wrapped in a fhirapi.Error", func(t *testing.T) {
		outcome := fhirclient.OperationOutcomeError{
			OperationOutcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{{
				Severity:    fhir.IssueSeverityError,
				Code:        fhir.IssueTypeInvalid,
				Diagnostics: to.Ptr("unknown patient 123456782"),
			}}},
			HttpStatusCode: http.StatusBadRequest,
		}
		err := &fhirapi.Error{Message: "Failed to read", Cause: outcome}

		assert.EqualError(t, RedactClientError(err), "Failed to read: OperationOutcome (status=400), issues: [invalid error]")
	})
}
