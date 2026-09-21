package subscription

import (
	"context"
	"log/slog"
)

// Authorization and consent verification hooks. The spec's implementation obligations 2
// and 3 require the server to verify, before activating a Subscription and again
// immediately before sending each notification, that the receiver is authorized and that
// the consent conditions for the payload mode are met.
//
// Real GF Authorization / Mitz consent evaluation is out of POC scope; what the POC
// demonstrates is the *hook placement* (research question 5): both hooks run at exactly
// the spec-mandated moments, log their invocation, and consult the vendor-registered
// authorization bases (POST /subscription/authorizations, per the vendor split in doc
// 09) — but always allow, logging whether a basis was found rather than enforcing it.

// verifyAuthorization is called before subscription activation and before each
// notification send. checkpoint is "activation" or "notification-send".
func (c *Component) verifyAuthorization(ctx context.Context, sub serverSubscription, checkpoint string) bool {
	basisFound := c.store.findAuthorizationBasis(sub.OwnerURAFilter(), sub.Topic)
	slog.InfoContext(ctx, "Authorization verification hook invoked (POC stub: always allows)",
		slog.String("checkpoint", checkpoint),
		slog.String("subscription", sub.ID),
		slog.String("receiverUra", sub.OwnerURAFilter()),
		slog.String("topic", sub.Topic),
		slog.Bool("registeredBasisFound", basisFound))
	return true
}

// verifyConsent is called immediately before each notification send; the consent
// conditions to verify depend on the payload mode (id-only is itself a disclosure).
func (c *Component) verifyConsent(ctx context.Context, sub serverSubscription) bool {
	slog.InfoContext(ctx, "Consent verification hook invoked (POC stub: always allows)",
		slog.String("subscription", sub.ID),
		slog.String("payloadMode", string(sub.Mode)))
	return true
}
