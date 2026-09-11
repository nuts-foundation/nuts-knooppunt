# PRS wire contract

`component/pseudonymisation` turns a BSN into the pseudonymous identifier the
NVI expects by calling the national pseudonymization service (PRS,
"Pseudoniemendienst"). Three descriptions of that service exist, and they
disagree. This document puts them side by side, states what the Knooppunt sends
today, and says whether that matches the version the acceptance environment is
documented to run. It is the checklist for the day acceptance is upgraded.

The tests that pin the Knooppunt side are in `component/pseudonymisation`
(`component_test.go`, `blinding_test.go`); they run against `mock-components/prs`,
which models the documented acceptance version. What the mock does and does
not model is stated in its package comment.

## The three dialects

| Dialect | Source | Status |
|---|---|---|
| **Acceptance, v0.0.18** | [minvws/gfmodules-pseudoniemendienst](https://github.com/minvws/gfmodules-pseudoniemendienst) at tag `v0.0.18` (commit `990cf81`, 2026-02-24). File references below are into that tag unless stated otherwise. | What the Proeftuin is documented to run. [`data/services.yaml`](https://github.com/minvws/gfmodules-technische-documentatie/blob/main/data/services.yaml) of minvws/gfmodules-technische-documentatie (commit `5a0027a`, 2026-09-03) lists PRS `v0.0.18` at `https://pseudoniemendienst.proeftuin.gf.irealisatie.nl` with `POST /oprf/eval` behind "mTLS + bearer token" (lines 41, 51, 53); `data/changelog.yaml` dates the initial Proeftuin release 2026-05-11 and records no PRS upgrade since. The running binary has not been observed from this repository: its `/version.json` refuses a connection without a client certificate. The Knooppunt client was written against this version (commit `3781ecb`, 2026-02-25). |
| **Service `main`, v0.0.34** | Same repository, `main` at `db1160b` (2026-09-08); latest tag `v0.0.34` (commit `ea91e01`, 2026-09-04). | Not documented as deployed to acceptance. Breaks the current client in two places (organization identifier, scope), which arrived in different tags; see the checklist. |
| **Draft IG** | [minvws/generiekefuncties-docs](https://github.com/minvws/generiekefuncties-docs), `input/pagecontent/pseudonymisation.md` (created in `29f8ed5`, 2026-04-21) and `input/pagecontent/localization.md`; published at [pseudonymisation.html](https://minvws.github.io/generiekefuncties-docs/en/pseudonymisation.html) (IG v0.10.0 ci-build, generated 2026-08-27). | Page status "Draft". Its production-evaluation dialect as a whole, `POST /evaluate` taking `blinded_input`, `recipient_organization` and `recipient_scope` and answering `evaluated_output`, has never been served by the service at any tag, nor by the ministry's PRS stub (minvws/gfmodules-pseudoniemendienst-stub). Some of the names do occur elsewhere in the service: `blinded_input` in the reference client and the test route (`oprf/client.py`, `app/routers/test_oprf.py:59`), `recipient_organization` and `recipient_scope` in the RID exchange route's payload (`app/routers/exchange.py:70-71`). `evaluated_output` and a `/evaluate` route appear nowhere in its history. Do not implement. |

The recipient side is governed by the NVI, not the PRS:
[minvws/gfmodules-nationale-verwijsindex](https://github.com/minvws/gfmodules-nationale-verwijsindex)
(`6930872`, 2026-09-08) and
[minvws/gfmodules-nationale-verwijsindex-crypto-service-api](https://github.com/minvws/gfmodules-nationale-verwijsindex-crypto-service-api)
(`e099e82`, 2026-09-08). The OPRF library behind the service is
[stef/liboprf](https://github.com/stef/liboprf) through its `pyoprf` binding;
v0.0.18 locks `pyoprf` 0.9.4 (`poetry.lock:1818-1824`) and compiles the C
library at v0.9.3 (`docker/Dockerfile:50`). File references to liboprf below
are into v0.9.4 (`ba45204`); `src/oprf.c` is identical at v0.9.3.

## Element by element

"Knooppunt sends" cites this repository. "Match" is against acceptance v0.0.18.

| Element | Acceptance v0.0.18 | Service `main` (v0.0.34) | Draft IG | Knooppunt sends | Match |
|---|---|---|---|---|---|
| **Endpoint** | `POST /oprf/eval` (`app/routers/oprf.py:14`) | `POST /oprf/eval` (`app/routers/oprf.py:28-29`) | `POST /evaluate` (`pseudonymisation.md:79`) | `POST {prsurl}/oprf/eval` (`component/pseudonymisation/component.go:154`) | Yes |
| **Request: blinded element** | `encryptedPersonalId`, string, at least 2 characters, must decode as base64url after padding (`app/services/oprf/oprf_service.py:13,17-26`). Evaluation decodes it again *without* the padding fix (`oprf_service.py:45`), so in practice the value must be padded. | `encryptedPersonalId`; the validator returns the padded value since v0.0.20 (`3c44866`; `app/models/requests.py:151-165` on `main`), so unpadded input is accepted. | `blinded_input`, "base64url-encoded" (`pseudonymisation.md:85`) | `encryptedPersonalId`: the 32-byte ristretto255 element as Go's `encoding/json` writes a `[]byte`, standard alphabet with padding (`component.go:125,142`) | Yes, by tolerance: `base64.urlsafe_b64decode` maps `-`/`_` to `+`/`/` and lets `+`/`/` through, so standard base64 decodes correctly (verified on CPython 3.9, 3.11 and 3.13; `mock-components/prs` models that decoder). Not byte-identical to what the reference client sends (`oprf/client.py:52`, urlsafe). |
| **Request: recipient organization** | `recipientOrganization`, string, must start with `ura:` or 400 `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`; the rest is looked up as the organization's URA, 404 if unknown (`oprf.py:22-30`). Organizations are created with an 8-digit URA (`app/models/requests.py:16`). | `recipientOrganization` is a `RecipientOrganizationOin`: `oin:` followed by a 20-character OIN, 8 digits, 8 or 9 alphanumerics and trailing zeros (`app/models/oin.py:58-63,142-179`). A value without the `oin:` prefix, or not a string, is a 422 with "Invalid recipient organization. Format: oin:<oin_number>" (`oin.py:149,164-172`); a prefixed but malformed OIN is a 422 with "Invalid OIN '...'. Expected 20 characters structured as 8 digit prefix + 8/9 alphanumeric mainnumber + 4/3 trailing zeros." (`oin.py:58-63,174-179`), the same at v0.0.24. `oin:` routing came with `dd17227` (2026-06-22, tag v0.0.22), the strict type with "Add type for RecipientOrganization" (`3687f45`, 2026-06-29, tag v0.0.24). | `recipient_organization`, example `"ura:90000901"` (`pseudonymisation.md:86`) | `"ura:" + recipientURA` (`component.go:140`), where `recipientURA` is `nvi.audience` from the configuration | Yes. **Breaks from v0.0.22**: nobody has OIN values for the NVI or the demo organizations yet. |
| **Request: recipient scope** | `recipientScope`, string, at least 2 characters; a public key must be registered for the organization under this scope, or under `*`, or 404 `{"error":"No public key found for this organization and/or scope"}` (`oprf_service.py:15`, `oprf.py:32-35`, `app/db/repositories/org_key_repository.py:18-30`) | Same (`requests.py:154`) | `recipient_scope` (`pseudonymisation.md:87`) | `nationale-verwijsindex` (`component/nvi/component.go:359`) | Yes, provided the NVI registered its key under that scope, or under `*`, on acceptance. |
| **Response** | `{"jwe": "<compact JWE>"}` (`oprf.py:44`) | Same key (`docs/endpoints.md:159-165`); the JWE payload gains an `extra_versions` claim during key rotation (`oprf_service.py:78-100`, `docs/endpoints.md:167`) | `{"evaluated_output": "<JWE compact serialization>"}` (`pseudonymisation.md:97-99`) | Reads `jwe` (`component.go:129`) and passes it on unopened | Yes |
| **JWE contents** (opaque to the client) | Protected header `alg` RSA-OAEP-256, `enc` A256GCM, `cty` application/json, `kid` = RFC 7638 thumbprint of the recipient key; claims `subject` = `pseudonym:eval:<padded base64url of the evaluated element>`, `aud` = the `recipientOrganization` string, `scope`, `version` "1.1", `iat`, `exp` = `iat` + 300 s (`app/services/oprf/jwe_token.py:13-29`, `oprf_service.py:51-57`) | Same, plus `extra_versions` and an optional configured `kid` (`docs/endpoints.md:27,167`) | "JWE compact serialization ... opaque to the client" | Forwarded verbatim as `evaluated_output` | Not applicable; the Knooppunt never opens it. |
| **Bytes: blind factor** (to the recipient) | Read by the recipient with `base64.urlsafe_b64decode` (service test receiver `oprf_service.py:83`; NVI crypto service `app/services/pseudonym_service.py:42`), 32-byte little-endian ristretto255 scalar (`liboprf python/pyoprf/__init__.py:145-156`). The acceptance documentation's own NVI example encodes it with standard `base64.b64encode` (`themes/gfmodules/layouts/page/integration-nvi.html:251`). | Same | "base64url-encoded blind_factor" (`localization.md:52`) | Standard base64 with padding, 32 bytes (`component.go:83,101,106`); scalar bytes are little-endian (go-ristretto `scalar.go:34-75`) | Yes, by the same decoder tolerance, and identical to the acceptance documentation's example. |
| **NVI identifier** (`List.subject.identifier`) | Governed by the NVI: system `http://minvws.github.io/generiekefuncties-docs/NamingSystem/nvi-identifier` (`app/models/fhir/resources/data.py:7`); value decoded with `base64.urlsafe_b64decode` after adding `=` padding (`app/utils/fhir.py:6-10`), members `evaluated_output` and `blind_factor` (`app/services/pseudonym_resolver.py:45-61`) | Same | `base64url(JSON{evaluated_output, blind_factor})` (`localization.md:47-56`); acceptance documentation shows unpadded base64url of compact JSON (`integration-nvi.html:250-254`) | Unpadded base64url of `{"blind_factor":"...","evaluated_output":"..."}` (`component.go:100-114`), system `coding.BSNTransportTokenNamingSystem` (`lib/coding/constants.go:12`) | Yes. The NVI's padding step adds up to four `=`; CPython accepts that for every length class (verified). |
| **Required OAuth scope** | None. The route only requires a bearer token that verifies and whose claims include a `scope` member; no scope value is checked (`app/application.py:94-105`, `app/auth.py:45-77`, `app/services/client_oauth.py:105`). | `prs:oprf`, since "Define scopes" (`d20cfc7`, 2026-09-02, tag v0.0.34; `app/routers/oprf.py:35-37`, `app/models/auth/data.py:6`). Scopes arrive in the `x-gf-scope` header from the OIN-verifier proxy (`app/models/auth/headers.py:20`, `docs/endpoints.md:176`); `prs:read` is explicitly tested as insufficient, 403 (`tests/test_authorization_scopes.py:55,68`). | `Authorization: Bearer <oauth-token-with-scope-prs:read>` (`pseudonymisation.md:81`) | Requests `prs:read` from the MinVWS token endpoint (`component.go:134`, `component/authn/oauth2.go`) | Inert on v0.0.18: any scope value passes. **403 from v0.0.34.** The acceptance OAuth documentation (`integration-oauth.html:72-74`) calls `epd:read` the only valid scope at the moment; see the note below. |
| **Caller identity** | URA from the token's `sub` claim (`app/auth.py:75`), mTLS with a UZI certificate (`services.yaml:53`); the token's `cnf.x5t#S256` must match the presented certificate (`client_oauth.py:116-140`) | OIN from proxy-verified headers `x-gf-sub`, `x-gf-act-sub`, `x-gf-act-cn`, `x-gf-audience` (`app/models/auth/headers.py:16-19`); the bearer scheme is only a Swagger marker (`app/auth.py:18-24`). The service stopped verifying the token itself in "Remove client oauth configuration" (`310e7f2`, 2026-06-29, tag v0.0.24, then headers `x-gf-oin` and `x-gf-audience`); the current header names date from "Use sub" (`3f1ef7f`, 2026-07-21, tag v0.0.29). | Not specified | Client certificate plus the local organization's URA in the token request (`component/authn/oauth2.go:38-80`) | Yes. From v0.0.24 the caller is identified by OIN, which the Knooppunt does not have. |
| **Organization identifier format** | `ura:` + URA | `oin:` + 20-character OIN (`app/models/oin.py:8-10`) | `ura:` + URA (`pseudonymisation.md:86`, localization use case: `recipient_organization = "ura:<NVI URA>"`) | `ura:` + URA | Yes; no from v0.0.22. |
| **HKDF parameters** | Not part of the service. The reference client shipped with the tag derives with HKDF-SHA256, 32 bytes, salt `None`, info `"{recv_org}|{recv_scope}|v1"` with `recv_org` like `ura:12345678`, and IKM `json.dumps({"landCode": ..., "type": ..., "value": ...})` in Python's default spacing (`oprf/client.py:42-45`). The service's own `/test/oprf/client` route blinds the string `NL:bsn:<number>` with no HKDF at all (`app/routers/test_oprf.py:57`, `app/personal_id.py:31`). | Documented in `README.md:279-288` and exercised in `tests/test_oprf_integration.py:113-123`: same parameters, info built from the `oin:...` recipient string, IKM compact JSON (`separators=(",", ":")`); the README notes that full RFC 8785 canonicalization is the intent. | HMAC-SHA-256, 32 bytes, no salt, info `"{recipient_organization}|{recipient_scope}|v1"`, IKM the JCS (RFC 8785) serialisation of `{"landCode","type","value"}` (`pseudonymisation.md:110-126`) | SHA-256, 32 bytes, nil salt (RFC 5869 zero salt), info `ura:<URA>|<scope>|v1`, IKM JCS (`blinding.go:17-31`, `canonical.go`). Pinned by `Test_deriveKey` against an OpenSSL-computed vector. | Parameters match the IG and the reference client. The IKM bytes differ from the v0.0.18 reference client's (it keeps `json.dumps` spaces), which the PRS never sees; it matters only for cross-client determinism, so every client of one NVI must derive alike. **From v0.0.22 the info string changes with the identifier format, so values derived after the switch no longer match the pseudonyms the NVI already holds.** |
| **OPRF suite and de-blinding** | pyoprf/liboprf: the hash-to-group, Blind, BlindEvaluate and Unblind steps of RFC 9497's OPRF(ristretto255, SHA-512) suite in base mode; hash-to-group DST `HashToGroup-OPRFV1-\x00-ristretto255-SHA512` (`liboprf src/oprf.c:35,253`); Evaluate is `Z = k * B` (`src/oprf.c`); the recipient computes `N = r^-1 * Z` and uses `N` itself as the pseudonym. RFC 9497's Finalize, which hashes the input together with the serialized `N`, is never applied (`oprf_service.py:78-86`; crypto service `pseudonym_service.py:41-43`); liboprf's binding calls that variant HashDH and says it is not the construction the RFC specifies (`python/pyoprf/__init__.py:122-127`). | Same library; production evaluates in an HSM per OIN and key version (`docs/oprf-eval-flow.md:54-66`) | "OPRF ... with Ristretto255", references draft-irtf-cfrg-voprf | circl `oprf.SuiteRistretto255` (`blinding.go:44`), identifier `ristretto255-SHA512`, DST assembled the same way (circl `oprf/oprf.go:62-65,180-186`), hash-to-element per RFC 9380 appendix B (circl `group/ristretto255.go:93-107`); the Knooppunt performs Blind only. | Yes. Both sides use RFC 9497's hash-to-group and evaluation mechanics for this suite; the application consumes the unblinded element rather than the Finalize output. The test suite does not execute liboprf; see the cross-implementation check below. |
| **Scalar and element encoding** | libsodium: 32-byte little-endian scalars, canonical 32-byte ristretto255 encodings | Same | Not specified | go-ristretto: little-endian scalar bytes (`scalar.go:34-75,1023-1027`), canonical element bytes (`ristretto.go:351-355`) | Yes, and covered by the cross-implementation check below. |
| **Transport** | mTLS plus bearer token (`services.yaml:53`) | mTLS terminated by the OIN-verifier proxy | Bearer token | The configured URL (`https` at acceptance, `http://mock-prs:8080` in the sandbox), the client the `authn` component returns: mTLS with the configured certificate, bearer from the MinVWS token endpoint (`component/authn/oauth2.go:42-80`) | Partly. `mock-components/prs` requires a bearer token and checks that the token came from its own ministry-style endpoint and was requested for it, so the component tests fail if the request does not go through the client the `authn` provider returned. It speaks plain HTTP and models no transport security: mTLS is terminated in front of the service, by the ingress where the mock is deployed and by nothing at all in compose and in the tests. Whether our client presents a certificate is exercised only by `component/pseudonymisation/integration_test.go`, against the real service. |

### Note on the OAuth scope

The scope is a contract with the OAuth service, not with the PRS. Acceptance
runs OAuth Service v0.0.8 (`services.yaml:6`), and its integration page states
that `epd:read` is the only valid scope at the moment
(`integration-oauth.html:72-74`). The PRS at v0.0.18 does not look at the
value, so what matters is whether the token endpoint issues a token for
`prs:read`. That is not verifiable without the acceptance certificates. The
component tests run the `authn` client against the mock's token endpoint,
which accepts what the client sends; only
`component/pseudonymisation/integration_test.go` asks the real one, when
`component/authn/cert.pem` and `cert-key.pem` are present. Two separate
questions follow. Whether the token endpoint issues a token for `prs:read`
today decides whether the integration works on acceptance at all, since the
token is requested before anything reaches the PRS; only the certificates can
answer it. The switch to `prs:oprf` is a different matter: it comes with a
PRS that checks the value, v0.0.34.

## Checklist for the day acceptance upgrades

Confirm the deployed version first: `GET /version.json` on the PRS (behind the
client certificate), or `data/services.yaml` in
minvws/gfmodules-technische-documentatie. The breaking changes arrived in
separate tags; apply what the target version includes and nothing more.
Switching the scope early is not harmless: the acceptance OAuth server
documents `epd:read` as its only valid scope today, and a v0.0.18 PRS ignores
the value, so an early `prs:oprf` gains nothing at the PRS and can only lose at
the token endpoint.

**At or after v0.0.20 (2026-05-28): base64 padding.** Unpadded blinded input
becomes acceptable (`3c44866`). No change needed; the Knooppunt pads.

**At or after v0.0.22 (2026-06-25): OIN recipients.**

1. **Recipient identifier.** `recipientOrganization` must become `oin:<OIN>`;
   `ura:` gets 400 `{"error":"Invalid recipient organization. Format:
   oin:<oin_number>"}` and an unknown OIN 404 `{"error":"No organization found
   for this OIN"}` (`app/routers/oprf.py:27-40` at v0.0.22). Obtain the OIN of
   the NVI and of every organization that registers with it; `nvi.audience` and
   the callers of `IdentifierToToken` carry URAs today.
2. **HKDF info string.** The info string contains the recipient identifier, so
   it changes with it. The rows the NVI already holds do not mutate, but every
   lookup value derived after the switch no longer matches them; plan a
   re-registration. Update the pinned vector in `Test_deriveKey`.

**At or after v0.0.24 (2026-06-30): strict OIN type, proxy-verified caller.**

3. **OIN validation.** `recipientOrganization` becomes a
   `RecipientOrganizationOin`, validated by pydantic: 422 "Invalid recipient
   organization. Format: oin:<oin_number>" without the `oin:` prefix, and 422
   "Invalid OIN ... Expected 20 characters structured as 8 digit prefix + 8/9
   alphanumeric mainnumber + 4/3 trailing zeros." for a prefixed value that
   is not an OIN (`app/models/requests.py:47`, `app/models/oin.py:58-63,142-179`
   at v0.0.24).
4. **Caller identity.** The service no longer verifies the bearer token
   itself; it reads the caller from headers the OIN-verifier proxy sets and
   answers 403 `{"detail":"Unauthorized request"}` without them
   (`app/auth.py:26-47` at v0.0.24; header names `x-gf-oin` and
   `x-gf-audience` there, `x-gf-sub`, `x-gf-act-sub`, `x-gf-act-cn` and
   `x-gf-audience` from v0.0.29). Check that the MinVWS OAuth flow the `authn`
   component implements still yields a token the proxy accepts.

**At or after v0.0.34 (2026-09-04): scope enforcement.**

5. **Scope.** The route requires `prs:oprf` in the proxy's `x-gf-scope` header
   (`app/routers/oprf.py:35-37`, `app/models/auth/data.py:6`); `prs:read` gets
   403 (`tests/test_authorization_scopes.py:55,68`). Request `prs:oprf`, once
   the token endpoint issues it.

**Any upgrade.**

6. **Key rotation.** The JWE may carry `extra_versions`. That is the NVI's
   concern; the Knooppunt forwards the JWE unopened.
7. **Mock and tests.** Update `mock-components/prs` to the new version and this
   document with it. The mock's package comment states the tag it models.
8. **Interpreter.** An image rebuilt on a newer CPython treats malformed
   base64 differently (3.14.6 rejects data after complete padding). The
   Knooppunt sends well-formed padded values, so only the mock's fidelity for
   garbage input is affected; re-run the decoder comparison against the
   image's interpreter.

## Cross-implementation check (review evidence, not enforced)

The component tests and `mock-components/prs` both use CIRCL for the group
arithmetic, so their round trip proves that the client and the mock agree with
each other, not that CIRCL agrees with the liboprf behind the service. A golden
value from liboprf is deliberately not in the test suite: nobody could
regenerate it without building a C library, so it would rot silently and be
"repaired" by pasting whatever the test then produced. The comparison was done
by hand instead and is recorded here so it can be redone when either library
is upgraded. Nothing below is enforced by a test.

On 2026-09-09, with the HKDF output that `Test_deriveKey` pins as input, CIRCL
and liboprf produced the same encoded group element and the same evaluation:

| Step | Value (hex) |
|---|---|
| Input `x`: HKDF output for `{"landCode":"NL","type":"BSN","value":"999940003"}`, `ura:90000901`, `nationale-verwijsindex` | `e96ce9a92e6b5fdf3ba6810b25c152c49c2969f6194e3e43fb050365a7104d9f` |
| Scalar `k`, little-endian, canonical (a test value) | `0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0f` |
| `HashToGroup(x)` under DST `HashToGroup-OPRFV1-\x00-ristretto255-SHA512` | `4cfe4459f0cc841792c6143df65078d7982912a6a6ddd3e0c5b68d2f8f3c5f31` |
| `k * HashToGroup(x)` | `164efdeacf87a2199aa217afdd2816f833b37bd6123b3c53001151c170c8753e` |
| `k^-1 * (k * HashToGroup(x))`, liboprf's `oprf_Unblind` with `k` as the blind | `4cfe4459f0cc841792c6143df65078d7982912a6a6ddd3e0c5b68d2f8f3c5f31` (the element above, on both sides) |

Versions: CIRCL v1.6.3 (`go.mod`); liboprf v0.9.4 (`ba45204`, the release of
the `pyoprf` binding v0.0.18 locks) and v0.9.3 (`a763867`, the C library the
v0.0.18 image compiles), whose `src/oprf.c` are identical; libsodium 1.0.20 for
the ristretto255 arithmetic on the liboprf side. This covers hash-to-group
(expand_message_xmd with SHA-512, the ristretto255 map and the DST), the
element encoding, scalar multiplication with the little-endian scalar
encoding, and scalar inversion through the unblind. Not compared: the random
blind itself, which each side draws for itself.

To redo it, on the CIRCL side:

```go
g := group.Ristretto255
p := g.HashToElement(x, []byte("HashToGroup-OPRFV1-\x00-ristretto255-SHA512"))
k := g.NewScalar()
_ = k.UnmarshalBinary(kBytes)
pBytes, _ := p.MarshalBinary()
z := g.NewElement().Mul(p, k)
zBytes, _ := z.MarshalBinary()
nBytes, _ := g.NewElement().Mul(z, g.NewScalar().Inv(k)).MarshalBinary() // == pBytes
```

and on the liboprf side, against a checkout of the tag to compare with:

```c
#include <sodium.h>
#include "oprf.h"
uint8_t p[crypto_core_ristretto255_BYTES], z[crypto_core_ristretto255_BYTES], n[crypto_core_ristretto255_BYTES];
voprf_hash_to_group(x, 32, p); /* src/oprf.c:252 */
oprf_Evaluate(k, p, z);
oprf_Unblind(k, z, n);         /* n == p */
```

```sh
cc -I liboprf/src main.c liboprf/src/oprf.c liboprf/src/utils.c $(pkg-config --cflags --libs libsodium)
```

## Where the mock's validation answers come from

`mock-components/prs` answers a malformed request the way v0.0.18 does because its
answers were captured from FastAPI 0.129.2, pydantic 2.12.5 and Starlette
0.52.1, the versions `poetry.lock` locks at v0.0.18 (lines 732-733, 1506-1507,
2219-2220), running the v0.0.18 `BlindRequest` class verbatim
(`app/services/oprf/oprf_service.py:12-26`) on a router with a dependency that
raises 401 like `get_auth_ctx` (`app/auth.py:59-61`), on CPython 3.11.6 and,
with identical results, on CPython 3.14.6. The service allows Python `^3.11`
(`pyproject.toml:14`) and builds its image from `python:3.14-slim`
(`docker/Dockerfile:3`), at a patch level nobody has observed; the answers
do not depend on it. The mock's model of Python's base64 decoder does: CPython
3.14.6 rejects data after a complete padding sequence, which 3.9 to 3.13
ignore, so for garbage input the mock follows the older interpreters.
Well-formed padded values decode the same everywhere. `TestService_Validation`
pins those answers with pydantic's `input` and `ctx` echoes and the JSON
decode position removed, which is where the mock knowingly differs. Redo it
in a virtual environment with those three packages and `httpx`:

```python
import base64
from typing import Optional
from fastapi import APIRouter, Depends, FastAPI, HTTPException
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from fastapi.testclient import TestClient
from pydantic import BaseModel, Field, field_validator
from starlette.responses import JSONResponse

class BlindRequest(BaseModel):  # oprf_service.py:12-26 at v0.0.18, verbatim
    encryptedPersonalId: str = Field(..., min_length=2)
    recipientOrganization: str = Field(..., min_length=2)
    recipientScope: str = Field(..., min_length=2)

    @field_validator("encryptedPersonalId")
    def validate_base64(cls, v: str) -> str:
        try:
            pad = "=" * ((4 - len(v) % 4) % 4)
            base64.urlsafe_b64decode(v + pad)
        except Exception as e:
            raise ValueError(f"must be base64url: {e}")
        return v

bearer = HTTPBearer(auto_error=False)

def get_auth_ctx(creds: Optional[HTTPAuthorizationCredentials] = Depends(bearer)):
    if creds is None or creds.scheme.lower() != "bearer":
        raise HTTPException(status_code=401, detail="Missing bearer token")

router = APIRouter()

@router.post("/oprf/eval")
def post_eval(req: BlindRequest) -> JSONResponse:
    if not req.recipientOrganization.startswith("ura:"):
        return JSONResponse({"error": "Invalid recipient organization. Format: ura:<ura_number>"}, status_code=400)
    return JSONResponse({"ok": True})

app = FastAPI()
app.include_router(router, dependencies=[Depends(get_auth_ctx)])
client = TestClient(app)
response = client.post("/oprf/eval", content=b"null", headers={"Content-Type": "application/json", "Authorization": "Bearer t"})
print(response.status_code, response.text)
```

## How the tests pin this

| Test | Pins |
|---|---|
| `TestComponent_IdentifierToToken/requests an access token ... with scope prs:read` | The scope, URA and audience handed to the `authn` component. Token issuance itself is stubbed. |
| `TestComponent_IdentifierToToken/obtains its token from the ministry-style endpoint and presents it` | That the `authn` client's token request is accepted by a token endpoint modelled on the ministry's documented one (client credentials with scope and target audience), and that the PRS route then sees a token that endpoint issued for it. The JWT-bearer assertion `authn` adds is not in the ministry's documentation and is ignored. |
| `TestComponent_IdentifierToToken/sends the request through the client the authn component returned` | That the request reaches the PRS with the bearer that client injects, so a component using any other client is caught. |
| `TestComponent_IdentifierToToken/posts the v0.0.18 evaluate request` | Method, path, `Content-Type`, the exact member names, the `ura:` prefix, the scope value, the blinded element's size, alphabet and padding, and the RFC 8785 member order of the body. |
| `TestComponent_IdentifierToToken/hands the PRS output to the NVI as base64url JSON` | The NVI identifier's system, its unpadded base64url encoding, the member names `blind_factor` and `evaluated_output`, that `evaluated_output` is the response's `jwe` verbatim, and the blind factor's size, alphabet, padding and canonical scalar form. |
| `TestComponent_IdentifierToToken/yields a token the recipient de-blinds to a stable pseudonym` | That a recipient using the service's own de-blinding turns two independently blinded tokens into the same pseudonym, equal to the unblinded OPRF of the HKDF-derived input. |
| `TestComponent_IdentifierToToken/yields different pseudonyms for different BSNs`, `.../binds the pseudonym to the recipient`, `.../keeps the configured base path`, `.../closes the PRS response body`, and the second organization and context marker in the access-token subtest | That the pseudonym depends on the BSN and on the recipient, that the calling organization, the caller's context and the configured base path reach the request rather than fixed values, and that response bodies are closed. |
| `TestComponent_IdentifierToToken/fails when the PRS ...` | That a 404 for an unknown recipient, a 500 and a 503 surface as errors carrying the status and body, and that a cancelled context stops the call before it reaches the PRS. |
| `Test_deriveKey/matches an independent HKDF-SHA256 derivation` | Every HKDF parameter at once, against a vector computed with OpenSSL. |
| `mock-components/prs` (`TestService_Eval`, `TestService_Validation`, `TestService_Transport`, `TestService_TokenEndpoint`, `TestService_Sandbox`, `Test_pythonURLSafeB64Decode`) | That the mock behaves as its package comment says: the v0.0.18 route checks with their status codes and bodies; the validation answers, against the FastAPI capture above; the bearer-token requirement; the token endpoint's checks and that only its own tokens, issued for this service, pass; the sandbox helper's round trips; and its model of Python's base64 decoder, against values from CPython. |
