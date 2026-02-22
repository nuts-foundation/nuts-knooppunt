package pseudonimization

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/cloudflare/circl/oprf"
	"github.com/stretchr/testify/require"
)

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
		data, input, err := blindIdentifier(identifier, "ura:1234", "nationale-verwijsindex")
		require.NoError(t, err)
		require.NotEmpty(t, data)
		require.NotEmpty(t, input)

		t.Log("blind: " + base64.URLEncoding.EncodeToString(data))
		t.Log("blinded input: " + base64.URLEncoding.EncodeToString(input))

		evaluatedBlind := evaluateBlind(t, input)

		t.Log("evaluated blind: " + base64.URLEncoding.EncodeToString(evaluatedBlind))
	})

}

func evaluateBlind(t *testing.T, blindedInputData []byte) []byte {
	privateKey, err := oprf.GenerateKey(oprf.SuiteRistretto255, rand.Reader)
	require.NoError(t, err)
	server := oprf.NewServer(oprf.SuiteRistretto255, privateKey)

	element := oprf.SuiteRistretto255.Group().NewElement()
	err = element.UnmarshalBinary(blindedInputData)
	require.NoError(t, err)
	evaluation, err := server.Evaluate(&oprf.EvaluationRequest{
		Elements: []oprf.Blinded{element},
	})
	require.NoError(t, err)
	evaluationOutput, err := evaluation.Elements[0].MarshalBinary()
	require.NoError(t, err)
	return evaluationOutput
}
