# Authentication API

## Context and Problem Statement

We need to decide if, and how, EHRs will interact with the Knooppunt for authentication towards remote EHR systems.

It could be used for the following use cases:

1. Machine-to-machine authentication of a care organization, for usage in data exchanges with other care organizations.
2. End-user authentication of a caregiver using DEZI, for:
    - Logging into the local EHR, and
    - Data exchanges with other care organizations.

### Design Goals

We want a solution that is easy to integrate in varying (existing) environments, without compromising on security and
simplicity.

Key design goals are:

- **Pluggability**: Should be as easy as possible to integrate, e.g. with existing client library support.
- Existing Nuts implementations could be leveraged, if possible.

## Considered Options

This section describes the considered options.

### No Knooppunt authentication API

In this option, the Knooppunt does not provide any authentication API.

EHR vendors will have to integrate the Nuts node v2 authentication API and DEZI directly in their EHR systems.

### Nuts v2 auth API

If the GF Authentication specifies Nuts as authentication mechanism, the EHR can use the Nuts v2 authentication API to
obtain access tokens for remote EHR systems.

It's a tailor-made REST API of the Nuts node, but can be used to

Advantages:

- Vendors with existing Nuts node implementations can use their existing implementation.

Disadvantages:

- Tailor-made API, so requires custom client implementation.
- Only works for Nuts-based authentication.
- The EHR will still need to integrate DEZI (an OpenID Connect API) for caregiver authentication.
- The DEZI id_token will have to be wrapped in a Verifiable Credential for usage in Nuts, adding complexity.

### OAuth2 / OpenID Connect

The Knooppunt could expose an OAuth2 / OpenID Connect (OIDC) API for end-user, and machine-to-machine authentication,
using standardized protocols

Advantages:

- Standard protocol, with existing client libraries for many programming languages.
- Supports multiple grant types, allowing for both machine-to-machine and end-user authentication.

Disadvantages:

- More complexity in the Knooppunt and deployment; it requires the Knooppunt to implement an OAuth2 / OIDC Provider,
  and the vendor to configure OAuth2/OIDC clients in their EHR systems and Knooppunt.

#### End-user authentication

For end-user authentication, the **OIDC Authorization Code** flow can be used to authenticate caregivers using DEZI.

Although DEZI looks like a standard OpenID Connect Provider implementation, it could do heavy lifting like decrypting
the `id_token`.

Note: supporting this OIDC flow is optional; the EHR could choose to directly integrate with DEZI, using the decrypted
`id_token` for machine-to-machine authentication.

#### Machine-to-machine authentication

In machine-to-machine authentication, the care organization is always authenticated.
In cases the data exchange requires a user identity, the pre-authenticated end-user is also included in the form of a
decrypted DEZI `id_token`.

There are several OAuth2 grant types that can be used for machine-to-machine authentication:

##### OAuth2 Client Credentials

The EHR authenticates to the Knooppunt using its `client_id` and static `client_secret` to obtain an access token,
providing a custom parameter for the end-user DEZI `id_token` if needed.

Example token exchange request:

```
grant_type=client_credentials
 &scope=<requested scopes>
 &client_id=<EHR client ID>
 &client_secret=<EHR client secret>
 &dezi_id_token=<DEZI id_token> (optional)
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
  (e.g., DEZI `id_token`s) are manually encoded and included.
- Limited delegation model: No standard way to represent "on behalf of" relationships or token chaining between systems.

##### OAuth2 JWT Bearer Grant (RFC7523)

Using this grant type EHR authenticates to the Knooppunt using a signed JWT assertion, which can include both the care
organization
and end-user identity (from the DEZI `id_token`).

Semantically identical to Client Credentials, but uses a signed JWT for authentication instead of static client secrets,
which makes it a bit easier to include end-user identity (a JSON object) in the JWT claims.

Advantages:

- More flexible: allows inclusion of additional claims (like end-user identity) in the JWT assertion without manual
  encoding of JSON objects to strings.
- Improved security: leverages asymmetric cryptography for authentication, reducing risks associated with static
  secrets.

Disadvantages:

- Increased complexity: requires JWT creation and signing logic, which may not be natively supported in all OAuth2
  libraries.
- Still limited delegation model: while more flexible than Client Credentials, it still lacks standardized support for
  complex "on behalf of" scenarios.

##### OAuth 2.0 Token Exchange (RFC8693)

[OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693.html) is a newer OAuth2 grant type that allows one token to be exchanged for another,
supporting "on behalf of" scenarios.
Using this flow, the EHR can exchange the DEZI `id_token` at the Knooppunt for an access token to a remote EHR system,
representing both the care organization and the authenticated caregiver.

The `subject_token` parameter is optional and can be omitted if no user is present.

Example token exchange request:

```
grant_type=urn:ietf:params:oauth:grant-type:token-exchange
 &audience=<remote EHR system (remote OAuth2 issuer URL)>
 &subject_token=<DEZI id_token>
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

- Limited library support: being a newer specification, it may not be widely supported in existing OAuth2 libraries and
  frameworks.

## Decision Outcome

Proposal:

- Optional OIDC Provider in the Knooppunt for end-user authentication using DEZI.
- OAuth2 API in the Knooppunt for machine-to-machine authentication, supporting at least:
    - OAuth2 Client Credentials grant.
    - OAuth2 Token Exchange if supported by enough vendors.