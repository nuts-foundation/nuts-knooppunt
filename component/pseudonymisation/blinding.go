package pseudonymisation

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"

	"github.com/cloudflare/circl/oprf"
)

// Implementation uses:
// - HKDF (RFC 5869) to derive the OPRF input
// - the hash-to-group and Blind steps of RFC 9497 OPRF(ristretto255, SHA-512)
//   (https://datatracker.ietf.org/doc/rfc9497/); the recipient de-blinds and
//   uses the group element without RFC 9497 Finalize, see docs/prs-contract.md

func deriveKey(identifier prsIdentifier, recipientOrganizationURA string, recipientScope string) ([]byte, error) {
	info := fmt.Sprintf("ura:%s|%s|v1", recipientOrganizationURA, recipientScope)

	identifierJSON, err := marshalPRS(identifier)
	if err != nil {
		return nil, fmt.Errorf("marshaling identifier: %w", err)
	}

	key, err := hkdf.Key(sha256.New, identifierJSON, nil, info, sha256.Size)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}

	return key, nil
}

type blindedIdentifier struct {
	blindedInput []byte
	blindFactor  []byte
}

func blindIdentifier(identifier prsIdentifier, recipientOrganization string, recipientScope string) (*blindedIdentifier, error) {
	derivedInput, err := deriveKey(identifier, recipientOrganization, recipientScope)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}
	blindedInput, blindFactor, err := BlindInput(derivedInput)
	if err != nil {
		return nil, err
	}
	return &blindedIdentifier{blindedInput: blindedInput, blindFactor: blindFactor}, nil
}

// BlindInput is the Blind step of RFC 9497 OPRF(ristretto255, SHA-512) on
// input: the blinded group element the PRS evaluates and the blind factor the
// recipient de-blinds with, both in the suite's encoding. It is exported so
// that the mock PRS's tests drive the mock with this component's own client
// step rather than a copy of it.
func BlindInput(input []byte) (blindedInput []byte, blindFactor []byte, err error) {
	finalizeData, request, err := oprf.NewClient(oprf.SuiteRistretto255).Blind([][]byte{input})
	if err != nil {
		return nil, nil, fmt.Errorf("oprf: %w", err)
	}
	blindedInput, err = request.Elements[0].MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("oprf marshaling blinded input: %w", err)
	}
	blindFactor, err = finalizeData.CopyBlinds()[0].MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling blind factor: %w", err)
	}
	return blindedInput, blindFactor, nil
}
