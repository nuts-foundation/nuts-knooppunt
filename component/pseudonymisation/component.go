//go:generate mockgen -destination=component_mock.go -package=pseudonymisation -source=component.go
package pseudonymisation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"

	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Config struct {
	PRSBaseURL string `koanf:"prsurl"`
}

type Pseudonymizer interface {
	IdentifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, recipientURA string, scope string) (*fhir.Identifier, error)
}

func New(cfg Config, httpClientFn authn.HTTPClientProvider) *Component {
	return &Component{
		httpClientFn: httpClientFn,
		config:       cfg,
	}
}

type Component struct {
	httpClientFn authn.HTTPClientProvider
	config       Config
}

// IdentifierToToken converts a BSN identifier to a pseudonymous transport token using the PRS service.
// The hash-to-group, blinding and evaluation steps are those of RFC 9497's
// OPRF(ristretto255, SHA-512) suite, but the recipient consumes the unblinded
// group element itself rather than RFC 9497's Finalize output; see
// docs/prs-contract.md. The process:
// 1. Create PRS identifier from BSN
// 2. Derive the OPRF input using HKDF
// 3. Blind the input (RFC 9497 Blind)
// 4. Send the blinded element to the PRS for evaluation (RFC 9497 BlindEvaluate)
// 5. PRS returns the evaluated element in a JWE for the recipient, which de-blinds it (the NVI)
func (c Component) IdentifierToToken(ctx context.Context, identifier fhir.Identifier, localOrganizationURA string, recipientURA string, scope string) (*fhir.Identifier, error) {
	if c.config.PRSBaseURL == "" {
		// TODO: Remove Fake Pseudonymizer fallback once PRS is properly integrated
		slog.WarnContext(ctx, "PRS base URL is not configured, using fake pseudonymizer for IdentifierToToken")
		return FakePseudonymizer{}.IdentifierToToken(ctx, identifier, localOrganizationURA, recipientURA, scope)
	}

	if identifier.System == nil || *identifier.System != coding.BSNNamingSystem || identifier.Value == nil {
		return &identifier, nil
	}

	// Step 1: Create PRS identifier
	prsID := prsIdentifier{
		LandCode: "NL",
		Type:     "BSN",
		Value:    *identifier.Value,
	}

	// Step 2 & 3: Blind the identifier (internally derives key and blinds)
	blinded, err := blindIdentifier(prsID, recipientURA, scope)
	if err != nil {
		return nil, fmt.Errorf("blinding identifier: %w", err)
	}

	// Step 4: Call PRS to evaluate the blinded input; PRS returns the evaluated output as a JWE
	evaluatedOutput, err := c.callPRSEvaluate(ctx, localOrganizationURA, recipientURA, scope, blinded.blindedInput)
	if err != nil {
		return nil, err
	}

	// Step 5: Build the subject identifier value as a base64url-encoded JSON
	// object per spec, carrying the blind factor so the consuming system can
	// deblind
	subjectIdentifierValue, err := marshalSubjectIdentifier(blinded.blindFactor, evaluatedOutput)
	if err != nil {
		return nil, err
	}

	return &fhir.Identifier{
		System: to.Ptr(coding.BSNTransportTokenNamingSystem),
		Value:  &subjectIdentifierValue,
	}, nil
}

type subjectIdentifier struct {
	BlindFactor     []byte `json:"blind_factor"`
	EvaluatedOutput string `json:"evaluated_output"`
}

func marshalSubjectIdentifier(blindFactor []byte, evaluatedOutput string) (string, error) {
	data, err := json.Marshal(subjectIdentifier{
		BlindFactor:     blindFactor,
		EvaluatedOutput: evaluatedOutput,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling subject identifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

type prsIdentifier struct {
	LandCode string `json:"landCode"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

type prsEvaluateRequest struct {
	RecipientOrganization string `json:"recipientOrganization"`
	RecipientScope        string `json:"recipientScope"`
	EncryptedPersonalID   []byte `json:"encryptedPersonalId"`
}

type prsEvaluateResponse struct {
	JWE string `json:"jwe"`
}

// callPRSEvaluate sends the blinded input to the PRS service and returns the pseudonymized identifier
func (c Component) callPRSEvaluate(ctx context.Context, localOrganizationURA string, recipientURA string, scope string, blindedInputData []byte) (string, error) {
	httpClient, err := c.httpClientFn(ctx, []string{"prs:read"}, localOrganizationURA, c.config.PRSBaseURL)
	if err != nil {
		return "", fmt.Errorf("creating PRS HTTP client: %w", err)
	}

	requestBody := prsEvaluateRequest{
		RecipientOrganization: "ura:" + recipientURA,
		RecipientScope:        scope,
		EncryptedPersonalID:   blindedInputData,
	}

	bodyBytes, err := marshalPRS(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshaling PRS request body: %w", err)
	}

	requestURL, err := url.Parse(c.config.PRSBaseURL)
	if err != nil {
		return "", err
	}
	requestURL.Path = path.Join(requestURL.Path, "oprf/eval")
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := httpClient.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("PRS request: %w", err)
	}
	defer httpResponse.Body.Close()

	response, err := from.JSONResponse[prsEvaluateResponse](httpResponse)
	if err != nil {
		return "", fmt.Errorf("PRS response: %w", err)
	}
	return response.JWE, nil
}
