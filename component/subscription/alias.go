package subscription

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// The id-only identifier requirements (stable, opaque, unpredictable, consumer-opaque)
// are SHALL-level in TTA Notifications v0.6. HAPI's default sequential ids fail opacity
// and unpredictability outright — which is precisely the research-question-8 finding this
// layer demonstrates: focus references are exposed under a keyed-transformation (HMAC)
// alias of the internal id, per the spec's own compliant-scheme example. Stable (same
// input, same alias), opaque and unpredictable without the server's key.
//
// The flip side (doc 09, honest limits): aliasing pulls the knooppunt into the pull path,
// because a receiver's follow-up read must come back through whatever can de-alias — see
// Component.ReadAliasedTask.

// base32lower is RFC-4648 base32 without padding, lowercased — matches the spec's example
// alias shape (j5xw6z3vnfxgk4ttmvwgc3dj) and stays URL-safe.
var base32lower = base32.StdEncoding.WithPadding(base32.NoPadding)

type aliaser struct {
	key []byte
}

func newAliaser(configuredKey string) *aliaser {
	key := []byte(configuredKey)
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("failed to generate alias key: " + err.Error())
		}
	}
	return &aliaser{key: key}
}

// alias computes the stable opaque alias id for an internal relative reference
// (e.g. Task/4711 -> j5xw6z3vnfxgk4ttmvwgc3dj).
func (a *aliaser) alias(internalRef string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(internalRef))
	digest := mac.Sum(nil)
	// 15 bytes -> 24 base32 characters, matching the spec example's length; ample
	// collision resistance for a POC.
	return strings.ToLower(base32lower.EncodeToString(digest[:15]))
}
