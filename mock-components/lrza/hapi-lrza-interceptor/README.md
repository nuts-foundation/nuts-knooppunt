# Mock LRZA: HAPI FHIR interceptor

Locally and in the e2e tests, the LRZA (Landelijk Register Zorgaanbieders) is the `lrza-root-directory`
partition of our HAPI image. This interceptor makes that partition treat its writers like the real
LRZA (Care Services Directory IG 1.0.0) does, so the Knooppunt's registration proxy
(`component/lrza`) fails locally and in CI when it sends something the LRZA would refuse.

Reads are not restricted, and other partitions are left alone.

## Rules

| Rule | Answer |
|---|---|
| A vendor writes a care provider's Organization, Location, HealthcareService or Endpoint without its mandate: an active OrganizationAffiliation with `organization` = the care provider, `participatingOrganization.identifier` = the vendor and code `lrza-careprovider-admin` (`http://fhir.generiekefuncties.nl/CodeSystem/nl-gf-authorization-type-cs`) | 403 |
| The mandate is `lrza-endpoint-admin`, and the write is not an Endpoint or a change of `endpoint[]` only | 403 |
| An update moves a resource to a care provider that did not grant the mandate | 403 |
| A care provider writes another care provider's resources | 403 |
| Anyone but the care provider (identified by URA) writes its OrganizationAffiliation | 403 |
| Practitioner, PractitionerRole or a type the LRZA doesn't hold | 403 |
| DELETE, also as a transaction entry: withdraw by status instead | 405 |
| PUT without `If-Match` (transaction: `request.ifMatch`) | 428 |
| Stale `If-Match` | 412 (HAPI itself answers 409) |
| The resource does not conform to its nl-gf profile (Provenance: base R4) | 422 |
| A Provenance signature does not verify, or its signer is not the care provider of the target | 422 |

The care provider of a resource is found as the Knooppunt does (`component/lrza/ownership.go`): an
Organization with a URA, else the end of its `partOf` chain; `managingOrganization` for Location and
Endpoint, `providedBy` for HealthcareService.

Provenance signatures are verified as the IG's signing page describes: a detached JWS in
`signature.data` with the certificate chain in `x5c`, over the `json#static` canonical form of the
target version (meta and narratives removed, RFC 8785). Certificate trust is not checked.

## Who is calling

The real LRZA identifies the caller by its mTLS certificate. The mock doesn't do mTLS:

- `LRZA_CALLER` (e.g. `http://kvk.nl|90001234`) is the caller of every request: the vendor operating
  the Knooppunt, which sends nothing special, as with its real certificate.
- `X-Mock-Caller: <system>|<value>` overrides it per request, so a test can act as another vendor or as
  a care provider (`urn:oid:2.16.528.1.1007.3.3|<URA>`), e.g. to grant or revoke a mandate.
- `X-Mock-Operator: true` skips every rule: seeding (`test/testdata/vectors`) and tests that set up or
  inspect the directory directly.

The Knooppunt never sends these headers.

## Failures on demand

To see the Knooppunt cope with the LRZA's rate limiting and outages, the operator arms failures for the
next non-operator requests:

```http
POST /fhir/lrza-root-directory/$mock-fail?status=429&count=2&retryAfter=1&method=PUT
X-Mock-Operator: true
```

`method` is optional (default: every method), as is `retryAfter` (seconds). `GET` shows what is armed,
`DELETE` disarms.

## Configuration

| Variable | Default | |
|---|---|---|
| `LRZA_TENANT` | `lrza-root-directory` | The partition the rules apply to |
| `LRZA_CALLER` | none | The caller of a request without `X-Mock-Caller`; without either, writes are refused |
| `LRZA_PACKAGES_DIR` | `/app/lrza-packages` | The IG package and its dependencies (`*.tgz`), downloaded by the image build |
| `LRZA_PROFILE_VALIDATION` | `true` | `false` turns profile validation off |

## Build and test

The image (`mock-components/hapi/Dockerfile`) builds this module and the NVI interceptor
(`mock-components/nvi`) into HAPI:

```bash
docker build -t knooppunt-hapi:local -f mock-components/hapi/Dockerfile mock-components
KNPT_HAPI_IMAGE=knooppunt-hapi:local go test -p 1 ./test/e2e/...
```

The e2e harness otherwise runs the published `ghcr.io/nuts-foundation/fake-nvi:main`. It reuses the
HAPI container across runs: after rebuilding the image under the same tag, remove the old container
(`docker rm -f knooppunt-unittest-fhirstore-knooppunt-hapi-local`).

Unit tests:

```bash
mvn test
```

The profile validation tests need the packages, and are skipped without them. Take them from the
image:

```bash
docker cp "$(docker create knooppunt-hapi:local):/app/lrza-packages" /tmp/lrza-packages
LRZA_PACKAGES_DIR=/tmp/lrza-packages mvn test
```

Manual check, with `docker compose up`:

```bash
# No mandate (Care2Cure granted none): 403
curl -X POST localhost:7050/fhir/lrza-root-directory/Endpoint -H 'Content-Type: application/fhir+json' \
  -d '{"resourceType":"Endpoint","status":"active","managingOrganization":{"reference":"Organization/a3e4080d-8d53-4e53-bfbc-564e85158649"},"connectionType":{"system":"http://terminology.hl7.org/CodeSystem/endpoint-connection-type","code":"hl7-fhir-rest"},"payloadType":[{"coding":[{"system":"http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs","code":"CareServiceEntities"}]}],"address":"https://example.com/fhir"}'
# The same as the operator: 201
curl ... -H 'X-Mock-Operator: true'
```

## Not mocked

mTLS and certificate trust (PKIoverheid/UZI), terminology beyond the packages (codes from a code
system none of them defines only give a warning), the real LRZA's rate limits and exact error texts.
The proeftuin remains the final check.
