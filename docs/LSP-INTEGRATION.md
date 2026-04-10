# LSP Integration

This document describes the fit-gap analysis and required changes for integrating the Knooppunt (Nuts)
with LSP/AORTA, covering two flows defined in the LSPxNuts PSA (section 8.10–8.11):

1. **Nuts querying AORTA** — outbound: Nuts requests an access token at AORTA's authorization server
2. **AORTA querying Nuts** — inbound: AORTA requests an access token at a Nuts authorization server

---

## Fit-Gap

### Token grant

Per PSA section 10.9, AORTA uses the `urn:ietf:params:oauth:grant-type:jwt-bearer` grant type (RFC 7523)
with two separate JWT assertions carrying Verifiable Presentations:

| Parameter               | Role              | VP contents                                                                                                                |
|-------------------------|-------------------|----------------------------------------------------------------------------------------------------------------------------|
| `assertion`             | Care organization | `HealthcareOrganizationCredential`, `HealthCareProfessionalDelegationCredential`, optionally `PatientEnrollmentCredential` |
| `client_assertion`      | Vendor            | `ServiceProviderCredential`, `ServiceProviderDelegationCredential`                                                         |
| `client_assertion_type` | —                 | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer`                                                                   |

| Capability                                                                  | Nuts querying AORTA (outbound)      | AORTA querying Nuts (inbound)                          |
|-----------------------------------------------------------------------------|-------------------------------------|--------------------------------------------------------|
| `urn:ietf:params:oauth:grant-type:jwt-bearer` (RFC 7523, PSA 10.9.3)        | **Gap** — not yet supported by Nuts | **Gap** — not yet supported by Nuts                    |
| Dual-assertion token request (`assertion` + `client_assertion`, PSA 10.9.6) | Out of scope                        | Out of scope — `client_assertion` accepted but ignored |

### Scopes

The PSA defines specific OAuth `scope` values for token requests (PSA 10.9). Fit-gap analysis for scope handling is **TBD**.

### Verifiable Credentials

| Credential                                   | VP                 | PSA section | Nuts querying AORTA (outbound)      | AORTA querying Nuts (inbound)       |
|----------------------------------------------|--------------------|-------------|-------------------------------------|-------------------------------------|
| `PatientEnrollmentCredential`                | `assertion`        | 10.6.5      | Supported, not validated — TBD      | Supported, not validated — TBD      |
| `HealthcareOrganizationCredential`           | `assertion`        | 10.6.3      | Supported, not validated — TBD      | Supported, not validated — TBD      |
| `HealthCareProfessionalDelegationCredential` | `assertion`        | 10.6.4      | Supported, not validated — TBD      | Supported, not validated — TBD      |
| `ServiceProviderCredential`                  | `client_assertion` | 10.6.6      | Out of scope                        | Out of scope                        |
| `ServiceProviderDelegationCredential`        | `client_assertion` | 10.6.7      | Out of scope                        | Out of scope                        |

---

## Required Changes

### [nuts-foundation/nuts-node#4079](https://github.com/nuts-foundation/nuts-node/issues/4079) — Generic did:x509 credential validation

Prerequisite for both token grant issues below.

Currently, Nuts only validates credentials typed as `X509Credential` against did:x509-specific rules
(certificate chain, CRL, issuer-DID-to-subject attribute matching). New credential types with a did:x509
issuer — including `HealthcareOrganizationCredential` — fall through to the default validator and cannot be
properly verified. This issue implements the generic validation rules from PSA 10.6.2 for all credential
types regardless of their `type` field.

Gaps resolved: `HealthcareOrganizationCredential` validation (TBD — may already be supported).

### [nuts-foundation/nuts-node#4078](https://github.com/nuts-foundation/nuts-node/issues/4078) — Client-side RFC 7523 JWT Bearer grant (Nuts querying AORTA)

Nuts currently requests access tokens using the `vp_token-bearer` grant type (RFC021) with a single VP.
This issue extends the Nuts node to request tokens using the RFC 7523 `jwt-bearer` grant type with a
single VP (care organization, `assertion`) as defined in PSA 10.9.3. The `client_assertion` VP is out of scope.

Gaps resolved: outbound `jwt-bearer` grant.

Depends on: [nuts-foundation/nuts-node#4079](https://github.com/nuts-foundation/nuts-node/issues/4079).

### [nuts-foundation/nuts-node#4080](https://github.com/nuts-foundation/nuts-node/issues/4080) — Server-side RFC 7523 JWT Bearer grant (AORTA querying Nuts)

Nuts' authorization server currently only accepts the `vp_token-bearer` grant type with a single VP.
This issue extends the AS to also accept the RFC 7523 `jwt-bearer` grant type where the token request
contains two VPs as defined in PSA 10.9.6, and validates both according to the credential-specific rules
from PSA 10.6.

The `client_assertion` VP is accepted but not validated (out of scope).

Gaps resolved: inbound `jwt-bearer` grant, inbound validation of `HealthcareOrganizationCredential` (TBD — may already be supported).

Depends on: [nuts-foundation/nuts-node#4079](https://github.com/nuts-foundation/nuts-node/issues/4079), [nuts-foundation/nuts-node#4078](https://github.com/nuts-foundation/nuts-node/issues/4078).
