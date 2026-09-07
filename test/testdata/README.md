# Test data

This package provides scripts for injecting test data, for:

- end-to-end tests
- local docker compose setups

It creates the following test data structure with multiple mCSD directories, plus
the GF Sandbox demo-patient pool and its NVI localization registrations.

## Care Organizations

### Ziekenhuis De Plataan
The consumer/requester hospital in the GF Sandbox BGZ demo. It localizes and
retrieves a patient's data from De Zonnebloem.
- URA: 00000010
- Vector package: `vectors/plataan`
- HAPI tenants: `plataan-admin` (9, admin directory), `plataan-patients` (10, its
  own hospital-side clinical data)

### Care Home Sunflower / Zorgcentrum De Zonnebloem
A fictional elderly-care organization; the source (data holder) of the BGZ in the
demo. (The `sunflower` vector and "De Zonnebloem" are the same organization.)
- URA: 00000020
- HAPI tenants: `sunflower-admin` (5), `sunflower-patients` (7)

### Care2Cure Hospital
A fictional hospital organization (second source, future scenarios).
- URA: 00000030

## mCSD Directory Structure

### Root Directory (LRZa)
The Dutch Landelijk Register Zorgaanbieders, a national registry of care providers.
It is the authentic source for organization names and URAs (primary identifier of
care organizations).

**Contains:**
- Organization registrations for all three care organizations (Plataan,
  Sunflower/Zonnebloem, Care2Cure)
- mCSD-directory endpoints pointing to each organization's admin directory

### Admin Directories
Each care organization maintains its own admin directory containing:
- Organization resource with detailed information
- FHIR endpoints for accessing the organization's services

### Query Directory
The aggregated directory that contains all resources from both root and admin
directories after the mCSD update process runs.

## GF Sandbox demo-patient pool

The `vectors/pool` package holds a small, fixed pool of ~6 synthetic patients
(Anna Jansen plus structural clones). Every pool patient is a complete two-source
fixture:

- **De Plataan side** (`plataan-patients` tenant): Patient + Condition (atrial
  fibrillation) + MedicationRequest (apixaban).
- **De Zonnebloem side** (`sunflower-patients` tenant): Patient (same BSN, own
  local id) + AllergyIntolerance (penicillin-class) + MedicationRequests
  (metoprolol, metformin) + Conditions (type 2 diabetes, hypertension).
- **NVI localization**: one List per data category De Zonnebloem holds
  (`Patient`, `AllergyIntolerance`, `MedicationRequest`, `Condition`), so the
  patient is findable by BSN (pseudonymized) at the source. De Plataan's side is
  deliberately not seeded: publishing it is what the GF Sandbox demonstrates
  when a practitioner shares a patient.

All FHIR resource IDs are derived deterministically from the patient key, so
seeding is an idempotent PUT-by-fixed-id upsert. Clones keep the same BGZ
structure with superficially varied names/values so repeated demos don't look
identical. UI counts must be derived from the seed, never hard-coded.

### Synthetic BSNs — pending RvIG verification

Pool BSNs are validated only by the Dutch elfproef (11-proof; see
`lib/bsnutil/ValidElfproef`) and avoid `999911120` (a Nictiz-fixture collision).
They are **pending RvIG verification**: before a hosted/public demo they must be
cross-checked against the published RvIG test-BSN set and replaced if absent.

## Seeding and reset

`vectors.Load(hapiBaseURL)` seeds all FHIR resources (mCSD directories, PIP, and
the pool on both sides). It is **idempotent and non-destructive**: every write is
a PUT-by-fixed-id upsert and there is no `$expunge` on the normal path.

`vectors.SeedNVI(ctx, knooppuntInternalBaseURL)` registers each pool patient's NVI
Lists through the Knooppunt's internal `/nvi` endpoint. It is idempotent via
delete-then-create per subject+custodian+**client** (the Knooppunt's
`POST /nvi/List` is otherwise an unconditional create, which would accumulate
duplicates). The client scope is the OAuth client id in `List.source`, and it is
applied as a `source:identifier` search parameter rather than as a filter on the
records that come back: the NVI rewrites `source.identifier` into a Device
reference on storage, so a client-side comparison would match nothing and delete
nothing while every count still looked right.

`vectors.Load` writes the demo organizations into the seeded **local** LRZa tenant
(`lrza-mcsd-admin`), which is what `KNPT_MCSD_ADMIN_LRZA_FHIRBASEURL` must point
at in compose. Pointing it at the external test LRZa instead fills the query
directory with unrelated organizations and none of the demo ones.

The compose `init` service runs `Load` + the Nuts subject bootstrap + `SeedNVI` +
an mCSD update. The update matters: the sync is request-driven (`POST
/mcsd/update`) with no background timer, so without it the seeded organizations
never reach the query directory and the patient is findable but not addressable.

### The compose credential pipeline

`did:web` DIDs embed a fresh UUID per subject creation, so an `X509Credential` —
which binds `credentialSubject.id` to a DID — cannot be generated ahead of time
and committed. Credentials therefore have to be issued *after* the subjects
exist, which is why the compose seed is three ordered services:

| Service | Does |
|---|---|
| `init` | seeds FHIR data, creates a Nuts subject per organization, writes `/shared/<org>.did` |
| `credential-issuer-{plataan,zonnebloem}` | runs `go-didx509-toolkit` against the committed test certificates, writes `/shared/<org>.jwt` |
| `init-credentials` | stores each credential in its subject's wallet, then registers the subject on the `bgz-test` discovery service |

Registration must come last: it builds a Verifiable Presentation from the
subject's wallet, which is empty until the credential is stored.

### Reset variants (used by the GF Sandbox backend)

- `vectors.ResetGlobal(ctx, hapiBaseURL, knooppuntInternalBaseURL, mitzMockBaseURL)` — clears the
  mutable patient stores (removing user-created records, which have random ids a
  plain re-seed cannot overwrite), then re-runs `Load` + `SeedNVI`. This is the
  "restore fixtures" path.

  Clearing is a search-and-delete per resource type with `_cascade=delete`, **not**
  `$expunge`. HAPI applies `expungeEverything=true` server-wide regardless of the
  tenant in the request path: it deletes every partition's data *and* the
  `PartitionEntity` rows, so one reset used to leave every tenant unresolvable
  (`Partition name "..." is not valid`) until the stack was rebuilt with
  `docker compose down -v`. The partition-scoped `?_expunge=true` form is correct
  but asynchronous (a Batch2 job on a 60s maintenance schedule), which is too slow
  for a reset behind a UI button. `TestResetGlobal_PreservesPartitions` guards this.
- `vectors.RecyclePatient(ctx, hapiBaseURL, knooppuntInternalBaseURL, patientKey)`
  — restores one patient to its seeded, unshared state (re-registers its NVI Lists
  and re-PUTs its FHIR resources) without touching any other patient.

### Known limitations

- **Per-patient recycle cannot remove that patient's user-created marker records**
  (they have random ids). A global reset is the escape hatch until a later epic
  tags user-created resources for targeted deletion.
- **NVI Lists registered under a BSN outside the pool survive a global reset.**
  The NVI tenant's pseudonymization interceptor rejects any `List` search not
  scoped to a patient/subject/source, so they cannot be enumerated to be deleted,
  and `SeedNVI`'s delete-then-create only covers the pool's own BSNs. DESIGN §5.6
  wants those gone; that needs a custodian-scoped listing on the Knooppunt (or the
  seed tracking what it registered).
- **Mitz subscriptions are not reset** by these paths. The compose `mitzmock`
  service is a stateless consent responder (closed questions only); a running
  demo's subscription survives a reset. This is expected, not a bug.
- **NVI delete-then-create is not atomic.** Acceptable for a single-writer seed
  (boot/reset only); concurrent seed + live registration could interleave.
- **The composed PDP is single-data-holder.** `KNPT_PDP_PIP_URL` points at
  `sunflower-patients`, because the PIP is a data-holder-side lookup and the
  composed Knooppunt acts as Zonnebloem's PDP. Once both demo organizations serve
  data, the PIP has to become per-tenant.
- **`data/nuts` is not a persistent volume.** Recreating the knooppunt container
  mints new DIDs; the seed re-runs credential issuance against whatever DIDs
  exist, so re-run the seed after such a restart.
- **There is no BGZ code, and there will not be one.** `List.code` is bound to
  `nl-gf-zorgcontext-vs`, whose 28 codes come from `nl-gf-data-categories-cs` and
  are data categories at FHIR resource granularity (`Condition`,
  `MedicationRequest`, `Patient`, ...). A patient summary is therefore the set of
  categories it contains, registered as one List each. An earlier IG used a
  single aggregate LOINC code and the published IG replaced it; `MEDAFSPRAAK`,
  which this seed used to register, is not a member of that value set at all.

## AC1 walkthrough: findable, addressable, retrievable

Issue [#542](https://github.com/nuts-foundation/nuts-knooppunt/issues/542) AC1:
*a fresh local or hosted deployment yields a findable, addressable and retrievable
patient without manual setup.*

From a clean checkout:

```bash
docker compose down -v      # only if you have an older stack lying around
docker compose up -d --wait
```

That is the whole setup. The PEPs, the mock Mitz and the credential pipeline are
all on the default path — no `--profile` needed.

The automated version of everything below is
`test/e2e/seed/ac1_retrieval_test.go`:

```bash
KNPT_E2E_COMPOSE=1 go test ./test/e2e/seed/ -run TestAC1 -v
```

> **⚠️ In-network vs host hostnames.** Inside the compose network the PEPs are
> `http://pep-zonnebloem:8080` and `http://pep-plataan:8080` — that is what the
> seeded mCSD Endpoints publish, and it is what other containers must use. From
> your shell on the host, the same PEPs are `http://localhost:9080` and
> `http://localhost:9081`. Using the in-network name from the host gives
> "could not resolve host"; this is the single likeliest source of confusion here.

### 1. Findable — locate the patient through the NVI

Anna Jansen's BSN is `999900006`. Ask the NVI which organizations hold data about
her, as Zonnebloem (URA `00000020`):

```bash
curl -s -H 'X-Tenant-ID: http://fhir.nl/fhir/NamingSystem/ura|00000020' \
  'http://localhost:8081/nvi/List?subject:identifier=http://fhir.nl/fhir/NamingSystem/bsn|999900006' \
  | jq '.total, .entry[].resource.id'
```

Expect a searchset Bundle with at least one `List`.

### 2. Addressable — resolve the custodian to an address via mCSD

```bash
QUERY_DIR='http://localhost:7050/fhir/knpt-mcsd-query'

ENDPOINT_REF=$(curl -s "$QUERY_DIR/Organization?identifier=http://fhir.nl/fhir/NamingSystem/ura|00000020" \
  | jq -r '.entry[0].resource.endpoint[0].reference')
echo "$ENDPOINT_REF"
```

Follow the returned `Endpoint/<id>` reference. Note the id is **not** the seeded
one: the query directory assigns its own ids when it ingests resources from the
admin directories, so always follow the reference rather than hardcoding an id.

```bash
curl -s "$QUERY_DIR/$ENDPOINT_REF" | jq -r '.address'
```

Expect `http://pep-zonnebloem:8080/fhir` — Zonnebloem's **PEP**, not
`:7050`. An address on `:7050` would point straight at HAPI and bypass the whole
authorization chain.

### 3. Retrievable — request a token and read through the PEP

Ask De Plataan's subject (`00000010`) for an access token against Zonnebloem's
authorization server. The `X509Credential` proving De Plataan's identity is
already in its wallet — the seed put it there — so only the two per-request
self-attested credentials are supplied here:

```bash
ACCESS_TOKEN=$(curl -s -X POST \
  -H 'Content-Type: application/json' \
  'http://localhost:8081/nuts/internal/auth/v2/00000010/request-service-access-token' \
  -d '{
    "authorization_server": "http://localhost:8080/nuts/oauth2/00000020",
    "scope": "medicatieoverdracht-gf",
    "token_type": "Bearer",
    "credentials": [
      {
        "@context": ["https://www.w3.org/2018/credentials/v1"],
        "type": ["VerifiableCredential", "HealthCareProfessionalDelegationCredential"],
        "credentialSubject": {
          "registeredBy": "http://fhir.nl/fhir/NamingSystem/uzi-nr-pers|000012345",
          "roleCode": "http://fhir.nl/fhir/NamingSystem/role-code-pers|01.015",
          "facilityType": "Z3"
        }
      },
      {
        "@context": ["https://www.w3.org/2018/credentials/v1"],
        "type": ["VerifiableCredential", "PatientEnrollmentCredential"],
        "credentialSubject": {
          "patientId": "http://fhir.nl/fhir/NamingSystem/bsn|999900006",
          "registeredBy": "urn:oid:2.16.528.1.1007.3.1.12345"
        }
      }
    ]
  }' | jq -r '.access_token')

echo "${ACCESS_TOKEN:0:40}..."
```

Now retrieve Anna's medication through Zonnebloem's PEP:

```bash
curl -s -H "Authorization: Bearer $ACCESS_TOKEN" \
  'http://localhost:9080/fhir/MedicationRequest?patient=Patient/pool-anna-zonnebloem-patient' \
  | jq '.resourceType, .total'
```

Expect `"Bundle"` and a non-zero total. The request travelled
PEP → token introspection → PDP → PIP (BSN lookup) → Mitz (consent) → HAPI.

### 4. Negative — prove the PEP is really in the path

```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  'http://localhost:9080/fhir/MedicationRequest?patient=Patient/pool-anna-zonnebloem-patient'
```

Expect `401`. If this returns `200`, the PEP is being bypassed and the success
above proved nothing.

### Troubleshooting

| Symptom | Likely cause | Where to look |
|---|---|---|
| Token request fails | The seed never stored De Plataan's credential, or never registered it on discovery | `docker compose logs init-credentials` and `init`; check `/shared/*.jwt` exists |
| `401` on an authorized read | Token was not accepted — expired or malformed credential | `docker compose logs pep-zonnebloem`; check the certs have not expired (`test/e2e/pep/certs/README.md`) |
| `403` on an authorized read | PDP denied. Most often the PIP cannot resolve the patient's BSN, so no consent is asked and the policy fails closed | `KNPT_PDP_PIP_URL` must point at the tenant holding the patient (`sunflower-patients`); check `mitzmock` is up |
| `502` from the PEP | Upstream tenant path wrong | `FHIR_UPSTREAM_PATH` on the PEP service |
| "could not resolve host" | Used an in-network hostname from the host shell | Use `localhost:9080`, not `pep-zonnebloem:8080` |
| Connection refused on `:9080` | PEP not up yet, or host port taken | `docker compose ps`; ports `9080`/`9081` must be free |
| Discovery registration fails | `bgz-test` not defined, or the CA hash changed | `config/discovery/bgz-test.json`; never regenerate the test CA (see the certs README) |
| Step 2 finds no Organization | The mCSD sync did not run, or ran against the external LRZa | `docker compose logs init` should show "Running mCSD update"; `KNPT_MCSD_ADMIN_LRZA_FHIRBASEURL` must be the local `lrza-mcsd-admin` tenant |
| Step 2 returns `null` for the address | Hardcoded Endpoint id | The query directory assigns its own ids — follow `Organization.endpoint[0].reference` instead |
