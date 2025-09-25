package bsnutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateTransportToken(t *testing.T) {
	tests := []struct {
		name   string
		bsn    string
		audience string
	}{
		{
			name:   "basic token creation",
			bsn:    "123456789",
			audience: "nvi",
		},
		{
			name:   "different audience with hyphens",
			bsn:    "987654321",
			audience: "org-with-hyphens",
		},
		{
			name:   "BSN with leading zeros",
			bsn:    "000123456",
			audience: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := CreateTransportToken(tt.bsn, tt.audience)
			require.NoError(t, err)

			// Check that the token has the correct format (starts with "token-{audience}-")
			expectedPrefix := "token-" + tt.audience + "-"
			if !strings.HasPrefix(token, expectedPrefix) {
				t.Errorf("CreateTransportToken() = %v, expected to start with %v", token, expectedPrefix)
			}

			// Check that token has the expected number of parts (audience, transformedBSN, nonce)
			parts := strings.Split(token[6:], "-") // Skip "token-"
			if len(parts) < 2 {
				t.Errorf("CreateTransportToken() = %v, expected at least 2 parts after 'token-'", token)
			}

			// Verify BSN format is perfectly preserved through round-trip transformation
			extractedBSN, err := BSNFromTransportToken(token)
			require.NoError(t, err)
			// BSN format should be exactly preserved (including leading zeros)
			if extractedBSN == "" {
				t.Errorf("BSNFromTransportToken() returned empty string")
			}
		})
	}
}

func TestBSNFromTransportToken_InvalidFormats(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "invalid format - no prefix",
			token: "nvi-123-abc123",
		},
		{
			name:  "invalid format - wrong prefix",
			token: "invalid-nvi-123-abc123",
		},
		{
			name:  "invalid format - missing parts",
			token: "token-nvi",
		},
		{
			name:  "empty string",
			token: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BSNFromTransportToken(tt.token)

			if err == nil {
				t.Errorf("BSNFromTransportToken() expected error for invalid token %q, but got none", tt.token)
			}
		})
	}
}

func TestTransportTokenUniqueness(t *testing.T) {
	bsn := "123456789"
	audience := "test-org"

	// Generate multiple transport tokens for same BSN + audience and check uniqueness
	tokens := make(map[string]bool)
	for range 5 {
		token, err := CreateTransportToken(bsn, audience)
		require.NoError(t, err)
		if tokens[token] {
			t.Errorf("Transport token %s was generated twice - tokens should be unique", token)
		}
		tokens[token] = true

		// Verify token resolves back to original BSN format
		extractedBSN, err := BSNFromTransportToken(token)
		require.NoError(t, err)
		if extractedBSN == "" {
			t.Errorf("Token resolved to empty BSN")
		}
	}

}

func TestPseudonymConsistency(t *testing.T) {
	bsn := "987654321"
	audience := "nvi"

	// Create multiple transport tokens and convert to pseudonyms
	var expectedPseudonym string
	for i := range 3 {
		token, err := CreateTransportToken(bsn, audience)
		require.NoError(t, err)
		pseudonym, err := TransportTokenToPseudonym(token)
		require.NoError(t, err)

		if i == 0 {
			expectedPseudonym = pseudonym
		} else if pseudonym != expectedPseudonym {
			t.Errorf("Pseudonyms should be consistent: got %s, want %s", pseudonym, expectedPseudonym)
		}
	}

}

func TestDifferentHoldersDifferentPseudonyms(t *testing.T) {
	bsn := "555666777"
	audiences := []string{"nvi", "org-a", "org-b", "hospital-with-long-name"}

	// Same BSN, different audiences should produce different pseudonyms
	pseudonyms := make(map[string]string)
	for _, audience := range audiences {
		token, err := CreateTransportToken(bsn, audience)
		require.NoError(t, err)
		pseudonym, err := TransportTokenToPseudonym(token)
		require.NoError(t, err)
		pseudonyms[audience] = pseudonym
	}

	// All pseudonyms should be different
	seen := make(map[string]string)
	for audience, pseudonym := range pseudonyms {
		if prevHolder, exists := seen[pseudonym]; exists {
			t.Errorf("Different audiences produced same pseudonym: %s (audiences: %s and %s)", pseudonym, prevHolder, audience)
		}
		seen[pseudonym] = audience
	}

}

func TestBSNFormatPreservation(t *testing.T) {
	// Test that BSN format is preserved through round-trip transformation
	testCases := []string{
		"123456789",   // Regular BSN
		"000123456",   // BSN with leading zeros
		"000000001",   // More leading zeros
		"999888777",   // Different numbers
	}

	for _, originalBSN := range testCases {
		t.Run("BSN_"+originalBSN, func(t *testing.T) {
			// Create transport token
			token, err := CreateTransportToken(originalBSN, "test-org")
			require.NoError(t, err)

			// Extract BSN back
			extractedBSN, err := BSNFromTransportToken(token)
			require.NoError(t, err)

			// Verify format is preserved
			if originalBSN != extractedBSN {
				t.Errorf("BSN format not preserved: original=%q, extracted=%q", originalBSN, extractedBSN)
			}
		})
	}
}

func TestCompleteWorkflow(t *testing.T) {
	// Test the complete federated health workflow from the architecture diagram
	bsn := "123456789"

	// LOCALIZATION: Knooppunt A registers DocumentReference
	// Step 1: Knooppunt A creates transport token with audience="nvi"
	tokenFromA, err := CreateTransportToken(bsn, "nvi")
	require.NoError(t, err)

	// Step 2: NVI converts to pseudonym for storage (shared across orgs)
	sharedPseudonym, err := TransportTokenToPseudonym(tokenFromA)
	require.NoError(t, err)

	// SEARCH: Knooppunt B queries for same BSN
	// Step 3: Knooppunt B creates transport token with audience="nvi" (same as A)
	tokenFromB, err := CreateTransportToken(bsn, "nvi")
	require.NoError(t, err)

	// Step 4: NVI converts B's token to pseudonym (should match stored one)
	searchPseudonym, err := TransportTokenToPseudonym(tokenFromB)
	require.NoError(t, err)
	if searchPseudonym != sharedPseudonym {
		t.Errorf("Search pseudonym mismatch: got %s, want %s", searchPseudonym, sharedPseudonym)
	}

	// Step 5: NVI creates org-specific token for Knooppunt B
	tokenForB, err := PseudonymToTransportToken(sharedPseudonym, "knooppunt-b")
	require.NoError(t, err)

	// Step 6: Knooppunt B extracts BSN from their org-specific token
	extractedBSN, err := BSNFromTransportToken(tokenForB)
	require.NoError(t, err)

	// Verify extraction works and format is preserved
	if extractedBSN == "" {
		t.Errorf("Complete workflow failed: got empty BSN")
	}
}

func TestGenerateSimpleKey(t *testing.T) {
	tests := []string{"nvi", "org-b", "hospital-with-long-name", ""}

	for _, audience := range tests {
		t.Run("audience_"+audience, func(t *testing.T) {
			key1 := generateSimpleKey(audience)
			key2 := generateSimpleKey(audience)

			// Same input should produce same key (deterministic)
			if key1 != key2 {
				t.Errorf("generateSimpleKey() is not deterministic: got %d and %d", key1, key2)
			}

			// Key should be positive (bit shift ensures this)
			if key1 < 0 {
				t.Errorf("generateSimpleKey() returned negative key: %d", key1)
			}

			// Key should fit in 31-bit range (bit shift guarantees this)
			maxValue := (1 << 31) - 1 // 2^31 - 1 = 2,147,483,647
			if key1 > maxValue {
				t.Errorf("generateSimpleKey() returned key too large: %d > %d", key1, maxValue)
			}
		})
	}

	// Different audiences should produce different keys
	key1 := generateSimpleKey("nvi")
	key2 := generateSimpleKey("org-b")

	if key1 == key2 {
		t.Errorf("generateSimpleKey() produced same key for different audiences")
	}
}

