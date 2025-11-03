package mitz

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := Config{
			MitzBase:      "http://example.com",
			GatewaySystem: "urn:oid:2.16.840.1.113883.2.4.6.6.1",
			SourceSystem:  "urn:oid:2.16.840.1.113883.2.4.6.6.90000017",
		}

		component, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, component)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.1", component.gatewaySystem)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.90000017", component.sourceSystem)
	})

	t.Run("missing mitzbase", func(t *testing.T) {
		config := Config{}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "mitzbase must be configured")
	})

	t.Run("invalid mitzbase", func(t *testing.T) {
		config := Config{
			MitzBase: "://invalid-url",
		}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "invalid mitzbase URL")
	})
}

func TestRegisterHttpHandlers(t *testing.T) {
	config := Config{
		MitzBase: "http://example.com",
	}
	component, err := New(config)
	require.NoError(t, err)

	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()

	component.RegisterHttpHandlers(publicMux, internalMux)

	// Test that notify handler is registered on publicMux
	bundle := fhir.Bundle{Type: fhir.BundleTypeTransaction}
	body, _ := json.Marshal(bundle)
	req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/fhir+json")
	w := httptest.NewRecorder()
	publicMux.ServeHTTP(w, req)
	// Should not be 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestComponentLifecycle(t *testing.T) {
	component := &Component{}

	err := component.Start()
	assert.NoError(t, err)

	err = component.Stop(context.Background())
	assert.NoError(t, err)
}

func TestCheckConsent(t *testing.T) {
	t.Run("missing consent check endpoint", func(t *testing.T) {
		component := &Component{
			consentCheckEndpoint: "",
		}

		authzReq := xacml.AuthzRequest{
			PatientBSN: "900186021",
			EventCode:  "GGC002",
		}

		_, err := component.CheckConsent(context.Background(), authzReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "consent check endpoint not configured")
	})

	t.Run("successful consent check", func(t *testing.T) {
		// Mock MITZ server with valid XACML response
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "text/xml", r.Header.Get("Content-Type"))

			// Return mock XACML response with Permit decision
			w.WriteHeader(http.StatusOK)
			xacmlResponse := `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>Permit</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`
			w.Write([]byte(xacmlResponse))
		}))
		defer mockServer.Close()

		component := &Component{
			httpClient:           mockServer.Client(),
			consentCheckEndpoint: mockServer.URL,
		}

		authzReq := xacml.AuthzRequest{
			PatientBSN:             "900186021",
			HealthcareFacilityType: "Z3",
			AuthorInstitutionID:    "00000659",
			EventCode:              "GGC002",
			SubjectRole:            "01.015",
			ProviderID:             "000095254",
			ProviderInstitutionID:  "00000666",
			ConsultingFacilityType: "Z3",
			PurposeOfUse:           "TREAT",
		}

		response, err := component.CheckConsent(context.Background(), authzReq)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, xacml.DecisionPermit, response.Decision)
		assert.NotNil(t, response.RawXML)
		assert.Contains(t, string(response.RawXML), "Permit")
	})

	t.Run("consent check with server error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<error>Server error</error>`))
		}))
		defer mockServer.Close()

		component := &Component{
			httpClient:           mockServer.Client(),
			consentCheckEndpoint: mockServer.URL,
		}

		authzReq := xacml.AuthzRequest{
			PatientBSN: "900186021",
			EventCode:  "GGC002",
		}

		_, err := component.CheckConsent(context.Background(), authzReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "consent check failed with status 500")
	})
}

func TestParseXACMLResponse(t *testing.T) {
	t.Run("parse permit decision", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>Permit</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, xacml.DecisionPermit, response.Decision)
		assert.Equal(t, xacmlResponse, response.RawXML)
	})

	t.Run("parse deny decision", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>Deny</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, xacml.DecisionDeny, response.Decision)
	})

	t.Run("parse not applicable decision", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>NotApplicable</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, xacml.DecisionNotApplicable, response.Decision)
	})

	t.Run("parse indeterminate decision", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>Indeterminate</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, xacml.DecisionIndeterminate, response.Decision)
	})

	t.Run("invalid XML", func(t *testing.T) {
		xacmlResponse := []byte(`not valid xml`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.Error(t, err)
		assert.Nil(t, response)
		// Either could happen depending on what the parser does with invalid XML
		assert.True(t, len(err.Error()) > 0)
	})

	t.Run("missing decision element", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "Decision element not found")
	})

	t.Run("invalid decision value", func(t *testing.T) {
		xacmlResponse := []byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
    <s:Body>
        <Response xmlns="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17">
            <Result>
                <Decision>InvalidValue</Decision>
            </Result>
        </Response>
    </s:Body>
</s:Envelope>`)

		response, err := xacml.ParseXACMLResponse(xacmlResponse)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "invalid decision value")
	})
}

// Helper function
func toPtr(s string) *string {
	return &s
}
