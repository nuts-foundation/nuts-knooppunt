# LSP Integration

This document describes the fit-gap analysis for integrating the Knooppunt (Nuts) with LSP/AORTA,
covering two flows defined in the LSPxNuts PSA (section 8.10–8.11):

1. **Nuts querying AORTA** — outbound: Nuts requests an access token at AORTA's authorization server
2. **AORTA querying Nuts** — inbound: AORTA requests an access token at a Nuts authorization server

Per PSA section 10.9, AORTA uses the `urn:ietf:params:oauth:grant-type:jwt-bearer` grant type (RFC 7523)
with two separate JWT assertions carrying Verifiable Presentations:

| Parameter               | Role              | VP contents                                                                                                                |
|-------------------------|-------------------|----------------------------------------------------------------------------------------------------------------------------|
| `assertion`             | Care organization | `HealthcareOrganizationCredential`, `HealthCareProfessionalDelegationCredential`, optionally `PatientEnrollmentCredential` |
| `client_assertion`      | Vendor            | `ServiceProviderCredential`, `ServiceProviderDelegationCredential`                                                         |
| `client_assertion_type` | —                 | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer`                                                                   |

---

## Fit-Gap

| PSA requirement                                                 | PSA ref | Outbound (Nuts → AORTA)                              | Inbound (AORTA → Nuts)                                 | Issue                                                                      |
|-----------------------------------------------------------------|---------|------------------------------------------------------|--------------------------------------------------------|----------------------------------------------------------------------------|
| `jwt-bearer` grant type (RFC 7523)                              | 10.9.3  | Supported                                            | **Gap**                                                | [nuts-node#4080](https://github.com/nuts-foundation/nuts-node/issues/4080) |
| Multiple scopes in token request                                | 10.9    | Supported (via `policy_id`)                          | **Gap**                                                | [nuts-node#4144](https://github.com/nuts-foundation/nuts-node/issues/4144) |
| Dual-assertion token request (`assertion` + `client_assertion`) | 10.9.6  | Out of scope — client_assertion accepted but ignored | Out of scope — `client_assertion` accepted but ignored | —                                                                          |
| `PatientEnrollmentCredential` validation                        | 10.6.5  | Supported (needs validation)                         | Supported (needs validation)                           | —                                                                          |
| `HealthcareOrganizationCredential` validation                   | 10.6.3  | Supported (needs validation)                         | Supported (needs validation)                           | [nuts-node#4079](https://github.com/nuts-foundation/nuts-node/issues/4079) |
| `HealthCareProfessionalDelegationCredential` validation         | 10.6.4  | Supported (needs validation)                         | Supported (needs validation)                           | —                                                                          |
| `ServiceProviderCredential` validation                          | 10.6.6  | Out of scope                                         | Out of scope                                           | —                                                                          |
| `ServiceProviderDelegationCredential` validation                | 10.6.7  | Out of scope                                         | Out of scope                                           | —                                                                          |
| OAuth scopes (specific values)                                  | 10.9    | TBD                                                  | TBD                                                    | —                                                                          |
