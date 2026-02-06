package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/policies"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/open-policy-agent/opa/v1/sdk"
)

// createOPAService creates a new Open Policy Agent instance with embedded policy bundles
func createOPAService(ctx context.Context, opaBundleBaseURL string) (*sdk.OPA, error) {
	configBundles := map[string]any{}
	bundles, err := policies.Bundles(ctx)
	if err != nil {
		return nil, err
	}
	for bundleName := range bundles {
		configBundles[bundleName] = map[string]any{
			"resource": fmt.Sprintf("%s.tar.gz", bundleName),
		}
	}
	configMap := map[string]any{
		"services": map[string]any{
			"knooppunt-pdp": map[string]any{
				"url": opaBundleBaseURL,
			},
		},
		"bundles": configBundles,
		"decision_logs": map[string]any{
			"console": true,
		},
	}
	configData, _ := json.Marshal(configMap)
	result, err := sdk.New(ctx, sdk.Options{
		ID:            "knooppunt-pdp",
		Config:        bytes.NewReader(configData),
		Logger:        logging.New(),
		ConsoleLogger: logging.New(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OPA SDK instance: %w", err)
	}
	return result, nil
}

// evalRegoPolicy evaluates a Rego policy using Open Policy Agent for the given scope and input
func (c *Component) evalRegoPolicy(ctx context.Context, policy string, policyInput PolicyInput) (*PolicyResult, error) {
	opaInputMap, err := to.JSONMap(policyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to convert policy input to map: %w", err)
	}
	result, err := c.opaService.Decision(ctx, sdk.DecisionOptions{Path: "/" + policy, Input: opaInputMap})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate policy: %w", err)
	}
	resultMap, ok := result.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected policy result type (expected map[string]any with 'allow' field, was %T)", result.Result)
	}
	allowed, ok := resultMap["allow"].(bool)
	if !ok {
		return nil, fmt.Errorf("unexpected 'allow' result type (expected bool, was %T)", resultMap["allow"])
	}
	policyResult := PolicyResult{
		Allow:  allowed,
		Policy: policy,
	}
	if !allowed {
		var infoLines []string
		// Sort keys to ensure deterministic output
		keys := make([]string, 0, len(resultMap))
		for key := range resultMap {
			if key != "allow" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			infoLines = append(infoLines, fmt.Sprintf("%s: %v", key, resultMap[key]))
		}
		policyResult.Reasons = []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: "access denied by policy",
			},
			{
				Code:        TypeResultCodeInformational,
				Description: strings.Join(infoLines, "\n"),
			},
		}
	}
	return &policyResult, nil
}
