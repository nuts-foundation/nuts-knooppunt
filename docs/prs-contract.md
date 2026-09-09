# PRS wire contract

`component/pseudonymisation` turns a BSN into the pseudonymous identifier the
NVI expects by calling the national pseudonymization service (PRS,
"Pseudoniemendienst"). Three descriptions of that service exist, and they
disagree. This document puts them side by side, states what the Knooppunt sends
today, and says whether that matches the version the acceptance environment
runs. It is the checklist for the day acceptance is upgraded.

The tests that pin the Knooppunt side are in `component/pseudonymisation`
(`component_test.go`, `blinding_test.go`); they run against `test/prsmock`,
which models the acceptance version.

## The three dialects

| Dialect | Source | Status |
|---|---|---|
| **Acceptance, v0.0.18** | [minvws/gfmodules-pseudoniemendienst](https://github.com/minvws/gfmodules-pseudoniemendienst) at tag `v0.0.18` (commit `990cf81`, 2026-02-24). File references below are into that tag unless stated otherwise. | What the Proeftuin runs. [`data/services.yaml`](https://github.com/minvws/gfmodules-technische-documentatie/blob/main/data/services.yaml) of minvws/gfmodules-technische-documentatie (commit `5a0027a`, 2026-09-03) lists PRS `v0.0.18` at `https://pseudoniemendienst.proeftuin.gf.irealisatie.nl` with `POST /oprf/eval` behind "mTLS + bearer token" (lines 41, 51, 53); `data/changelog.yaml` dates the initial Proeftuin release 2026-05-11 and records no PRS upgrade since. The Knooppunt client was written against this version (commit `3781ecb`, 2026-02-25). |
| **Service `main`, v0.0.34** | Same repository, `main` at `db1160b` (2026-09-08); latest tag `v0.0.34` (commit `ea91e01`, 2026-09-04). | Not deployed to acceptance. Breaks the current client in two places (organization identifier, scope). |
| **Draft IG** | [minvws/generiekefuncties-docs](https://github.com/minvws/generiekefuncties-docs), `input/pagecontent/pseudonymisation.md` (created in `29f8ed5`, 2026-04-21) and `input/pagecontent/localization.md`; published at [pseudonymisation.html](https://minvws.github.io/generiekefuncties-docs/en/pseudonymisation.html) (IG v0.10.0 ci-build, generated 2026-08-27). | Page status "Draft". Its endpoint and field names have never existed in the service, at any tag, nor in the ministry's own PRS stub (minvws/gfmodules-pseudoniemendienst-stub). Do not implement. |

The recipient side is governed by the NVI, not the PRS:
[minvws/gfmodules-nationale-verwijsindex](https://github.com/minvws/gfmodules-nationale-verwijsindex)
(`6930872`, 2026-09-08) and
[minvws/gfmodules-nationale-verwijsindex-crypto-service-api](https://github.com/minvws/gfmodules-nationale-verwijsindex-crypto-service-api)
(`e099e82`, 2026-09-08). The OPRF library behind the service is
[stef/liboprf](https://github.com/stef/liboprf) (`f925b87`, 2026-09-01) through
its `pyoprf` binding.

## Element by element

"Knooppunt sends" cites this repository. "Match" is against acceptance v0.0.18.

| Element | Acceptance v0.0.18 | Service `main` (v0.0.34) | Draft IG | Knooppunt sends | Match |
|---|---|---|---|---|---|
| **Endpoint** | `POST /oprf/eval` (`app/routers/oprf.py:14`) | `POST /oprf/eval` (`app/routers/oprf.py:28-29`) | `POST /evaluate` (`pseudonymisation.md:79`) | `POST {prsurl}/oprf/eval` (`component/pseudonymisation/component.go:151`) | Yes |
| **Request: blinded element** | `encryptedPersonalId`, string, at least 2 characters, must decode as base64url after padding (`app/services/oprf/oprf_service.py:13,17-26`). Evaluation decodes it again *without* the padding fix (`oprf_service.py:45`), so in practice the value must be padded. | `encryptedPersonalId`; the validator now returns the padded value (`app/models/requests.py:151-165`), so unpadded input is accepted. | `blinded_input`, "base64url-encoded" (`pseudonymisation.md:85`) | `encryptedPersonalId`: the 32-byte ristretto255 element as Go's `encoding/json` writes a `[]byte`, standard alphabet with padding (`component.go:122,139`) | Yes, by tolerance: `base64.urlsafe_b64decode` maps `-`/`_` to `+`/`/` and lets `+`/`/` through, so standard base64 decodes correctly (verified on CPython 3, and `test/prsmock` models it). Not byte-identical to what the reference client sends (`oprf/client.py:52`, urlsafe). |
| **Request: recipient organization** | `recipientOrganization`, string, must start with `ura:` or 400 `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`; the rest is looked up as the organization's URA, 404 if unknown (`oprf.py:22-30`). Organizations are created with an 8-digit URA (`app/models/requests.py:16`). | `recipientOrganization` is a `RecipientOrganizationOin`: `oin:` followed by a 20-character OIN (`app/models/oin.py:10-17,142-211`); anything else is a 422 with "Invalid recipient organization. Format: oin:<oin_number>". Introduced by "Add type for RecipientOrganization" (`3687f45`, 2026-06-29) after "deprecate all UraNumber modules and usage" (`5ea4910`, 2026-06-25); shipped from tags v0.0.22 and v0.0.24. | `recipient_organization`, example `"ura:90000901"` (`pseudonymisation.md:86`) | `"ura:" + recipientURA` (`component.go:137`), where `recipientURA` is `nvi.audience` from the configuration | Yes. **Breaks on `main`**: nobody has OIN values for the NVI or the demo organizations yet. |
| **Request: recipient scope** | `recipientScope`, string, at least 2 characters; a public key must be registered for the organization under exactly this scope or 404 `{"error":"No public key found for this organization and/or scope"}` (`oprf_service.py:15`, `oprf.py:32-35`) | Same (`requests.py:154`) | `recipient_scope` (`pseudonymisation.md:87`) | `nationale-verwijsindex` (`component/nvi/component.go:359`) | Yes, provided the NVI registered its key under that scope on acceptance. |
| **Response** | `{"jwe": "<compact JWE>"}` (`oprf.py:44`) | Same key (`docs/endpoints.md:159-165`); the JWE payload gains an `extra_versions` claim during key rotation (`oprf_service.py:78-100`, `docs/endpoints.md:167`) | `{"evaluated_output": "<JWE compact serialization>"}` (`pseudonymisation.md:97-99`) | Reads `jwe` (`component.go:126`) and passes it on unopened | Yes |
| **JWE contents** (opaque to the client) | Protected header `alg` RSA-OAEP-256, `enc` A256GCM, `cty` application/json, `kid` = RFC 7638 thumbprint of the recipient key; claims `subject` = `pseudonym:eval:<padded base64url of the evaluated element>`, `aud` = the `recipientOrganization` string, `scope`, `version` "1.1", `iat`, `exp` = `iat` + 300 s (`app/services/oprf/jwe_token.py:13-29`, `oprf_service.py:51-57`) | Same, plus `extra_versions` and an optional configured `kid` (`docs/endpoints.md:27,167`) | "JWE compact serialization ... opaque to the client" | Forwarded verbatim as `evaluated_output` | Not applicable; the Knooppunt never opens it. |
| **Bytes: blind factor** (to the recipient) | Read by the recipient with `base64.urlsafe_b64decode` (service test receiver `oprf_service.py:83`; NVI crypto service `app/services/pseudonym_service.py:42`), 32-byte little-endian ristretto255 scalar (`liboprf python/pyoprf/__init__.py:145-156`). The acceptance documentation's own NVI example encodes it with standard `base64.b64encode` (`themes/gfmodules/layouts/page/integration-nvi.html:251`). | Same | "base64url-encoded blind_factor" (`localization.md:52`) | Standard base64 with padding, 32 bytes (`component.go:80,98,103`); scalar bytes are little-endian (go-ristretto `scalar.go:34-75`) | Yes, by the same decoder tolerance, and identical to the acceptance documentation's example. |
| **NVI identifier** (`List.subject.identifier`) | Governed by the NVI: system `http://minvws.github.io/generiekefuncties-docs/NamingSystem/nvi-identifier` (`app/models/fhir/resources/data.py:7`); value decoded with `base64.urlsafe_b64decode` after adding `=` padding (`app/utils/fhir.py:6-10`), members `evaluated_output` and `blind_factor` (`app/services/pseudonym_resolver.py:45-61`) | Same | `base64url(JSON{evaluated_output, blind_factor})` (`localization.md:47-56`); acceptance documentation shows unpadded base64url of compact JSON (`integration-nvi.html:250-254`) | Unpadded base64url of `{"blind_factor":"...","evaluated_output":"..."}` (`component.go:97-111`), system `coding.BSNTransportTokenNamingSystem` (`lib/coding/constants.go:12`) | Yes. The NVI's padding step adds up to four `=`; CPython accepts that for every length class (verified). |
| **Required OAuth scope** | None. The route only requires a verified bearer token whose claims include a `scope` member; no scope value is checked (`app/application.py:94-105`, `app/auth.py:45-77`, `app/services/client_oauth.py:105`). | `prs:oprf` (`app/routers/oprf.py:35-37`, `app/models/auth/data.py:6`). Scopes arrive in the `x-gf-scope` header from the OIN-verifier proxy (`app/models/auth/headers.py:20`, `docs/endpoints.md:176`); `prs:read` is explicitly tested as insufficient, 403 (`tests/test_authorization_scopes.py:55,68`). | `Authorization: Bearer <oauth-token-with-scope-prs:read>` (`pseudonymisation.md:81`) | Requests `prs:read` from the MinVWS token endpoint (`component.go:131`, `component/authn/oauth2.go`) | Inert on v0.0.18: any scope value passes. **403 on `main`.** The acceptance OAuth documentation (`integration-oauth.html:72-74`) calls `epd:read` the only valid scope at the moment; see the note below. |
| **Caller identity** | URA from the token's `sub` claim (`app/auth.py:75`), mTLS with a UZI certificate (`services.yaml:53`) | OIN from proxy-verified headers `x-gf-sub`, `x-gf-act-sub`, `x-gf-act-cn`, `x-gf-audience` (`app/models/auth/headers.py:16-19`); the bearer scheme is only a Swagger marker (`app/auth.py:18-24`) | Not specified | Client certificate plus the local organization's URA in the token request (`component/authn/oauth2.go:38-80`) | Yes. On `main` the caller is identified by OIN, which the Knooppunt does not have. |
| **Organization identifier format** | `ura:` + URA | `oin:` + 20-character OIN (`app/models/oin.py:8-10`) | `ura:` + URA (`pseudonymisation.md:86`, localization use case: `recipient_organization = "ura:<NVI URA>"`) | `ura:` + URA | Yes; no on `main`. |
| **HKDF parameters** | Not part of the service. The reference client shipped with the tag derives with HKDF-SHA256, 32 bytes, salt `None`, info `"{recv_org}|{recv_scope}|v1"` with `recv_org` like `ura:12345678`, and IKM `json.dumps({"landCode": ..., "type": ..., "value": ...})` in Python's default spacing (`oprf/client.py:42-45`). The service's own `/test/oprf/client` route blinds the string `NL:bsn:<number>` with no HKDF at all (`app/routers/test_oprf.py:57`, `app/personal_id.py:31`). | Documented in `README.md:279-288` and exercised in `tests/test_oprf_integration.py:113-123`: same parameters, info built from the `oin:...` recipient string, IKM compact JSON (`separators=(",", ":")`); the README notes that full RFC 8785 canonicalization is the intent. | HMAC-SHA-256, 32 bytes, no salt, info `"{recipient_organization}|{recipient_scope}|v1"`, IKM the JCS (RFC 8785) serialisation of `{"landCode","type","value"}` (`pseudonymisation.md:110-126`) | SHA-256, 32 bytes, nil salt (RFC 5869 zero salt), info `ura:<URA>|<scope>|v1`, IKM JCS (`blinding.go:15-29`, `canonical.go`). Pinned by `Test_deriveKey` against an OpenSSL-computed vector. | Parameters match the IG and the reference client. The IKM bytes differ from the v0.0.18 reference client's (it keeps `json.dumps` spaces), which the PRS never sees; it matters only for cross-client determinism, so every client of one NVI must derive alike. **On `main` the info string changes with the identifier format, so every pseudonym changes.** |
| **OPRF suite and de-blinding** | pyoprf/liboprf: OPRF(ristretto255, SHA-512), base mode; hash-to-group DST `HashToGroup-OPRFV1-\x00-ristretto255-SHA512` (`liboprf src/oprf.c:35,253`); Evaluate is `Z = k * B` (`src/oprf.c:344-348`); the recipient computes `N = r^-1 * Z` (`src/oprf.c:362-392`) and uses `N` itself as the pseudonym, without the RFC 9497 Finalize hash (`oprf_service.py:78-86`; crypto service `pseudonym_service.py:41-43`) | Same library; production evaluates in an HSM per OIN and key version (`docs/oprf-eval-flow.md:54-66`) | "OPRF ... with Ristretto255", references draft-irtf-cfrg-voprf | circl `oprf.SuiteRistretto255` (`blinding.go:42`), identifier `ristretto255-SHA512`, DST assembled the same way (circl `oprf/oprf.go:62-65,180-186`), hash-to-element per RFC 9380 appendix B (circl `group/ristretto255.go:93-107`) | Consistent by construction; both follow RFC 9497. Not executed cross-implementation in this repository (`test/prsmock` evaluates with circl's group arithmetic and a hand-written DST). |
| **Scalar and element encoding** | libsodium: 32-byte little-endian scalars, canonical 32-byte ristretto255 encodings | Same | Not specified | go-ristretto: little-endian scalar bytes (`scalar.go:34-75,1023-1027`), canonical element bytes (`ristretto.go:351-355`) | Yes |
| **Transport** | mTLS plus bearer token (`services.yaml:53`) | mTLS terminated by the OIN-verifier proxy | Bearer token | mTLS with the configured certificate, bearer from the MinVWS token endpoint | Yes |

### Note on the OAuth scope

The scope is a contract with the OAuth service, not with the PRS. Acceptance
runs OAuth Service v0.0.8 (`services.yaml:6`), and its integration page states
that `epd:read` is the only valid scope at the moment
(`integration-oauth.html:72-74`). The PRS at v0.0.18 does not look at the
value, so what matters is whether the token endpoint issues a token for
`prs:read`. That is not verifiable without the acceptance certificates;
`component/pseudonymisation/integration_test.go` exercises it when
`component/authn/cert.pem` and `cert-key.pem` are present. Changing the
requested scope is only meaningful once acceptance moves to a PRS that checks
it, and then the value is `prs:oprf`.

## Checklist for the day acceptance upgrades

Confirm the deployed version first: `GET /version.json` on the PRS, or
`data/services.yaml` in minvws/gfmodules-technische-documentatie. Then, for a
service at or after v0.0.22 (2026-06-25, the first tag without URA-based
recipients):

1. **Recipient identifier.** `recipientOrganization` must become `oin:<OIN>`.
   Obtain the OIN of the NVI and of every organization that registers with it;
   `nvi.audience` and the callers of `IdentifierToToken` carry URAs today.
2. **HKDF info string.** The info string contains the recipient identifier, so
   it changes with it. Every pseudonym the NVI holds for the Knooppunt's
   organizations changes; plan a re-registration. Update the pinned vector in
   `Test_deriveKey`.
3. **Scope.** Request `prs:oprf`. `prs:read` gets a 403.
4. **Caller identity.** `main` identifies the caller by OIN through
   proxy-verified headers; check that the MinVWS OAuth flow the `authn`
   component implements still yields an accepted token.
5. **Base64 padding.** Unpadded blinded input becomes acceptable; no change
   needed, the Knooppunt pads.
6. **Key rotation.** The JWE may carry `extra_versions`. That is the NVI's
   concern; the Knooppunt forwards the JWE unopened.
7. **Mock and tests.** Update `test/prsmock` to the new version and this
   document with it. The mock's package comment states the tag it models.

## How the tests pin this

| Test | Pins |
|---|---|
| `TestComponent_IdentifierToToken/posts the v0.0.18 evaluate request` | Method, path, `Content-Type`, the exact member names, the `ura:` prefix, the scope value, the blinded element's size, alphabet and padding, and the RFC 8785 member order of the body. |
| `TestComponent_IdentifierToToken/requests an access token ... with scope prs:read` | The scope, URA and audience handed to the `authn` component. |
| `TestComponent_IdentifierToToken/hands the PRS output to the NVI as base64url JSON` | The NVI identifier's system, its unpadded base64url encoding, the member names `blind_factor` and `evaluated_output`, that `evaluated_output` is the response's `jwe` verbatim, and the blind factor's size, alphabet, padding and canonical scalar form. |
| `TestComponent_IdentifierToToken/yields a token the recipient de-blinds to a stable pseudonym` | That a recipient using the service's own de-blinding turns two independently blinded tokens into the same pseudonym, equal to the unblinded OPRF of the HKDF-derived input. |
| `Test_deriveKey/matches an independent HKDF-SHA256 derivation` | Every HKDF parameter at once, against a vector computed with OpenSSL. |
| `test/prsmock` (`TestService_Eval`) | That the mock itself enforces the v0.0.18 request validation, status codes and error bodies, and produces the v0.0.18 JWE. |
