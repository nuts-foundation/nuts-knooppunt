package lrza

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/gowebpki/jcs"
	"github.com/lestrrat-go/jwx/v2/cert"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/nuts-foundation/nuts-knooppunt/lib/certutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
)

// Provenance signing (optional in the IG): after a write, the tenant signs the stored resource, and a
// Provenance carrying the signature is registered alongside it, so a consumer can verify the
// resource came from the care provider. The signature is a detached JWS over the resource's
// canonical form, per https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/signing.html.
// It is only made for tenants with a signing certificate configured (lrza.registration.signing).

const (
	// staticCanonicalization is the canonicalization the signature is computed over.
	staticCanonicalization = "http://hl7.org/fhir/canonicalization/json#static"
	// signatureTargetFormat is Provenance.signature.targetFormat: the signed resource, canonicalized.
	signatureTargetFormat = "application/fhir+json;canonicalization=" + staticCanonicalization
	signatureFormat       = "application/jose"
)

// signer signs with one tenant's certificate and key.
type signer struct {
	key   crypto.Signer
	alg   jwa.SignatureAlgorithm
	chain *cert.Chain
}

func loadSigner(config SigningConfig) (*signer, error) {
	keyPair, err := certutil.Load(config.CertFile, config.KeyFile, config.KeyPassword)
	if err != nil {
		return nil, err
	}
	return newSigner(keyPair.PrivateKey, keyPair.Chain)
}

// newSigner creates a signer for a key and its certificate chain (DER, leaf first).
func newSigner(key crypto.Signer, chainDER [][]byte) (*signer, error) {
	var alg jwa.SignatureAlgorithm
	switch typed := key.Public().(type) {
	case *rsa.PublicKey:
		alg = jwa.RS256
	case *ecdsa.PublicKey:
		switch typed.Curve {
		case elliptic.P256():
			alg = jwa.ES256
		case elliptic.P384():
			alg = jwa.ES384
		default:
			return nil, errors.New("unsupported ECDSA curve")
		}
	default:
		return nil, fmt.Errorf("unsupported key type %T", typed)
	}
	chain := &cert.Chain{}
	for _, der := range chainDER {
		if err := chain.AddString(base64.StdEncoding.EncodeToString(der)); err != nil {
			return nil, err
		}
	}
	return &signer{key: key, alg: alg, chain: chain}, nil
}

// canonicalize returns the canonical form of a resource the signature is computed over (json#static):
// Resource.meta and every narrative (.text) element are removed, recursively, and the result is
// serialized per RFC 8785 (JCS).
func canonicalize(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	stripped, err := json.Marshal(stripForSigning(value))
	if err != nil {
		return nil, err
	}
	return jcs.Transform(stripped)
}

// stripForSigning removes meta and narratives. Every "meta" object in a resource is a Resource.meta
// (of the resource itself or a contained one). A "text" is only removed when it is a Narrative - an
// object - since e.g. CodeableConcept.text is a plain string that is part of the signed content.
func stripForSigning(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if key == "meta" {
				if _, ok := child.(map[string]any); ok {
					continue
				}
			}
			if key == "text" {
				if _, ok := child.(map[string]any); ok {
					continue
				}
			}
			result[key] = stripForSigning(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = stripForSigning(child)
		}
		return result
	default:
		return value
	}
}

// signDetached returns the compact detached JWS (header..signature) over payload.
func (s *signer) signDetached(payload []byte, signingTime time.Time) ([]byte, error) {
	headers := jws.NewHeaders()
	for key, value := range map[string]any{
		jws.TypeKey:          "JOSE",
		jws.X509CertChainKey: s.chain,
		"canon":              staticCanonicalization,
		"sigT":               signingTime.UTC().Format(time.RFC3339),
		"srCms": []any{map[string]any{
			"commId": map[string]any{"id": "urn:oid:" + coding.AuthorSignatureCode, "desc": "Author's Signature"},
		}},
	} {
		if err := headers.Set(key, value); err != nil {
			return nil, fmt.Errorf("failed to set JWS header %s: %w", key, err)
		}
	}
	return jws.Sign(nil, jws.WithKey(s.alg, s.key, jws.WithProtectedHeaders(headers)), jws.WithDetachedPayload(payload))
}

// signProvenance signs a stored resource and registers the Provenance carrying the signature: the
// tenant's Organization (in the LRZA) is both the agent (author) and the signer.
func signProvenance(ctx context.Context, client fhirclient.Client, s *signer, tenantURA string, organization libfhir.Resource, now time.Time, stored []byte) error {
	if organization == nil {
		return fmt.Errorf("the tenant's Organization (URA %s) is not registered in the LRZA", tenantURA)
	}
	provenance, err := buildProvenance(s, tenantURA, organization.Reference(), now, stored)
	if err != nil {
		return err
	}
	var headers fhirclient.Headers
	var status int
	if err := client.CreateWithContext(ctx, provenance, nil, fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status)); err != nil {
		return upstreamError("register the Provenance at", err, status, headers.Header)
	}
	return nil
}

// buildProvenance builds the Provenance for a stored resource, signed with s on behalf of the tenant,
// whose Organization is organizationReference.
func buildProvenance(s *signer, tenantURA, organizationReference string, now time.Time, stored []byte) (libfhir.Resource, error) {
	target, err := libfhir.ParseResource(stored)
	if err != nil {
		return nil, err
	}
	if target.ID() == "" {
		return nil, errors.New("the stored resource has no id")
	}
	targetReference := target.Reference()
	if versionID := target.VersionID(); versionID != "" {
		targetReference += "/_history/" + versionID
	}
	canonical, err := canonicalize(stored)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize the resource: %w", err)
	}
	now = now.UTC()
	compact, err := s.signDetached(canonical, now)
	if err != nil {
		return nil, fmt.Errorf("failed to sign the resource: %w", err)
	}
	who := map[string]any{
		"reference":  organizationReference,
		"type":       "Organization",
		"identifier": map[string]any{"system": coding.URAOIDNamingSystem, "value": tenantURA},
	}
	recorded := now.Format(time.RFC3339)
	return libfhir.Resource{
		"resourceType": "Provenance",
		"target":       []any{map[string]any{"reference": targetReference}},
		"recorded":     recorded,
		"agent": []any{map[string]any{
			"type": map[string]any{"coding": []any{map[string]any{
				"system": coding.ProvenanceParticipantTypeSystem,
				"code":   "author",
			}}},
			"who": who,
		}},
		"signature": []any{map[string]any{
			"type": []any{map[string]any{
				"system":  coding.SignatureTypeSystem,
				"code":    coding.AuthorSignatureCode,
				"display": "Author's Signature",
			}},
			"when":         recorded,
			"who":          who,
			"targetFormat": signatureTargetFormat,
			"sigFormat":    signatureFormat,
			// The compact JWS is itself base64url-encoded text; the IG requires base64-encoding it
			// once more for the base64Binary data element.
			"data": base64.StdEncoding.EncodeToString(compact),
		}},
	}, nil
}
