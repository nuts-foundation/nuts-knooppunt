# Authentication API

## Context and Problem Statement

We need to decide how clients and end-users authenticate to the Knooppunt,
and what features the Knooppunt offers to vendors to ease integration of GF Authentication.

This ADR addresses the following scenarios:

- Logging in: end-user authenticates to the local EHR using Dezi.
- Data exchange: EHR performing data exchange with external system, using care organization identity.
- Data exchange: EHR performing data exchange with external system, using both care organization and end-user identity from Dezi.
- API authentication: EHR using Knooppunt APIs.

### Design Goals

We want a solution that is easy to integrate in varying (existing) environments, without compromising on security and
simplicity.

Key design goals are:

- **Pluggability**: Should be as easy as possible to integrate, e.g. with existing client library support.
- Existing Nuts implementations could be leveraged, if possible.
- **Security**: Should promote secure deployments.
- **Simplicity**: Should be as simple as possible to implement and deploy.

## Decision outcome

We embrace OAuth2 and OpenID Connect, and leverage existing Nuts APIs:

- Logging in:
  - Introduce OpenID Connect Provider for logging in users through Dezi. It will be an abstraction for Dezi.
  - It'll return an `id_token` with standard claims, derived from the Dezi `id_token`.
  - The `id_token` can be used directly in APIs to authenticate the data exchanges.
- Data exchange:
  - We extend the existing Nuts Auth v2 API
  - The EHR can then pass in the Dezi `id_token` when obtaining access tokens for remote EHR systems.
- API authentication:
  - We introduce an optional OAuth2 Client Credentials flow for EHRs to authenticate to the Knooppunt APIs.

## Considered Options

This section describes the considered options.

### Logging in

This section describes options for EHRs logging in end-users using Dezi.

#### Logging in: no authentication API

In this option, the Knooppunt does not provide an end-user authentication API.

EHR vendors will have to integrate the Nuts node v2 authentication API and Dezi directly in their EHR systems.

#### Logging in: OpenID Connect Provider as Dezi abstraction

The Knooppunt could expose an OpenID Connect (OIDC) API for end-user authentication, using standardized protocols.

Although Dezi looks like a standard OpenID Connect Provider implementation, it has non-standard claims,
and requires the client to decrypt `id_token`s and perform non-standard token validation checks (LoA claim level and revocation checking).
The Knooppunt can offload this complexity from the EHR.

Advantages:

- Standard protocol, with existing client libraries for many programming languages.
- Supports multiple grant types, allowing for both machine-to-machine and end-user authentication.

Disadvantages:

- More complexity in the Knooppunt and deployment; it requires the Knooppunt to implement an OAuth2 / OIDC Provider,
  and the vendor to configure OAuth2/OIDC clients in their EHR systems and Knooppunt.

Note: supporting this OIDC flow is optional; the EHR could choose to directly integrate with Dezi, using the decrypted
`id_token` for machine-to-machine authentication.


### Data exchange

This section describes options for EHRs performing data exchange with remote EHR systems, using a care organization identity
and optional end-user identity from Dezi.

#### Data Exchange: Nuts v2 auth API

If Nuts aligns with the GF Authentication, the EHR can use the embedded Nuts v2 authentication API to obtain access tokens for remote EHR systems.

Advantages:

- Vendors with existing Nuts node implementations can use their existing implementation.

Disadvantages:

- Tailor-made API, so requires custom client implementation.
- Only works for Nuts-based authentication.
- The Dezi id_token will have to be wrapped in a Verifiable Credential for usage in Nuts, adding complexity,
- or: the Nuts API will have to be extended to support Dezi id_tokens directly.

#### Data Exchange: OAuth2 Client Credentials

The EHR authenticates to the Knooppunt using its `client_id` and static `client_secret` to obtain an access token,
providing a custom parameter for the end-user Dezi `id_token` if needed.

Example token exchange request:

```
grant_type=client_credentials
 &scope=<requested scopes>
 &client_id=<EHR client ID>
 &client_secret=<EHR client secret>
 &dezi_id_token=<Dezi id_token> (optional)
 &nuts_subject_id=<Nuts subject ID>
```

Advantages:

- Simplicity: widely understood and easy to implement. Most OAuth2 libraries and frameworks natively support this flow.
- Mature and standardized: broad industry adoption and extensive tooling/support.
- Good for pure system-level communication: ideal when only the organization (not an end-user) needs to be
  authenticated.
- Low integration overhead: minimal requirements for token construction or signing logic.

Disadvantages:

- Static credentials: requires secure management of client secrets,
  which can be hard in distributed or multi-tenant systems.
- No built-in user context: can't represent a caregiver or end-user identity unless additional tokens
  (e.g., Dezi `id_token`s) are manually encoded and included.
- Limited delegation model: No standard way to represent "on behalf of" relationships or token chaining between systems.

#### Data Exchange: OAuth2 JWT Bearer Grant (RFC7523)

Using this grant type EHR authenticates to the Knooppunt using a signed JWT assertion, which can include both the care
organization
and end-user identity (from the Dezi `id_token`).

Semantically identical to Client Credentials, but uses a signed JWT for authentication instead of static client secrets,
which makes it a bit easier to include end-user identity (a JSON object) in the JWT claims.

Advantages:

- More flexible: allows inclusion of additional claims (like end-user identity) in the JWT assertion without manual
  encoding of JSON objects to strings.
- Improved security: leverages asymmetric cryptography for authentication, reducing risks associated with static
  secrets.

Disadvantages:

- Another endpoint for the same task; the Nuts node already has an API for this (or existing integrations have to migrate).
- Increased complexity: requires JWT creation and signing logic, which may not be natively supported in all OAuth2 libraries.
- Still limited delegation model: while more flexible than Client Credentials, it still lacks standardized support for
  complex "on behalf of" scenarios.

#### Data Exchange: OAuth 2.0 Token Exchange (RFC8693)

[OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693.html) is a newer OAuth2 grant type that allows one token to be exchanged for another,
supporting "on behalf of" scenarios.
Using this flow, the EHR can exchange the Dezi `id_token` at the Knooppunt for an access token to a remote EHR system,
representing both the care organization and the authenticated caregiver.

The `subject_token` parameter is optional and can be omitted if no user is present.

Example token exchange request:

```
grant_type=urn:ietf:params:oauth:grant-type:token-exchange
 &audience=<remote EHR system (remote OAuth2 issuer URL)>
 &subject_token=<Dezi id_token>
 &subject_token_type=urn:ietf:params:oauth:token-type:id_token
 &actor_token=<Nuts subject ID>
 &actor_token_type=nuts-subject-id
 &requested_token_type=urn:ietf:params:oauth:token-type:access_token
 &scope=<requested scopes>
 &client_id=<EHR client ID>
 &client_secret=<EHR client secret>
```

Advantages:

- Very good fit for "on behalf of" scenarios: designed specifically to handle cases where an application needs to act on
  behalf of a user.

Disadvantages:

- Another endpoint for the same task; the Nuts node already has an API for this (or existing integrations have to migrate).
- Limited library support: being a newer specification, it may not be widely supported in existing OAuth2 libraries and frameworks.
- Depends heavily on unspecified semantics of `actor` and `subject` tokens, which might not be easier to integrate than a custom API in practice.

### API authentication

This section describes options for EHRs authenticating to their local Knooppunt APIs.

#### API authentication: OAuth2 Client Credentials

The EHR authenticates to the Knooppunt using its `client_id` and secret (can be static at first, JWT-based later) to obtain an access token.

This makes it easier to deploy the Knooppunt in a secure way, since vendors need to worry less about API security.

We could introduce multiple scope types for different API areas/clients, e.g.:

- Proxies/Policy Enforcement Points need access to:
  - Token Introspection API
  - Policy Decision Point API
- EHRs need access to the Knooppunt FHIR APIs, but not the ones above.

Advantages:

- Standard protocol, with existing client libraries for many programming languages.
- Fine-grained access control using scopes.

Disadvantages:

- More complexity in the Knooppunt and deployment; it requires the Knooppunt to implement an OAuth2 Provider,
  and the vendor to configure OAuth2 clients in their EHR systems and Knooppunt.

#### API authentication: no authentication

EHR vendors would have to secure the Knooppunt APIs in some other way, e.g. by restricting network access.

Advantages:

- Simplicity: no additional authentication mechanisms to implement or manage.
- Easier EHR integration: no need to implement OAuth2 client logic in the EHR systems.

Disadvantages:

- Security risks: without proper authentication, the Knooppunt APIs could be exposed to unauthorized access.
  This increases deployment complexity, as vendors need to ensure secure network configurations.