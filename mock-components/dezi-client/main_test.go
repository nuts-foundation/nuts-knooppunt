package main

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestGenerateCodeVerifier(t *testing.T) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("Failed to generate code verifier: %v", err)
	}

	if len(verifier) == 0 {
		t.Error("Code verifier should not be empty")
	}

	// Verify it's valid base64 URL encoding
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("Code verifier length %d is outside valid range [43-128]", len(verifier))
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := generateCodeChallenge(verifier)

	// The challenge should be a base64 URL encoded SHA256 hash
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if challenge != expected {
		t.Errorf("Code challenge mismatch: got %s, expected %s", challenge, expected)
	}
}

func TestGenerateRandomString(t *testing.T) {
	length := 32
	str := generateRandomString(length)

	if len(str) != length {
		t.Errorf("Random string length mismatch: got %d, expected %d", len(str), length)
	}

	// Generate another and verify they're different
	str2 := generateRandomString(length)
	if str == str2 {
		t.Error("Random strings should be different")
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		voorletters string
		voorvoegsel *string
		achternaam  string
		expected    string
	}{
		{"A.B.", nil, "Zorgmedewerker", "A.B. Zorgmedewerker"},
		{"J.", stringPtr("van"), "Dijk", "J. van Dijk"},
		{"M.C.", stringPtr("de"), "Vries", "M.C. de Vries"},
		{"P.", stringPtr(""), "Jansen", "P. Jansen"},
	}

	for _, tt := range tests {
		result := formatName(tt.voorletters, tt.voorvoegsel, tt.achternaam)
		if result != tt.expected {
			t.Errorf("formatName(%s, %v, %s) = %s, expected %s",
				tt.voorletters, tt.voorvoegsel, tt.achternaam, result, tt.expected)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestParseVerklaringJWT(t *testing.T) {
	// Example JWT from the Dezi specification (simplified)
	// This is a test JWT with a mock payload
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{
		"jti": "61b1fafc-4ec7-4489-a280-8d0a50a3d5a9",
		"iss": "abonnee.dezi.nl",
		"exp": 1740131176,
		"nbf": 1732182376,
		"json_schema": "https://www.dezi.nl/json_schemas/v1/verklaring.json",
		"loa_dezi": "http://eidas.europe.eu/LoA/high",
		"verklaring_id": "8539f75d-634c-47db-bb41-28791dfd1f8d",
		"dezi_nummer": "123456789",
		"voorletters": "A.B.",
		"voorvoegsel": null,
		"achternaam": "Zorgmedewerker",
		"abonnee_nummer": "87654321",
		"abonnee_naam": "Zorgaanbieder",
		"rol_code": "01.000",
		"rol_naam": "Arts",
		"rol_code_bron": "http://www.dezi.nl/rol_code_bron/big",
		"status_uri": "https://auth.dezi.nl/status/v1/verklaring/8539f75d-634c-47db-bb41-28791dfd1f8d"
	}`))
	signature := "fake-signature"
	jwtToken := fmt.Sprintf("%s.%s.%s", header, payload, signature)

	verklaring, err := parseVerklaringJWT(jwtToken)
	if err != nil {
		t.Fatalf("Failed to parse verklaring: %v", err)
	}

	if verklaring.DeziNummer != "123456789" {
		t.Errorf("Expected dezi_nummer 123456789, got %s", verklaring.DeziNummer)
	}

	if verklaring.Voorletters != "A.B." {
		t.Errorf("Expected voorletters A.B., got %s", verklaring.Voorletters)
	}

	if verklaring.Achternaam != "Zorgmedewerker" {
		t.Errorf("Expected achternaam Zorgmedewerker, got %s", verklaring.Achternaam)
	}

	if verklaring.RolCode != "01.000" {
		t.Errorf("Expected rol_code 01.000, got %s", verklaring.RolCode)
	}
}


