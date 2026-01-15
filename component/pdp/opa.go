package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/bundles"
	"github.com/open-policy-agent/opa/types"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/logging"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/sdk"
)

var opaBundleBaseURL = "http://localhost:8081/pdp/bundles/"

// createOPAService creates a new OPAService service with embedded policy bundles
func (c *Component) createOPAService(ctx context.Context) (*sdk.OPA, error) {
	rego.RegisterBuiltin1(&rego.Function{
		Name: "mitz.has_consent",
		Decl: types.NewFunction(types.Args(types.A), types.B),
	}, func(bctx rego.BuiltinContext, inputArg *ast.Term) (*ast.Term, error) {
		var input PolicyInput
		inputMap := map[string]any{}
		if err := ast.As(inputArg.Value, &inputMap); err != nil {
			return nil, err
		}
		if inputData, err := json.Marshal(inputMap); err != nil {
			return nil, fmt.Errorf("failed to marshal input map: %w", err)
		} else if err := json.Unmarshal(inputData, &input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
		}

		policyResult := c.evalMitzPolicy(bctx.Context, input)
		if !policyResult.Allow {
			slog.InfoContext(bctx.Context, "mitz.has_consent evaluated to false", "reasons", policyResult.Reasons)
		}
		return ast.BooleanTerm(policyResult.Allow), nil
	})

	configBundles := map[string]any{}
	for bundleName := range bundles.BundleMap {
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
func (c Component) evalRegoPolicy(ctx context.Context, scope string, policyInput PolicyInput) (*PolicyResult, error) {
	// get the named policy decision for the specified input
	result, err := c.opaService.Decision(ctx, sdk.DecisionOptions{Path: "/" + scope + "/allow", Input: policyInput})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate policy: %w", err)
	} else if _, ok := result.Result.(bool); !ok {
		return nil, fmt.Errorf("unexpected policy result type (expected bool, was %T)", result.Result)
	}

	allowed := result.Result.(bool)
	policyResult := PolicyResult{
		Allow: allowed,
	}
	if !allowed {
		policyResult.Reasons = []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: "access denied by policy",
			},
		}
	}
	return &policyResult, nil
}
