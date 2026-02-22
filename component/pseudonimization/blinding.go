package pseudonimization

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

func deriveKey(identifier prsIdentifier, recipientOrganization string, recipientScope string) ([]byte, error) {
	info := fmt.Sprintf("%s|%s|v1", recipientOrganization, recipientScope)

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

func blindIdentifier(identifier prsIdentifier, recipientOrganization string, recipientScope string) ([]byte, []byte, error) {
	pseudonym, err := deriveKey(identifier, recipientOrganization, recipientScope)
	if err != nil {
		return nil, nil, fmt.Errorf("deriving key: %w", err)
	}

	client := oprf.NewClient(oprf.SuiteRistretto255)
	blindFactor, blindedInput, err := client.Blind([][]byte{pseudonym})
	if err != nil {
		return nil, nil, fmt.Errorf("oprf: %w", err)
	}
	blindData, err := blindFactor.CopyBlinds()[0].MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("oprf marshaling blind factor: %w", err)
	}
	blindedInputData, err := blindedInput.Elements[0].MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("oprf marshaling blinded input: %w", err)
	}
	return blindData, blindedInputData, nil
}
