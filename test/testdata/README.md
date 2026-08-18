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
- **NVI localization**: one List per custodian (Plataan 00000010 and Zonnebloem
  00000020), so the patient is findable by BSN (pseudonymized) from either side.

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
delete-then-create per subject+custodian (the Knooppunt's `POST /nvi/List` is
otherwise an unconditional create, which would accumulate duplicates).

The compose `init` service runs `Load` + the Nuts subject bootstrap + `SeedNVI`.

### Reset variants (used by the GF Sandbox backend)

- `vectors.ResetGlobal(ctx, hapiBaseURL, knooppuntInternalBaseURL)` — expunges the
  mutable patient stores (removing user-created records, which have random ids a
  plain re-seed cannot overwrite), then re-runs `Load` + `SeedNVI`. This is the
  "restore fixtures" path.
- `vectors.RecyclePatient(ctx, hapiBaseURL, knooppuntInternalBaseURL, patientKey)`
  — restores one patient to its seeded, unshared state (re-registers its NVI Lists
  and re-PUTs its FHIR resources) without touching any other patient.

### Known limitations

- **Per-patient recycle cannot remove that patient's user-created marker records**
  (they have random ids). A global reset (expunge) is the escape hatch until a
  later epic tags user-created resources for targeted deletion.
- **Mitz subscriptions are not reset** by these paths (there is no standalone mitz
  service yet). A running demo's subscription survives a reset until that lands —
  this is expected, not a bug.
- **NVI delete-then-create is not atomic.** Acceptable for a single-writer seed
  (boot/reset only); concurrent seed + live registration could interleave.
- **did:web subjects / discovery** registration in the seed is best-effort: the
  local compose Nuts node has no discovery service defined, so it is expected to
  warn there. The URA credential is a separate epic (`TODO(E2)`).
- **`MEDAFSPRAAK` is a placeholder zorgcontext** for the seeded BGZ data; swap it
  when the CodeSystem gains a real BGZ code.
