# Test certificates

Test UZI certificates used by the PEP e2e test and by the local `docker compose`
seed, which issues an `X509Credential` per demo organization at seed time.

These are **test fixtures only** — the CA is a fake root, and the private keys are
committed deliberately so a clean checkout can run the full authorization chain
without manual setup.

## Contents

| File | URA | Organization | Used by |
|---|---|---|---|
| `requester.{key,pem}`, `requester-chain.pem` | `87654321` | Test Hospital B.V. | `test/e2e/pep/authorization_test.go` |
| `plataan.{key,pem}`, `plataan-chain.pem` | `00000010` | Ziekenhuis De Plataan | compose seed (`credential-issuer-plataan`) |
| `zonnebloem.{key,pem}`, `zonnebloem-chain.pem` | `00000020` | Sunflower Care Home | compose seed (`credential-issuer-zonnebloem`) |
| `ca.{key,pem}`, `ca.srl` | — | Fake UZI Root CA | issuer of all of the above |

## ⚠️ Do not regenerate the CA

`config/discovery/bgz-test.json` pins the issuer by the CA's public key hash:

```
^did:x509:0:sha256:OPYJNiIh8WSwSDzJL0R2d2yPlolN9eAzZ8zvfMK3lcM::.*$
```

Running `generate-root-ca.sh` mints a new CA, which changes that hash and silently
breaks discovery registration for every certificate. Only re-issue leaf
certificates against the **existing** CA.

## ⚠️ Certificates expire after 365 days

`issue-cert.sh` hardcodes `-days 365`. When they expire, credential issuance in the
seed fails and AC1 breaks. `test/e2e/seed/ac1_retrieval_test.go` asserts `NotAfter`
is in the future so this fails loudly rather than as an opaque `401`.

## Regeneration

The organization names must match the seeded mCSD `Organization.name` **exactly**
(`test/testdata/vectors/{plataan,sunflower}/vectors.go`) — the `O=` field flows
through the discovery presentation definition into the token introspection claims,
and a mismatch surfaces only as a failed authorization.

```bash
cd test/e2e/pep/certs
./issue-cert.sh plataan    "Ziekenhuis De Plataan" Amsterdam 000012345 00000010 0
./issue-cert.sh zonnebloem "Sunflower Care Home"   Amsterdam 000067890 00000020 0
```

Then commit the six generated files plus the updated `ca.srl`.

Verify a re-issued certificate:

```bash
openssl x509 -in zonnebloem.pem -noout -subject -enddate
openssl x509 -in zonnebloem.pem -noout -text | grep -A2 "Subject Alternative Name"
openssl verify -CAfile ca.pem zonnebloem.pem
```

The SAN must contain an `otherName` of the form
`2.16.528.1.1007.99.2110-1-<uzi>-S-<ura>-00.000-<agb>`.
