package pseudonymisation

import (
	"encoding/json"

	"github.com/gowebpki/jcs"
)

// marshalPRS returns the RFC 8785 (JCS) canonical JSON for v.
// Used wherever the byte representation affects PRS interop or cryptographic
// output (HKDF input for key derivation, PRS /oprf/eval request body).
func marshalPRS(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}
