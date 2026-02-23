package pseudonimization

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_IdentifierToToken(t *testing.T) {
	t.Run("converts BSN to transport token via PRS", func(t *testing.T) {
		// Setup mock PRS server
		var receivedBlindedInput []byte
		prsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/orpf/eval", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Read request
			var req prsEvaluateRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			receivedBlindedInput = req.EncryptedPersonalID

			// PRS returns the final pseudonymized identifier (not the evaluated output)
			// In a real PRS, this would be the result of evaluation + deblinding + encoding
			resp := prsEvaluateResponse{
				PseudonymizedIdentifier: "pseudonym-12345-abc",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer prsServer.Close()

		// Create component
		component := New(&http.Client{}, prsServer.URL)

		// Test BSN identifier
		bsnIdentifier := fhir.Identifier{
			System: to.Ptr(coding.BSNNamingSystem),
			Value:  to.Ptr("900186021"),
		}

		// Convert to token
		result, err := component.IdentifierToToken(t.Context(), bsnIdentifier, "ura:1234")
		require.NoError(t, err)

		// Verify result
		assert.NotNil(t, result)
		assert.Equal(t, coding.BSNTransportTokenNamingSystem, *result.System)
		assert.Equal(t, "pseudonym-12345-abc", *result.Value)
		assert.NotEmpty(t, receivedBlindedInput)

		t.Logf("Transport token: %s", *result.Value)
	})

	t.Run("returns same identifier for non-BSN", func(t *testing.T) {
		component := New(&http.Client{}, "http://localhost:9999")

		identifier := fhir.Identifier{
			System: to.Ptr("http://example.com/other"),
			Value:  to.Ptr("12345"),
		}

		result, err := component.IdentifierToToken(t.Context(), identifier, "ura:1234")
		require.NoError(t, err)
		assert.Equal(t, &identifier, result)
	})

	t.Run("handles PRS server error", func(t *testing.T) {
		prsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("PRS server error"))
		}))
		defer prsServer.Close()

		component := New(&http.Client{}, prsServer.URL)

		bsnIdentifier := fhir.Identifier{
			System: to.Ptr(coding.BSNNamingSystem),
			Value:  to.Ptr("900186021"),
		}

		result, err := component.IdentifierToToken(t.Context(), bsnIdentifier, "ura:1234")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "PRS returned status 500")
	})
}

func TestComponent_TokenToBSN(t *testing.T) {
	t.Run("returns error for transport token", func(t *testing.T) {
		component := New(&http.Client{}, "http://localhost:9999")

		tokenIdentifier := fhir.Identifier{
			System: to.Ptr(coding.BSNTransportTokenNamingSystem),
			Value:  to.Ptr("some-token-value"),
		}

		result, err := component.TokenToBSN(tokenIdentifier, "ura:1234")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "TokenToBSN is not supported")
	})

	t.Run("returns same identifier for non-token", func(t *testing.T) {
		component := New(&http.Client{}, "http://localhost:9999")

		identifier := fhir.Identifier{
			System: to.Ptr("http://example.com/other"),
			Value:  to.Ptr("12345"),
		}

		result, err := component.TokenToBSN(identifier, "ura:1234")
		require.NoError(t, err)
		assert.Equal(t, &identifier, result)
	})
}
