//go:generate mockgen -destination=component_mock.go -package=mitz -source=interface.go
package mitz

import (
	"context"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

// ConsentChecker defines the public API for the MITZ component
type ConsentChecker interface {
	// CheckConsent triggers a consent check by invoking MITZ closed query.
	// It takes an AuthzRequest containing all required parameters for the consent check.
	// Returns an XACMLResponse containing the decision (Permit/Deny/NotApplicable/Indeterminate) and the full XML response.
	CheckConsent(ctx context.Context, authzReq xacml.AuthzRequest) (*xacml.XACMLResponse, error)
}

var _ ConsentChecker = (*Component)(nil)
