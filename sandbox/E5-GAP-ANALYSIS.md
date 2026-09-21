# E5 gap analysis — canonical demo dataset, seed and reset

Scope: [issue #542](https://github.com/nuts-foundation/nuts-knooppunt/issues/542) (EPIC E5), measured against
branch `sandbox-e5-seed-data` (diff vs `main`) and [DESIGN.md](DESIGN.md) §5 / §9 E5.

**Summary:** the dataset, seed and reset *engine* is implemented and tested. The
identity bootstrap (X509/URA credentials, tenant registration), the hosted
deployment path (Helm seed job, PRS config, failure surfacing) and the §5.4 PEP
repointing are not. Two of the five acceptance criteria are unmet for hosted
deployments.

---

## 1. Work items

| # | Ticket work item | Status | Evidence / gap |
| --- | --- | --- | --- |
| 1 | Plataan organization, URA 00000010, tenant vector | ✅ Done | `vectors/plataan` (tenants `plataan-admin` 9, `plataan-patients` 10), `vectors/lrza/plataan.go`, mCSD Organization + Endpoint |
| 2 | did:web identifiers | 🟡 Partial | `bootstrapNutsSubjects` in `test/testdata/main.go` creates a subject per org, but it is **best-effort** (failures only warn) |
| 3 | Wallet-held X509 credentials with URA in cert SAN | ❌ Missing | No X509/SAN code anywhere in the diff. `NUTS_ISSUER_DID: TODO` still stands in `docker-compose.yml`; code defers with `TODO(E2)` |
| 4 | mCSD Organizations + Endpoints | ✅ Done | Seeded in LRZa root and each org's admin directory |
| 5 | Tenant registrations (pseudonymization) | ❌ Missing | Nothing registers URA 00000010 as a pseudonymization tenant |
| 6 | NVI Lists | ✅ Done | One List per (patient, custodian) via `SeedNVI`; idempotent by delete-then-create |
| 7 | Validate all BSNs against RvIG test sets | 🟡 Partial | Only the elfproef is implemented (`lib/bsnutil.ValidElfproef`) and `999911120` is avoided. **Blocked on the external RvIG dependency**; code is explicit that BSNs are "pending RvIG verification" |
| 8 | Seed runs through the compose init service | ✅ Done | `init` service runs `Load` + subject bootstrap + `SeedNVI` (ordering bug fixed, see §4) |
| 9 | Seed runs as a hosted deployment job | ❌ Missing | No seed Job/Hook in `helm/`. §5 of DESIGN explicitly requires "a seed job/hook in the Helm deployment when hosted" |
| 10 | Resettable, respecting active demo locks | ✅ Done | `ResetGlobal` / `RecyclePatient`; advisory lock registry with TTL + ownership; recycle 409s on a locked patient, global reset warns and lists locks with an explicit override |
| 11 | Reset removes user-created FHIR resources | ✅ Done | Expunges both patient tenants (random-id records cannot be overwritten by a re-seed) |
| 12 | Reset removes NVI Lists | ✅ Done | NVI tenant now expunged in `ResetGlobal` (was a gap; fixed — see §4) |
| 13 | Reset removes subscriptions | ❌ Missing | Mitz subscriptions are not reset — documented as a known limitation (no standalone mitz service yet) |
| 14 | Reset removes step events | ❌ Missing | Step-event retention is E6; nothing to reset yet, but §5.6 lists it as reset scope |
| 15 | Hosted pseudonymization against PRS acceptance (`prsurl`) | ❌ Missing | Not configured anywhere |

## 2. Acceptance criteria

| # | Criterion | Verdict |
| --- | --- | --- |
| 1 | Fresh deployments immediately functional, no manual config | 🟡 **Local only.** Compose works (after the `depends_on` fix). Hosted has no seed job, so a fresh hosted deploy is *not* functional without manual steps |
| 2 | Re-running the seed is idempotent | ✅ **Met.** Every write is a PUT-by-fixed-id upsert; NVI uses delete-then-create. Asserted by `TestSeed_NVIIsIdempotent` (double-seed → exactly 1 List per patient per custodian) |
| 3 | Reset restores published fixtures without redeployment | ✅ **Met.** `POST /demo/reset` and `POST /demo/patients/{key}/recycle` |
| 4 | Per-patient reset does not interrupt other demos | ✅ **Met.** Asserted by `TestRecyclePatient_RestoresTargetLeavesOthersIntact`; lock registry enforces it at the handler |
| 5 | Hosted environments show visible failure when PRS is unavailable | ❌ **Not met.** No PRS integration and no failure surfacing |

## 3. Additional DESIGN §5 gaps not itemised in the ticket

- **§5.4 PEP repointing — explicitly called E5 scope.** The Zonnebloem endpoint
  still points straight at HAPI (`http://localhost:7050/fhir/sunflower-patients`),
  bypassing PEP/PDP/Mitz entirely, and the compose PEP is still configured for
  URA `00000666`. Plataan's new endpoint repeats the pattern with a `TODO(E4)`.
  Addresses are hardcoded `localhost`, not deployment-aware as §5.4 requires.
- **§5.1 Location / HealthcareService** entries (backing the "sublocation" line,
  e.g. ward De Vlinder) are absent.
- **§5.7 Mitz consent set to yes** per pool patient is not seeded.
- **Lock trigger** is manual-only (`/lock`, `/release`); the scenario-run trigger
  is E4/E6, which the README states deliberately.

## 4. Fixes applied in this pass

| Finding | Severity | Resolution |
| --- | --- | --- |
| `init` seeded NVI through the Knooppunt but only waited for HAPI — first-boot seeding could hard-fail (`main.go` panics) | **Bug** | Added a `knooppunt-healthcheck` service polling internal `/status` (200 only once all components are ready) and made `init` depend on it |
| `ResetGlobal` did not expunge the NVI tenant, so a List registered under a non-pool BSN survived a global reset, contradicting §5.6 | **Bug** | Added `nvi.HAPITenant()` to the expunge set; documented why |
| `formOrQuery` discarded the `ParseForm` error — a malformed body silently read as `override=""` | Correctness | Returns an error; handlers reply `400`. Regression test `TestReset_MalformedFormIsRejected` asserts a reset is *not* triggered |
| `handleRecycle` checked patient existence before `configured()`, inconsistent with `handleReset` | Consistency | Reordered so both report a disabled backend identically |
| `pool.Patients()` rebuilt the slice per call; `PatientByKey` was O(n) over a fresh allocation | Efficiency | `sync.OnceValue` for the pool plus a key index |
| `ExpungeAll` is system-wide and destructive but read as routine | Clarity | Doc comment now leads with DANGER and points to `ResetGlobal` |

Verified: `go vet` clean on both modules, `docker compose config` valid,
`go test ./sandbox/... ./lib/...` all pass.

Not changed: `derefName` was flagged as a possible duplicate — no existing
`Deref` helper exists in the repo or the `caramel/to` package, so it stays.

## 5. Recommendation

The dataset/seed/reset work is coherent and merge-worthy on its own. The
remaining items split cleanly into three follow-ups:

1. **Identity bootstrap** (items 3, 5) — X509/URA credentials + pseudonymization
   tenant registration. Overlaps E2; confirm ownership before scheduling.
2. **Hosted deployment** (items 9, 15; AC 1, 5) — Helm seed job, `prsurl` config,
   PRS failure surfacing. Blocked on the external PRS credentials dependency.
3. **PEP repointing / deployment-aware addressing** (§5.4) — currently drifting
   toward E4 via `TODO(E4)` despite DESIGN naming it E5 scope. Worth an explicit
   decision rather than letting it slide.

Item 7 (RvIG) stays open until the verified test-BSN dataset arrives; the code is
structured so only the BSN constants change.
