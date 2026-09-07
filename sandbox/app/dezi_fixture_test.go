package main

import (
	"encoding/base64"
	"encoding/json"
)

// fixtureAttestation is a syntactically valid compact JWS carrying the canonical
// persona claims. Its signature is meaningless on purpose: this backend is not
// the trust boundary, so it never verifies one. Real signature verification is
// the Nuts node's job and is covered by mock-components/dezi/contract_test.go.
var fixtureAttestation = buildFixtureAttestation()

func buildFixtureAttestation() string {
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "sandbox-dezi-1"})
	payload, _ := json.Marshal(map[string]any{
		"dezi_nummer":    "900001234",
		"voorletters":    "S.",
		"voorvoegsel":    "",
		"achternaam":     "el Amrani",
		"abonnee_nummer": "00000010",
		"abonnee_naam":   "Ziekenhuis De Plataan",
		"rol_code":       "01.022",
		"rol_naam":       "Klinisch geriater",
		"rol_code_bron":  "http://www.dezi.nl/rol_bron/big",
		"exp":            4102444800, // 2100-01-01, so the fixture never expires
	})
	enc := base64.RawURLEncoding.EncodeToString
	return enc(header) + "." + enc(payload) + ".c2lnbmF0dXJl"
}
