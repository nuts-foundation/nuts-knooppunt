package pseudonymisation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_marshalPRS_identifier(t *testing.T) {
	identifier := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    "999940003",
	}
	out, err := marshalPRS(identifier)
	require.NoError(t, err)
	require.Equal(t, `{"landCode":"NL","type":"BSN","value":"999940003"}`, string(out))
}

func Test_deriveKey(t *testing.T) {
	identifier := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    "900186021",
	}
	key1, err := deriveKey(identifier, "ura:1234", "nationale-verwijsindex")
	require.NoError(t, err)
	key2, err := deriveKey(identifier, "ura:1234", "nationale-verwijsindex")
	require.NoError(t, err)

	require.Equal(t, key1, key2)
}

func Test_blindIdentifier(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		identifier := prsIdentifier{
			LandCode: "NL",
			Type:     "BSN",
			Value:    "900186021",
		}
		blindedInputData, err := blindIdentifier(identifier, "ura:1234", "nationale-verwijsindex")
		require.NoError(t, err)
		require.NotEmpty(t, blindedInputData)
	})
}
