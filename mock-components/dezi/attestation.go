package main

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Signer mints Dezi v0.7 "verklaring" attestations and publishes its public key
// as a JWK Set. The claim and header shape follows the Dezi v0.7 subject that
// the Nuts node reads in vcr/credential/dezi.go.
type Signer struct {
	key    *rsa.PrivateKey
	keyID  string
	issuer string
	jku    string
}

func NewSigner(key *rsa.PrivateKey, keyID, issuer, jku string) *Signer {
	return &Signer{key: key, keyID: keyID, issuer: issuer, jku: jku}
}

// Attestation returns a signed v0.7 verklaring JWT. iss, jti, nbf and exp are
// mandatory: the Nuts node maps them onto the credential issuer, id,
// issuanceDate and expirationDate, and rejects the credential if they disagree.
func (s *Signer) Attestation(p Practitioner, now time.Time, validity time.Duration) (string, error) {
	claims := map[string]any{
		jwt.JwtIDKey:      uuid.NewString(),
		jwt.IssuerKey:     s.issuer,
		jwt.NotBeforeKey:  now.Unix(),
		jwt.ExpirationKey: now.Add(validity).Unix(),
		"json_schema":     "https://www.dezi.nl/json_schemas/v1/verklaring.json",
		"loa_dezi":        "http://eidas.europa.eu/LoA/high",
		"verklaring_id":   uuid.NewString(),
		"dezi_nummer":     p.DeziNumber,
		"voorletters":     p.Initials,
		"voorvoegsel":     p.SurnamePrefix,
		"achternaam":      p.Surname,
		"abonnee_nummer":  p.OrgURA,
		"abonnee_naam":    p.OrgName,
		"rol_code":        p.RoleCode,
		"rol_naam":        p.RoleName,
		"rol_code_bron":   p.RoleRegistry,
	}
	token := jwt.New()
	for name, value := range claims {
		if err := token.Set(name, value); err != nil {
			return "", fmt.Errorf("set claim %s: %w", name, err)
		}
	}

	headers := jws.NewHeaders()
	for name, value := range map[string]any{"alg": "RS256", "kid": s.keyID, "jku": s.jku} {
		if err := headers.Set(name, value); err != nil {
			return "", fmt.Errorf("set header %s: %w", name, err)
		}
	}

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, s.key, jws.WithProtectedHeaders(headers)))
	if err != nil {
		return "", fmt.Errorf("sign attestation: %w", err)
	}
	return string(signed), nil
}

// JWKSet returns the serialized JWK Set served at the jku URL.
func (s *Signer) JWKSet() ([]byte, error) {
	key, err := jwk.FromRaw(s.key.Public())
	if err != nil {
		return nil, fmt.Errorf("build JWK: %w", err)
	}
	if err := key.Set(jwk.KeyIDKey, s.keyID); err != nil {
		return nil, err
	}
	if err := key.Set(jwk.AlgorithmKey, jwa.RS256); err != nil {
		return nil, err
	}
	set := jwk.NewSet()
	if err := set.AddKey(key); err != nil {
		return nil, err
	}
	return json.Marshal(set)
}
