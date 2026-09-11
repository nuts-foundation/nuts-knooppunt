package pseudonymisation

import (
	"encoding/hex"
	"testing"

	"github.com/cloudflare/circl/group"
	"github.com/stretchr/testify/require"
)

func Test_marshalPRS(t *testing.T) {
	t.Run("identifier", func(t *testing.T) {
		identifier := prsIdentifier{
			LandCode: "NL",
			Type:     "BSN",
			Value:    "999940003",
		}
		out, err := marshalPRS(identifier)
		require.NoError(t, err)
		require.Equal(t, `{"landCode":"NL","type":"BSN","value":"999940003"}`, string(out))
	})

	t.Run("canonicalizes member order and escaping", func(t *testing.T) {
		// The identifier's members happen to be declared in canonical order,
		// so this input is where RFC 8785 and encoding/json visibly differ:
		// members sorted by name, and no HTML-safe escaping of "&".
		out, err := marshalPRS(struct {
			Z string `json:"z"`
			A string `json:"a"`
		}{Z: "x&y", A: "1"})
		require.NoError(t, err)
		require.Equal(t, `{"a":"1","z":"x&y"}`, string(out))
	})
}

func Test_deriveKey(t *testing.T) {
	identifier := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    "999940003",
	}

	t.Run("matches an independent HKDF-SHA256 derivation", func(t *testing.T) {
		// Expected value computed outside Go with OpenSSL 3.6.3:
		//
		//   openssl kdf -keylen 32 -kdfopt digest:SHA256 \
		//     -kdfopt hexkey:$(printf '%s' '{"landCode":"NL","type":"BSN","value":"999940003"}' | xxd -p | tr -d '\n') \
		//     -kdfopt info:'ura:90000901|nationale-verwijsindex|v1' HKDF
		//
		// This pins every HKDF parameter at once: SHA-256, 32 bytes, no salt
		// (RFC 5869 then uses HashLen zero bytes), the info string with its
		// "ura:" prefix and "|v1" suffix, and the canonical JSON identifier as
		// input keying material.
		key, err := deriveKey(identifier, "90000901", "nationale-verwijsindex")
		require.NoError(t, err)
		require.Equal(t, "e96ce9a92e6b5fdf3ba6810b25c152c49c2969f6194e3e43fb050365a7104d9f", hex.EncodeToString(key))
	})

	t.Run("is bound to identifier, recipient and scope", func(t *testing.T) {
		key, err := deriveKey(identifier, "90000901", "nationale-verwijsindex")
		require.NoError(t, err)
		otherIdentifier, err := deriveKey(prsIdentifier{LandCode: "NL", Type: "BSN", Value: "999940005"}, "90000901", "nationale-verwijsindex")
		require.NoError(t, err)
		otherRecipient, err := deriveKey(identifier, "90000902", "nationale-verwijsindex")
		require.NoError(t, err)
		otherScope, err := deriveKey(identifier, "90000901", "other-scope")
		require.NoError(t, err)

		require.NotEqual(t, key, otherIdentifier)
		require.NotEqual(t, key, otherRecipient)
		require.NotEqual(t, key, otherScope)
	})
}

func Test_blindIdentifier(t *testing.T) {
	identifier := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    "999940003",
	}
	blinded, err := blindIdentifier(identifier, "90000901", "nationale-verwijsindex")
	require.NoError(t, err)

	require.Len(t, blinded.blindedInput, 32)
	require.NoError(t, group.Ristretto255.NewElement().UnmarshalBinary(blinded.blindedInput), "a ristretto255 element")
	require.Len(t, blinded.blindFactor, 32)
	require.NoError(t, group.Ristretto255.NewScalar().UnmarshalBinary(blinded.blindFactor), "a ristretto255 scalar")
}
