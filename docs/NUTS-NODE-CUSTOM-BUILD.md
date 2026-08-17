# Nuts Node Custom Build

Knooppunt embeds a custom build of the Nuts node, built from the `project-gf` / `project-gf-pilot`
branches of [nuts-foundation/nuts-node](https://github.com/nuts-foundation/nuts-node) rather than
`master`. Those branches carry feature work that either hasn't been upstreamed yet or was dropped
during a `master` merge and kept only for this custom build.

This document tracks that gap so upgrades to a `master`-based Nuts node don't silently lose
functionality Knooppunt relies on. Status was last verified against upstream on 2026-08-17.

Sources:
- [nuts-node#3983](https://github.com/nuts-foundation/nuts-node/pull/3983) — `project-gf` (PoC phase), merges the feature branches listed below
- [nuts-node#4308](https://github.com/nuts-foundation/nuts-node/pull/4308) — `project-gf-pilot`, adds further features on top of `project-gf`

## Features not on `master`

| Feature | Description | Master status |
|---|---|---|
| Dezi ID-token credential support | Validates end-user credentials (`DeziUserCredential`) presented via a DEZI `id_token` in the OAuth token request (`vcr/credential/dezi.go`). | [nuts-node#3981](https://github.com/nuts-foundation/nuts-node/pull/3981) — closed unmerged, folded into `project-gf` directly |
| Configurable revocation-list max-age | Lets the status-list cache max-age be configured instead of hardcoded. | [nuts-node#3992](https://github.com/nuts-foundation/nuts-node/pull/3992) — closed unmerged |
| Server-side RFC 7523 JWT-bearer handler + `auth.granttypes` config | `master` only has the client-side two-VP JWT-bearer flow ([nuts-node#4275](https://github.com/nuts-foundation/nuts-node/pull/4275), merged). The custom build additionally keeps a server-side handler for this grant type and a configurable list of supported grant types. | Not on master |
| `OpenIdCredentialIssuerMetadata` / `VerifiableCredentials` OpenID4VCI client methods | Extra methods on the `auth/client/iam` client interface for fetching issuer metadata and requesting credentials directly. | Not on master (verified against `auth/client/iam/interface.go`) |
| Access-token cache disabled | The OAuth access-token cache is turned off so credential-revocation testing behaves deterministically. | Not on master (deliberately not upstreamable — testing aid) |
| Revocation/expiry demo behavior in wallet | `wallet.List()` includes revoked/expired credentials, and status-list credentials are always refetched (cache bypass). | Not on master (demo aid) |
| Log credential-issuer response body on non-200 | Extra diagnostic logging when a credential issuer returns an error. | Not on master |
| `credential_details` as base body of the OpenID4VCI Credential Request | `master` merged a related but differently-shaped feature: `credential_request_params` applied as an *overlay* on the request ([nuts-node#4236](https://github.com/nuts-foundation/nuts-node/pull/4236), merged). The custom build's version applies `credential_details` as the request's *base body* instead — behavior differs, not a straight superset. | Related feature merged, semantics differ |
| Configurable OAuth client authentication for the OpenID4VCI auth-code flow | | [nuts-node#4327](https://github.com/nuts-foundation/nuts-node/pull/4327) — open, unmerged |
| `http.client.log` — log outgoing HTTP requests | | [nuts-node#4337](https://github.com/nuts-foundation/nuts-node/pull/4337) — open, unmerged |
| Configurable OpenID4VCI request profiles, with a built-in AET profile | The custom build additionally aligns the AET profile's scope to `DefaultConfig`, which isn't reflected in the upstream PR. | [nuts-node#4340](https://github.com/nuts-foundation/nuts-node/pull/4340) — open, unmerged |
| Client-facing "credential is revoked" error message | Originated from a branch (`copilot/improve-client-error-message`) that no longer exists upstream; current `master` status could not be confirmed. | Unverified |

## Already merged into `master`

These were called out in the `project-gf-pilot` PR as gaps but have since landed upstream — no action needed:

- Metadata discovery: support both insert (RFC 8414) and append (OIDC) well-known locations, tolerate a trailing slash in the identifier match — [nuts-node#4346](https://github.com/nuts-foundation/nuts-node/pull/4346), merged 2026-07-03
- Remove non-conformant `Display` from issuer metadata — [nuts-node#4347](https://github.com/nuts-foundation/nuts-node/pull/4347), merged 2026-06-30
- Client-side RFC 7523 JWT-bearer grant with two VPs — [nuts-node#4275](https://github.com/nuts-foundation/nuts-node/pull/4275), merged 2026-05-26

## Not gaps (intentionally not adopted)

- `authorization_request_params` on the OpenID4VCI `requestCredential` API — merged into `master` as [nuts-node#4333](https://github.com/nuts-foundation/nuts-node/pull/4333), but deliberately not carried into `project-gf-pilot`; the AET `auth_method=SmartCard` use case is covered by the request-profiles work above instead.
- `policy_id` on the JWT-bearer request — dropped from the custom build when its JWT-bearer implementation was reconciled with `master`'s; the presentation definition is now resolved from `scope` on both.

## Dev/test tooling only (not node features)

- `lspxnuts` — local Docker Compose setup and certs for testing LSP integration (`development/lspxnuts/*`)
- LSP CA certificate bundle swapped from Sectigo Public to PKIoverheid Private
- `gorm` pinned at v1.30.2

## Keeping this up to date

When rebasing the custom build onto a newer `master`, or when any of the "open, unmerged" upstream
PRs above merge, re-check this table — either drop the merged item or move it to the "already
merged" section.
