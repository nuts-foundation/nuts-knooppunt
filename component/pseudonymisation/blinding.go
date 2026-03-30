package pseudonymisation

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/cloudflare/circl/oprf"
)

// Implementation uses:
// - HKDF
// - OPRF: https://datatracker.ietf.org/doc/rfc9497/ with Ristretto255 (https://datatracker.ietf.org/doc/draft-irtf-cfrg-voprf/)

func deriveKey(identifier prsIdentifier, recipientOrganizationURA string, recipientScope string) ([]byte, error) {
	info := fmt.Sprintf("ura:%s|%s|v1", recipientOrganizationURA, recipientScope)

	identifierJSON, err := json.Marshal(identifier)
	if err != nil {
		// can't happen
		return nil, err
	}

	key, err := hkdf.Key(sha256.New, identifierJSON, nil, info, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}

	return key, nil
}

type blindedIdentifier struct {
	blindedInput []byte
	finalizeData *oprf.FinalizeData
}

func blindIdentifier(identifier prsIdentifier, recipientOrganization string, recipientScope string) (*blindedIdentifier, error) {
	derivedInput, err := deriveKey(identifier, recipientOrganization, recipientScope)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}

	client := oprf.NewClient(oprf.SuiteRistretto255)
	finalizeData, blindedInput, err := client.Blind([][]byte{derivedInput})
	if err != nil {
		return nil, fmt.Errorf("oprf: %w", err)
	}
	blindedInputData, err := blindedInput.Elements[0].MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("oprf marshaling blinded input: %w", err)
	}
	return &blindedIdentifier{
		blindedInput: blindedInputData,
		finalizeData: finalizeData,
	}, nil
}
