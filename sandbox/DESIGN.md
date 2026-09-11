# GF Sandbox: application design

Design for the GF Sandbox, the application that demonstrates the Generieke Functies (GF) for Dutch healthcare data
exchange on top of the Nuts Knooppunt. This document consolidates the earlier functional layout and epics and the design
decisions taken since. Scope of the first release is the **/demo** path; /connect is designed ahead (section 8) so
nothing built now blocks it.

The /demo user story and UX follow [issue #532](https://github.com/nuts-foundation/nuts-knooppunt/issues/532): a medical
specialist at a hospital localizes and retrieves a patient's data from an elderly care institution via the NVI (indexed
pull). The flow deviates deliberately from the issue's screen list where later rounds improved it (a patient list
instead of BSN entry at registration, a separate authorization result page, the marker path); the wireframe is the
reference for those decisions.

The sandbox lives in this folder as a subproject of the monorepo.

## 1. Purpose and audiences

One application, two paths over one shared backbone:

- **/demo: the indexed-pull story of a BGZ exchange, for functional managers, product owners and prospect demos. No
  protocol detail in the main flow; the GF viewer is the deliberate exception: it makes visible what goes over the
  wire (Functional narration by default, Technical requests and spans opt-in), scoped to the scripted chain on the demo
  pool (section 5.7). It proves that the GF chain delivers functional value: the right data reaches the right provider,
  lawfully.**
- **/connect** (later): the sandbox plays a counterpart role so external parties test their own software against it,
  with the inspection tooling aimed at their own runs (request/response bodies, PDP decision reasons, correlated traces)
  for real troubleshooting and gap-filling.

There is no separate /debug path anymore: its goal, making transparent what happens per step, ships with /demo as the GF
viewer, and its deep-inspection remainder becomes the /connect inspector (section 8). The living documentation for
implementers is the viewer itself: anyone can read along with every step without connecting anything.

v1 definition of done for /demo, aligned with the acceptance criteria of #532:

- Dezi session info is visible in the EMR top bar at all times.
- Patient registration publishes an NVI localization record per data category and starts a Mitz subscription, both
  confirmed on screen with what the calls established and nothing more.
- The localization results clearly distinguish NVI output from GF Adressering output.
- Retrieval requires explicit user confirmation before data is pulled.
- The authorization result page shows all sub-checks with the demo disclaimer.
- Every data element on the enriched home screen is visually attributable to its source.
- Consent revocation triggers a notification, removes the source's data, and blocks re-retrieval.
- Synthetic data only; the demo runs from a hosted environment whose fixed dataset is ensured at deployment (idempotent
  seed job) and restorable at any moment via the reset action, without redeploying. `docker compose` remains the offline
  development path (the demo-ehr and PEP services currently sit behind compose profiles; E5 makes the sandbox set
  default or names the exact profile list).

## 2. Standing decisions

These were settled during the planning, mockup and user-story rounds and are inputs to this design, not open questions:

1. **No orchestrator tier.** The client-visible chain is four calls (NVI query, mCSD lookup, access token, data
   retrieval); whoever makes the calls knows when each step starts and ends, and the interior steps (Mitz check,
   pseudonymization, PIP, introspection) are invisible to any external caller regardless. What remains is a **capture
   middleware** on the sandbox backend that emits step events over SSE, following the step-event schema (GF label,
   sanitized allowlisted request/response fields, outcome, timing, correlation ID). Externally initiated calls
   (/connect) never pass an orchestrator anyway; they are covered by OTel and PEP logging.
2. **Explicit result screens, not notifications. Decision by the author of #532: the flow uses full result screens and
   modals (registration confirmation, localization results, authorization outcome) instead of snack bars or stacked
   notifications, so it is visually clear what happens at every step.** The earlier "look under the hood" toggle has
   since grown into the GF viewer: a collapsible side panel that follows the demo step by step (section 4,
   cross-cutting), the transparency feature of /demo and the component the later /connect inspector reuses.
3. **The source is a genuinely separate application in a separate browser tab.** Team decision: entering data inside the
   demo surface would suggest everything lives in one system and the GF adds nothing. The source institution's record
   system is its own app with its own branding and URL; the only road between the two systems is the GF chain. In the
   #532 flow this carries the optional marker-proof path (section 4, step 0b).
4. **Single multi-tenant Knooppunt instance** hosting both the consumer and the source organization (NVI
   pseudonymization is keyed per tenant via `X-Tenant-ID`).
5. **Knooppunt has no CORS and the internal mux (:8081) is never exposed on shared deployments**; every browser call
   goes through an application backend, and the sandbox backend is the only component allowed to talk to :8081. Note:
   the checked-in compose still publishes `8081:8081` for local development; removing or gating that mapping on hosted
   deployments is E5 scope.

## 3. System landscape

Two user-facing applications on top of the existing mocked stack:

| System                   | What it is                                                                                                                                                                                                                                                                                                                                      | Style                                               | Default port                                                                                                                                        |
|--------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|
| **GF Sandbox**           | Shell (path chooser, demo controls, reset) whose /demo path renders the consumer EMR experience: **Plataan EHR**, the hospital system of Ziekenhuis De Plataan. Go backend (sandbox/app) proxies to the Knooppunt and emits step events                                                                                                         | Shell: quiet frame (6.1). EMR: warm editorial (6.2) | own port, e.g. :8091 (the current demo-ehr maps host :8091 to container :3000 and is to be split; host :3000 itself is taken by the mock VC issuer) |
| **ZorgDossier**          | The elderly care institution's own record system: resident record, data entry, NVI registration on save. Stack decided in E7 (demo-ehr reuse or the sandbox's Go shape), restyled and reduced                                                                                                                                                   | Own brand, deliberately different (6.3)             | own port, e.g. :3001                                                                                                                                |
| Nuts Knooppunt           | Real GF components: NVI, mCSD, PDP, Mitz client, Nuts node proxy                                                                                                                                                                                                                                                                                | n/a                                                 | :8080 public, :8081 internal                                                                                                                        |
| PRS (pseudonymization)   | **mocked offline, real when hosted**: since PR #561 the compose overlay points `prsurl` at `mock-components/prs`, a mock modelled on acceptance PRS v0.0.18 that performs real OPRF on ristretto255 and stands in for the ministry's token endpoint, so the Knooppunt's own authn flow runs unchanged. The hosted sandbox points the same configuration at the PRS acceptance environment. The built-in fake pseudonymizer is now only reached when `prsurl` is unset, which the e2e harness still does. Scope: acceptance v0.0.18 checks no scope value, the service's `main` requires `prs:oprf`, and the acceptance OAuth service documents `epd:read`; see `docs/prs-contract.md` | n/a                                                 | external (acc)                                                                                                                                      |
| Mocked national services | fake-NVI backing store (HAPI), fake LRZa, mock Mitz (user-controllable answer, subscriptions and notifications), mock Dezi (signed v0.7 attestation over an authorization code flow), mock VC issuer (`HealthcareProviderRoleTypeCredential` over OID4VCI; still in compose on :3000, but not on the sandbox authentication path, see E2)                                                                                                                                                                                   | n/a                                                 | existing compose ports                                                                                                                              |

Mitz is mocked deliberately for `/demo`; none of the sandbox organizations or URAs need registration in the real Mitz
for v1. Consent is the national-state answer the presenter changes live and reset restores per pool patient. Using
test-Mitz would add onboarding, availability and shared-state dependencies, and changing the answer from the demo would
require a consent-registration interface that the Knooppunt does not implement. The Knooppunt-side path remains real:
`component/mitz` sends XACML closed authorization questions and creates FHIR subscriptions against the mock. Real
test-Mitz connectivity remains a separate component-testing track, while national onboarding belongs to the post-v1
`/connect` mode 2. The asymmetry with PRS is intentional: pseudonymization is stateless from the demo's perspective, so
its real acceptance environment adds realism without sacrificing deterministic reset.

Interaction rules:

- The browser never calls the Knooppunt; each app's backend proxies its own calls.
- ZorgDossier and the sandbox share **no state, no session, no API**. ZorgDossier writes to its own FHIR store and
  registers in the NVI; the sandbox finds that data only through the GF chain (NVI, mCSD, token, retrieval).
- The sandbox backend records every proxied call as a step event (correlation ID per scenario run, bounded in-memory
  retention, SSE to the browser). Capture is allowlist-based and removes authorization headers, cookies, access tokens,
  credentials and other secrets before an event exists. BSN visibility in events is deployment-conditional: plaintext
  test BSNs locally, masked on shared deployments.
- Opening ZorgDossier from the sandbox is a plain `target="_blank"` link: genuinely a second browser tab, matching
  decision 3.

Application tech stack: the sandbox UI is a server-rendered hypermedia application on a small standalone Go backend in
`sandbox/app` (stdlib `net/http` + `html/template` + `go:embed`, following the `mcsdadmin` pattern in this repo), with
**Datastar as the v1 frontend implementation**. demo-ehr's pages remain reference material; its proxy role is superseded
by Go equivalents (the knooppunt client plumbing in `lib/` is reused directly from E2/E4 on). Whether ZorgDossier (E7)
reuses demo-ehr's Node plumbing or follows the same Go shape is decided in E7. Pages are server templates ported from
the wireframe, live updates arrive over the E6 SSE stream, the journey strip stays vanilla JS, and Datastar is vendored
as one static file: no npm dependency tree and no build step.

The choice is deliberately reversible, with exactly one frontend implementation active: Datastar initially, or Preact +
htm after a cutover. Backend routes, the step-event JSON schema, design tokens and CSS, and the framework-free
journey-map driver stay independent of Datastar. Review the choice after the first complete vertical slice and again
when the consent-revocation flow lands. If Datastar requires growing bespoke client JavaScript or duplicated client
state, makes SSE reconnect/resume behavior fragile, or materially slows implementation and testing of the stateful
viewer and forms, stop adding workarounds and use AI-assisted refactoring immediately to replace it with the existing
buildless **Preact + htm** shape in one migration.

## 4. The /demo experience

The flow of #532, framed by a slim sandbox demo bar (scenario label, reset). Consent withdrawal is a control on the
enriched record's optional-ending card, and the GF viewer opens from a collapsible tab on the right edge (see
cross-cutting below). Screens (the wireframe numbers these 1-8, splitting record overview and sharing into two screens):

0. **Start** (`/demo`): the shell's path chooser and a brief scenario intro (E1; not part of the clickable wireframe,
   which begins at the login screen). **Two optional side doors:**

- **0b. Marker proof** (optional, before or during the demo): open ZorgDossier in a new tab, enter a recognisable record
  for the patient (for example an allergy with a self-chosen marker word in the free-text note) and register it in the
  NVI via an explicit checkbox (the source's patiëntaanmelding). Later retrieval highlights the marker: proof the data
  really comes from the other system.

1. **Login (GF Authenticatie): the specialist logs into the Plataan EHR via a mock Dezi login.** From here the EMR top
   bar permanently shows the Dezi session: practitioner name, role, UZI number, and organization context ("Logged in to:
   Ziekenhuis De Plataan"). Name, role and organization are rendered as the attestation spells them, so they appear in
   Dutch and carry no courtesy title; only the application's own chrome is English.
2. **Patient list: the EMR opens on the practitioner's patient list. Every record already holds locally recorded data; a
   status chip per patient distinguishes "shared via GF" (locatable through the NVI) from "local only".** All pool
   patients (section 5.7) start as local only; the presenter picks a fresh one (Anna in the canonical walkthrough), and
   patients locked by a running demo are marked "demo in progress".
3. **Record overview and sharing (patiëntaanmelding)**: opening Anna's record shows her existing local data plus a "not
   findable yet" callout. Sharing the patient publishes one NVI localization record per data category the hospital
   holds (there is no aggregate code; see section 10) and starts the Mitz subscription, both confirmed on screen with
   their registered attributes; no clinical data leaves the source, only a pointer. The record is then marked shared, an "available sources" card shows where data comes from (initially
   only De Plataan itself), and a prominent "retrieve data" button opens the localize flow.
4. **Retrieve data** (modal): the localize flow.
1. Loading state: "localizing where patient data can be found".
2. Results list with a clear visual split between the two GFs: **NVI** rows ("data type X found at URA of Zonnebloem",
   with all context the NVI returns) and **GF Adressering** rows ("org with URA X stores this data in sublocation Z on
   system Y", the resolved endpoint).
3. The practitioner explicitly confirms retrieval from that source before anything is pulled.
5. **Authorization result page**: the GF Autorisatie outcome, top-level result plus all sub-checks performed under the
   use-case policy: authentication check passed (Dezi VC, URA VC, RoletypeVC), practitioner role check passed (Dezi
   info), consent check passed (verified via Mitz), data request scope check passed (within what the use case permits).
   **Demo disclaimer at the top**: in production these checks run on the source side and only a yes/no comes back; the
   breakdown is shown for demo purposes only.
6. **Enriched home screen**: the overview now shows data from two sources, De Plataan and Zonnebloem, with source
   attribution visually explicit on every element. On the marker path the user's own ZorgDossier entry appears with the
   marker word highlighted and a "just entered by you in ZorgDossier" chip.
7. **Consent revocation (GF Toestemming, optional ending)**: the patient revokes consent in Mitz. In the demo, the
   "withdraw consent" control on the enriched record's optional-ending card flips the mock Mitz answer and presents that
   as the patient having acted in the Mitz portal. The subscription notification is a UX event only: demo plumbing shows
   it to the practitioner and removes previously retrieved Zonnebloem data from the overview; it does not grant or deny
   access. On the next retrieval attempt the PEP asks the PDP for a new authorization decision, the PDP performs a fresh
   Mitz closed question, and the BGZ policy denies the request fail-closed when consent is absent. Access enforcement
   therefore remains correct even if notification delivery is delayed or fails.

Cross-cutting:

- **Reset** restores the fixed dataset exactly (section 5.6) and removes user-created data.
- **The GF viewer ("under the hood")**: a right-hand side panel (~560px) that opens by default once the user is past
  login and collapses to a tab on the screen edge. It follows the demo live: a compact journey strip (national registers
  above, the two organizations below) animates every step, with the step details underneath. Two modes: **Functional**
  narrates the unlocking metaphor in plain language (a pointer lands in the referral index, a pin drops on the found
  source, proof travels, a key comes back, three locks open, the question document travels with the key, the vault
  swings open and the answer returns filled in; on revocation a refusal travels back instead of a key and the earlier
  copy is removed): no protocol detail. **Technical** adds the captured request per step (method, path, status) and the
  interior OTel spans (PRS, PEP introspection, PDP). An "open the full journey" page shows the same run as a large
  animated journey map (Functional) or as the trace layout (Technical). Functional is the default.
- Demo language is English; Dutch scheme and system names (Mitz, NVI, Dezi, UZI, BSN, URA, BGZ) stay as proper nouns.
  All identifiers and patients are synthetic.

## 5. Canonical dataset

One dataset, consistent across every system it touches. Extends the existing test/testdata vectors; applied by an
idempotent loader that runs at deployment (the init compose service locally, a seed job/hook in the Helm deployment when
hosted) and again on every reset.

### 5.1 Organizations

| Organization              | Role                                                   | URA      | Tenant / vector             | Notes                                                                           |
|---------------------------|--------------------------------------------------------|----------|-----------------------------|---------------------------------------------------------------------------------|
| Ziekenhuis De Plataan     | consumer (requester)                                   | 00000010 | new vector `plataan`        | new URA, currently unused in testdata                                           |
| Zorgcentrum De Zonnebloem | source (data holder, elderly care institution)         | 00000020 | existing `sunflower` vector | the vector already models a care home; sunflower ↔ zonnebloem, deliberate match |
| Care2Cure Hospital        | second source (future scenarios, /connect counterpart) | 00000030 | existing `care2cure`        | unchanged                                                                       |

Each organization gets, via the idempotent bootstrap: a did:web plus a wallet-held `X509Credential` carrying the URA in
its certificate SAN (issued from the demo UZI-style certificate with the didx509 toolkit and stored through the holder
endpoint, as `test/e2e/pep/authorization_test.go` does; not from the mock VC issuer, which is not on this path), an mCSD
Organization plus Endpoint entry in the LRZa admin directory (plus the
Location/HealthcareService entries that back the "sublocation" line in the retrieve modal, e.g. ward De Vlinder at De
Zonnebloem: a FHIR R4 Endpoint alone cannot express a ward), and a tenant registration for pseudonymization.

### 5.2 Personas and patient

| Item            | Value                                                                                                                                                                                                                      | System of record                                                                             |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| Requesting user | S. el Amrani, klinisch geriater (RoleCodeNL 01.022), Ziekenhuis De Plataan, UZI 900001234 (synthetic)                                                                                                                           | mock Dezi session (claims: name, role, UZI, URA; used in PDP input and shown in the top bar) |
| Source user     | B. Willems, verpleegkundig specialist, De Zonnebloem                                                                                                                                                                       | ZorgDossier display only                                                                     |
| Patient         | Anna Jansen, born 1944, resident of De Zonnebloem, known at De Plataan; canonical member of the demo pool (section 5.7)                                                                                                    | Patient resource in both FHIR stores (same BSN, own local identifiers)                       |
| BSN             | each pool patient gets its own test BSN; 999911120 is Anna's placeholder (passes the elfproef, but **all pool BSNs must be checked against the published RvIG test-BSN set during implementation and replaced if absent**) | Patient.identifier                                                                           |

The role code is a RoleCodeNL value from the Nictiz value set for OID 2.16.840.1.113883.2.4.15.111. "Internist
ouderengeneeskunde" was not a valid entry: 01.016 Internist, 01.022 Klinisch geriater and 01.047 Specialist
ouderengeneeskunde are distinct codes. 01.022 is used because De Plataan is the consuming hospital; 01.047 is the
care-home physician role and belongs to De Zonnebloem's side.

### 5.3 Clinical content (the fixed dataset)

Two-source data so the home screen can show attribution honestly. Defined below for Anna; every pool patient carries the
same two-source structure with superficially varied names and values, so repeated demos do not look identical:

Seeded at **De Plataan** (own hospital data, visible from step 3):

- Condition: atrial fibrillation (2025, cardiology De Plataan).
- Medication: apixaban 5 mg 2dd (De Plataan prescription).

Seeded at **De Zonnebloem** (the BGZ retrieved in steps 4-5 and shown from step 6):

- Allergy: penicillin (suspected, registered 2019).
- Medication: metoprolol 50 mg 1dd, metformin 500 mg 2dd.
- Conditions: type 2 diabetes (2012), hypertension (2009).

The enriched home screen renders exactly this union, every element tagged with its source. Counts shown in either UI
must be derived from the seed, not hard-coded. The issue author notes there is no BGZ test dataset yet (to be requested
in COT); until one lands, this synthetic set is the stand-in and is structured as BGZ sections so a real set can replace
it.

### 5.4 Exchange plumbing

| Fact                                                 | Where it lives                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | Who reads it                                            |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------|
| "Zonnebloem has a record for pseudonym(BSN)"         | NVI List resource (fake-NVI store), seeded and re-registered on ZorgDossier save                                                                                                                                                                                                                                                                                                                                                                                                   | Knooppunt NVI component, queried by the sandbox backend |
| "De Plataan has a record for pseudonym(BSN)"         | NVI List resource, published during the patiëntaanmelding (step 3)                                                                                                                                                                                                                                                                                                                                                                                                                 | same                                                    |
| Zonnebloem's endpoints (FHIR, notification)          | mCSD admin directory (`sunflower-admin`), synced to the query directory. **Must point at Zonnebloem's PEP**: the current seed points straight at HAPI (`http://localhost:7050/fhir/sunflower-patients`), which would bypass PEP/PDP/Mitz entirely, and the compose PEP is configured for URA 00000666; repointing the endpoint and the PEP tenant config is E5 scope, and the seeded address must be deployment-aware (compose-internal hostname locally, cluster URL when hosted) | Knooppunt mCSD, queried by the sandbox backend          |
| Anna's consent (share with treating physicians: yes) | mock Mitz, answer flippable via the withdraw-consent control on the enriched record                                                                                                                                                                                                                                                                                                                                                                                                | PDP during the authorization step                       |
| De Plataan's Mitz subscription for Anna              | mock Mitz, created during the patiëntaanmelding. It carries no `channel.endpoint`: the composed Knooppunt configures no `notifyendpoint` and the sandbox has nothing listening, so the subscription exists but no notification is delivered until E8 builds the receiving side. The share screen says so rather than promising delivery                                                                                                                                                                                                                                                                                                                                                                                            | sandbox backend (renders the step 7 notification)       |
| Access policy (BGZ scope requires Mitz consent)      | OPA bundles in the PDP                                                                                                                                                                                                                                                                                                                                                                                                                                                             | PEP and PDP on inbound retrieval                        |
| Dezi session claims of S. el Amrani                 | mock Dezi login; the attestation becomes a `DeziUserCredential` inside the Nuts node                                                                                                                                                                                                                                                                                                                                                                                                                     | top bar display, PDP authentication and role sub-checks |

### 5.5 The marker record (path 0b)

When the user saves in ZorgDossier:

1. An AllergyIntolerance (category/substance/status from the form) with the marker word in `note` is written to
   Zonnebloem's FHIR store, tagged as user-created (for reset).
2. If the checkbox is on, the record is (re)registered in the NVI for Anna's pseudonym: the same `/nvi/List` call the
   seed uses. Registration must be idempotent; today `POST /nvi/List` is an unconditional create, so repeated saves
   would accumulate duplicate Lists (open point).
3. Nothing is sent to the sandbox. The later retrieval in step 4 finds the marker only if the whole chain works; the
   enriched home screen highlights any entry whose note carries the marker convention (prefix `DEMO-`).

### 5.6 Reset semantics

Reset = expunge the mutable stores (both FHIR datasets, NVI Lists, Mitz subscriptions, step-event retention) and re-run
the loader. After reset the fixed dataset is exactly restored (semantically identical fixtures; byte identity is not a
HAPI guarantee), user-created records and their NVI registrations are gone, and the demo starts clean at step 1.

Two gaps against that as implemented, both recorded in `test/testdata/README.md`:

- Reset and recycle clear Mitz subscriptions only where a mock is configured. E3 gave the standalone `mitzmock` a
  subscription store and a `DELETE` that takes an optional `providerid`/`patientid` scope; `ResetGlobal` clears
  everything and `RecyclePatient` clears one pair, both when `MITZMOCK_URL` is set. The compose overlay sets it, the
  base file does not, so a stack brought up without the overlay leaves a running demo's subscription in place — and
  says so: both paths return `ErrPartialReset` and the UI reports a partial restore rather than a clean one. Reset
  also leaves the accumulated raw XACML request bodies.
- NVI Lists registered under a BSN outside the pool survive, because the NVI tenant's pseudonymization interceptor
  rejects a List search that is not scoped to a patient, subject or source, so they cannot be enumerated to be deleted.

A third limitation sits on the Mitz path, outside reset, and is inherited from the Knooppunt rather than chosen here.
`component/mitz` reports a subscription as created when its request to Mitz fails without a parseable
`OperationOutcome`: a dial failure, a deadline, a gateway 502, an empty 401. The Knooppunt then answers 201, the share
screen renders "Consent subscription started" over a subscription that was never created, and the sandbox cannot
detect it, because success is what it was told. `main` acknowledges the quirk in a `NOTE` in `CreateSubscription`; the
fix belongs in its own PR against that component.

Seeding is idempotent so reset, redeploy and first boot are the same code path. Because demo state is per patient
(section 5.7), a global reset is rarely needed between demos: presenters consume fresh patients from the pool, and reset
replenishes the pool once it runs dry. A per-patient recycle (restore one patient to unshared) covers the common case
without touching anyone else's run. Both are demo-bar actions backed by endpoints, never a redeploy.

### 5.7 Demo pool and locking

The seed provides a **demo pool** of about six patients (Anna plus structural clones with their own names, BSNs and
local identifiers). Every pool patient is a complete, unshared fixture: local data at De Plataan, a BGZ at De Zonnebloem
with an NVI registration, and Mitz consent set to yes. A demo run consumes one patient (share, retrieve, optionally
revoke); the shared/local-only chips on the patient list show which patients are still fresh.

All flow state is keyed by patient (NVI pointer, Mitz subscription and consent answer, retrieved data, step-event runs),
so concurrent and consecutive demos isolate naturally. Free-form patient creation is deliberately out: a new patient has
no data at the source, so the indexed-pull payoff cannot happen, and authoring source data inside the sandbox would
contradict standing decision 3.

To keep a running demo safe on the shared environment, starting a run **locks** its patient: a lightweight advisory lock
on the sandbox backend (set when a scenario run starts on the patient, refreshed by activity, expiring on a TTL). Locked
patients show as "demo in progress" in the patient list, per-patient recycle refuses them, and a global reset warns and
lists active locks before proceeding (an explicit override stays possible: this is a demo tool, not production). No
presenter accounts or authentication are involved.

## 6. Visual design

Three distinguishable identities, because "these are different systems" must be visible before a single word is read.
The clickable wireframe (`sandbox/wireframe.html`) is the single source of truth for style: tokens, components, layout
and copy of all three guises are extracted from the wireframe during implementation, not from this document. This
section only records the intent per guise.

### 6.1 GF Sandbox shell

The thin frame around the demo (demo bar, path chooser, /connect later): quiet and functional, so it never competes with
the EMR it frames. In the wireframe: the slim dark-blue demo bar with the scenario label and reset.

### 6.2 Plataan EHR: warm editorial (chosen)

The primary demo surface: everything from login to enriched home screen is this EMR. After four mockup rounds (dark
clinical, soft gradient, compact enterprise, warm editorial) the warm editorial direction was chosen: cream surfaces, a
display serif, a single petrol accent, pill navigation. Two things are design requirements rather than styling: the
patient card, the "available sources" card and the source attribution chips are first-class components (they carry the
demo's core message), and the quality bar is high because this is the full-screen sales demo surface: it must look like
a product, not a test harness.

### 6.3 ZorgDossier: the source institution's own system

The elderly care institution's record system, opened in its own tab for the marker path. It must look like a different
vendor built it, and a good one: own name, mark, type and green palette, nothing shared with the other guises. The NVI
checkbox is styled as a deliberate, explained action; a test-environment banner stays visible.

The GF viewer and the full-journey view use the wireframe's dark "under the hood" styling, so the journey map's glow and
color coding stay readable against the warm EMR.

demo-ehr is reference material for the EMR guises; the sandbox backend is Go (section 3), and E7 decides ZorgDossier's
stack.

## 7. Step events (the shared substrate)

The sandbox backend emits one step event per proxied call:

```json
{
  "runId": "…",
  // correlation ID per scenario run
  "seq": 3,
  "gf": "localization",
  // GF label: pseudonym|localization|addressing|authentication|consent|authorization|exchange
  "actor": "sandbox-backend",
  "request": {
    "method": "GET",
    "path": "/nvi/List?…",
    "body": null
  },
  // sanitized, allowlisted fields only
  "response": {
    "status": 200,
    "body": {}
  },
  // sanitized, allowlisted fields only
  "outcome": "ok",
  // ok|deny|error
  "durationMs": 180,
  "ts": "…"
}
```

- The patiëntaanmelding (NVI publish, Mitz subscribe) and the inbound Mitz consent notification are events in the same
  stream, with `gf: "localization"` and `gf: "consent"` respectively.
- Transport: SSE to the browser for the run's own events; bounded in-memory retention (survives a page refresh within
  the window).
- Secret redaction: capture only allowlisted headers and payload fields. Authorization headers, cookies, access tokens,
  credentials and other secrets are removed before the event object is constructed, so they are neither retained nor
  streamed.
- Identifier redaction: synthetic test BSNs are plaintext locally and masked on shared deployments (deployment flag, not
  always-on).
- Demo consumes `gf`, `outcome`, `durationMs` plus selected response fields (the result screens render real chain
  output); the full records are retained for the deeper inspector level (section 8).
- Pseudonymization is interior to the Knooppunt (the sandbox backend never calls PRS itself), so `gf: "pseudonym"`
  entries derive from the Knooppunt's OTel span rather than the capture middleware. On the hosted environment that span
  is a real call to the PRS acceptance environment; offline it is a real call to the mock PRS.
- Request strings shown in the wireframe's Technical mode are illustrative, not API contracts: the NVI List API requires
  `patient:identifier=...` (not `patient=`), and BGZ retrieval is a set of resource-level FHIR queries rather than one
  `/bgz/Bundle` call.

## 8. /connect later, without an orchestrator

The transparency goal once assigned to a /debug path ships with /demo: the GF viewer renders the always-on step-event
stream (section 7) per step and per run, scoped to the scripted chain on the demo pool. What remains for later is one
deeper inspection level plus the counterpart role, both under /connect.

### The inspector (the /debug remainder)

An opt-in detail level of the same viewer, not a separate capture pipeline or route:

- Per step: the stored request/response bodies from the capture middleware, GF labelling, and the PDP decision with
  reasons (emitted mainly on deny/error today).
- Interior Knooppunt steps (Mitz XACML call, pseudonymization, PIP lookups, OAuth2/VC exchange) via correlated OTel
  spans: presence and timing. This requires a queryable trace backend (today spans only export to Aspire, which has no
  query API); adding Tempo/Jaeger or similar is the one new infrastructure piece, needed when /connect lands, not for
  /demo v1.
- Within /demo the inspector stays scoped to the demo pool patients; cross-system troubleshooting beyond that is
  /connect territory.
- Because the inspector renders raw payloads, a shared deployment keeps it behind authentication and synthetic data
  only.

### /connect

The sandbox plays a counterpart role; external parties bring their own stack and use the inspector to troubleshoot their
own runs and fill the gaps in their implementation:

- Externally initiated calls bypass the sandbox backend by definition, so step events cannot cover them; correlation
  comes from OTel spans and PEP logs, rendered through the same inspector. This is why dropping the orchestrator cost
  /connect nothing: it was never on that path.
- Mode 1 (hosted mocks, no external credentials): register your organization and endpoints (mCSD admin), be discovered,
  run an exchange against the seeded source (De Zonnebloem) or play the source yourself with Care2Cure as the
  sandbox-side consumer, call the PDP standalone.
- Mode 2 (national proeftuin, onboarding-gated): per-GF onboarding documentation and links; out of scope for the hosted
  sandbox itself.
- Open piece, unchanged from the planning doc: the ingress/correlation story for inbound calls, tenant identity derived
  from authentication rather than the spoofable `X-Tenant-ID` header, and run scoping (an external party sees only its
  own runs in the inspector).

## 9. Build plan: epics

Epics for the /demo release. The clickable wireframe (`sandbox/wireframe.html`, a design artifact, not production code)
is the screen-by-screen reference for layout and copy. Each epic names the existing components it extends, so nothing is
green-field by accident. The two redesigns live inside E1 (Plataan EHR) and E7 (ZorgDossier), not as separate epics, so
no screen ever ships unstyled. Every UI-bearing epic (E1-E4, E7 and E8) follows the section 3 Datastar-first decision
and its AI-assisted Preact + htm fallback; E6 keeps the shared event contract framework-neutral so that migration does
not require backend rework.

### E1 Sandbox shell and Plataan EHR design system

Landing/path chooser (/, /demo; /connect as a visible-but-disabled stub), the slim demo bar (scenario label, reset), the
Plataan EHR app skeleton implementing the section 6.2 design system (tokens, sidebar, top bar, cards, buttons) as
reusable components, and the GF viewer shell: the collapsible right-hand side panel with its edge tab,
Functional/Technical switch and journey strip (live states arrive with E6's step events; the full-journey page can land
with E6). Implement the v1 UI with vendored Datastar while keeping backend routes, the step-event contract, CSS/design
tokens and the journey-map driver independent enough for a direct AI-assisted migration to buildless Preact + htm if the
section 3 fallback triggers occur.

- Extends: the `mcsdadmin` template/embed pattern is the code precedent; demo-ehr's pages are reference material only.
- Acceptance: a user lands on /, picks Demo, sees the Plataan EHR login; chrome matches the wireframe; all UI copy is
  English; Datastar drives the server-rendered interactions without coupling the backend event contract or shared visual
  assets to Datastar.

### E2 Dezi login and session (GF Authentication)

The split-screen mock Dezi login and a backend session carrying the practitioner claims (name, role, UZI number, URA),
displayed permanently in the top bar. The session holds the signed Dezi attestation, which the access-token request
submits as `id_token`; the resulting claims feed the PDP input downstream.

The mock VC issuer is not involved. The Nuts node builds the Dezi credential itself from the submitted attestation,
through `credential.CreateDeziUserCredential` in the nuts-node dependency (`vcr/credential/dezi.go` in that module, not
a path in this repository); `mock-components/dezi/contract_test.go` calls it here against the real validator.
Request-scoped credentials are passed as unsigned self-asserted JSON and rejected if they carry an issuer, and the
organization identity is a wallet-held `X509Credential` derived from a UZI certificate, which cannot be self-asserted
because the validator requires a `did:x509` issuer. `NUTS_ISSUER_DID` belongs to the vektis-issuer's own Nuts-delegated
signing mode and nothing on this path reads it. Should a BGZ presentation definition later require the issuer's
`HealthcareProviderRoleTypeCredential`, that is a choice made when authoring the definition, not a standing dependency.

- Extends: sandbox backend (session handling), `config/policy/policy.json` (a BGZ scope must exist or the token request
  fails with `invalid_scope`).
- Acceptance: after "Sign in with Dezi" every screen shows name, role, UZI number and "Logged in to: Ziekenhuis De
  Plataan", each as the attestation spells it; the session produces a service access token whose introspected claims
  populate everything the Mitz check requires.

### E3 Patient registration (patiëntaanmelding)

The patient list (records with pre-existing local data, a shared/local-only status chip and a "demo in progress" marker
for locked patients, section 5.7), and the sharing flow: publication of the NVI localization record for the Plataan
tenant and the Mitz subscription, both confirmed on screen with their registered attributes. Registration is framed as
sharing an existing local record, not creating a patient; no clinical data leaves the source.

- Extends: Knooppunt NVI List API (called through the sandbox backend; (re)registration needs defined idempotency
  semantics, the API today does an unconditional create); mitzmock already captures subscriptions, so the E3 delta is the
  subscription itself. Controllable consent changes and notification delivery are E8: E3 creates a subscription with no
  `channel.endpoint`, because no `notifyendpoint` is configured and nothing here receives one.
- Acceptance: after registration the NVI holds a List for pseudonym (BSN) with holder URA 00000010, the mock Mitz holds
  a subscription for De Plataan, and both facts are shown in the confirmation cards without claiming more than the calls
  established: the cards distinguish a subscription this request started from one it found, and an outcome nobody could
  establish from a confirmed failure. One exception is inherited from the Knooppunt and recorded in section 5.6.

### E4 Retrieval chain and result screens

The scripted chain in the sandbox backend plus the screens that render it: record home (own data, sources card, retrieve
button), the retrieve modal (NVI result versus GF Addressing result, explicit confirmation), the chain itself (NVI List
query, whose pseudonymization leg runs against the PRS acceptance environment inside the Knooppunt; mCSD endpoint
lookup; access token via `/nuts/auth/v2/{subjectID}/request-service-access-token` on the internal mux; FHIR BGZ
retrieval), the authorization result page (real top-level decision, sub-check breakdown, demo disclaimer), and the
enriched home with per-item source attribution and marker highlighting.

- Extends: rewrites the stale `/nvi/DocumentReference` module (`mock-components/demo-ehr/src/api/nviApi.js`) to the List
  API, and widens the demo-ehr proxy allowlist, which today only permits the obsolete DocumentReference query (NVI List
  search, mCSD Endpoint search, token and BGZ retrieval calls are all new entries).
- Depends on: E5 (data must exist), E6 (step events drive the live screen states).
- Acceptance: one confirmation click runs the full chain against the stack; the #532 acceptance criteria for the
  localization split, explicit confirmation and the authorization breakdown hold; every retrieved item carries a De
  Zonnebloem source chip. Note: this is the first full-chain BGZ integration; the e2e suite covers the links separately
  (NVI registration/search, mCSD sync, PDP BGZ decision in isolation, and a full PEP flow only for medicatieoverdracht
  with different credential types), so E4 should land a derived full-chain e2e test.

### E5 Canonical dataset, seed and reset

Everything in section 5: the plataan vector (URA 00000010), the idempotent bootstrap (did:web, wallet-held
`X509Credential` for the URA, mCSD Organization plus Endpoint, tenant registration), Anna's two-source clinical data,
the seeded NVI registration for De Zonnebloem, BSN verification against the RvIG test set, and the reset endpoint
(expunge mutable stores, re-run the loader, and clear Mitz subscriptions where a mock is configured, reporting a partial
restore where one is not; see section 5.6). The seed must run both as the
compose init service and as a job in the hosted deployment; reset reuses it. Seeding covers the whole demo pool (section
5.7), and the reset endpoint gains a per-patient recycle variant; both respect demo locks. The hosted deployment also
configures the pseudonymisation component against the PRS acceptance environment (`prsurl`); offline compose points
the same setting at the mock PRS (PR #561).

- Extends: `test/testdata` vectors, the `init` compose service and its hosted seed-job counterpart, HAPI `$expunge`.
- Acceptance: a fresh deployment (compose locally, Helm hosted) yields a findable, addressable, retrievable Anna without
  manual steps; reset returns every store to the published fixture and removes user-created records, without
  redeploying, within the two limits named in section 5.6.

### E6 Step events (capture middleware)

Capture middleware on the sandbox backend proxy: correlation ID per scenario run, one step-event record per forwarded
call following the section 7 schema, allowlist-based capture with secret redaction before event construction, SSE to the
browser, bounded in-memory retention, and deployment-conditional BSN redaction. The run correlation also drives the
per-patient demo lock (section 5.7). Feeds the demo's live states now and the /connect inspector later. The event schema
and resume semantics are framework-neutral: Datastar consumes them first, but a Preact + htm fallback consumes the same
contract without backend changes.

- Acceptance: events arrive in order, carry the run's correlation ID, survive a page refresh within the retention
  window, contain no authorization headers, cookies, access tokens, credentials or other secrets, and can be consumed
  without Datastar-specific fields or transport behavior.

### E7 ZorgDossier source app and path B marker entry

The source-side extension of demo-ehr: split the source guise into the standalone ZorgDossier application (own port,
opened via `target="_blank"` from the sandbox card), restyle it per section 6.3 into a credible product from another
vendor, and build the path B flow: the allergy entry form with a free-text note for the marker word (`DEMO-` prefix
convention), the explicit NVI checkbox that (re)registers the record via `/nvi/List` on save, and tagging of
user-created resources for reset. It uses the same Datastar-first, buildless stack and the same Preact + htm fallback
rule as the sandbox, without sharing visual components or branding.

- Extends: demo-ehr (FHIR write plumbing and proxy reused); the NVI registration path is shared with the E5 seed.
- Acceptance: a record entered in ZorgDossier is retrievable in the sandbox purely through the chain; the marker word
  appears highlighted in the enriched record with the "just entered by you in ZorgDossier" chip; reset removes it from
  both the FHIR store and the NVI.

### E8 Consent revocation (GF Consent)

Promote `test/mitzmock` from an e2e-test helper to a standalone compose service with a consent toggle and the
subscription/notification flow: flipping consent notifies subscribers (the E3 subscription), the EHR shows the Mitz
notification and removes De Zonnebloem's data from the overview, and a new retrieval independently triggers a fresh
closed question and fails on the consent sub-check (fail-closed) with the deny breakdown. Notification processing is
demo UX plumbing, not an authorization dependency.

- Extends: `test/mitzmock`, the Knooppunt's `/mitz/notify` handler (which today logs the notification and discards the
  body), sandbox backend (notification intake and correlation to the patient), the bell/notification UI.
- Acceptance: the #532 revocation criteria hold: notification received, source data removed from the overview,
  re-retrieval blocked by a fresh consent check even if the notification is delayed or lost; restoring consent restores
  the happy path.

### Sequencing

E1, E5, E6 and E7 can start in parallel. E2 and E3 build on E1; E4 needs E5 and E6; E8 closes the flow and needs E3's
subscription. The open question on the authorization breakdown source (section 10) must be settled at the start of E4.

## 10. Open points

- Verify BSN 999911120 against the published RvIG test set (E5) and replace if absent. Extra reason to check: this BSN
  appears in the Nictiz BgZ test fixtures as a different persona (male, born 1964), which invites cross-fixture
  confusion; prefer a BSN unused in public fixtures.
- No BGZ test dataset exists yet; ask in COT (per the issue author) and swap the synthetic set of 5.3 for it when
  available.
- The authorization sub-check breakdown (step 5) needs a source: in production the PDP returns yes/no. Decide at the
  start of E4 whether the demo derives the breakdown from PDP debug output or scripts it alongside the real top-level
  decision, and keep the disclaimer either way.
- Scope of the mock Dezi login: which claims the stub issues (name, role, UZI, URA) and how they flow into the token/PDP
  input.
- Mitz subscription and notification shape in the mock: no national spec is implemented in `mitzmock` yet, and the
  current `/mitz/notify` handler logs and discards notifications. Keep the mock's interface minimal and swappable, and
  make E8's run-scoped notification delivery explicitly independent from authorization enforcement.
- Naming: "GF Sandbox", "Plataan EHR" and "ZorgDossier" are working titles. Issue #532 called the hospital "Ziekenhuis
  ZMC", but ZMC is the abbreviation of a real hospital and demo organizations must be fictional; hence "Ziekenhuis De
  Plataan", following the botanical naming of the demo organizations (the existing `sunflower` vector maps to De
  Zonnebloem). Check any future name against real organizations before it lands in code.
- Hosting: the sandbox is demoed from a hosted environment (extending the existing Helm charts); docker compose is the
  offline dev path only. Charts for the sandbox and ZorgDossier apps, plus the seed job, are needed for the first hosted
  deploy.
- PRS acceptance environment: confirm access (connection details, auth and allowlisting of the demo URAs
  00000010/00000020) and that RvIG test-BSNs are accepted there. Note the failure mode: with `prsurl` set, an
  unreachable PRS fails the lookup (no silent fake fallback), so a demo depends on acc availability; decide whether that
  is acceptable or needs a visible degradation.
- ~~The NVI registration needs a BGZ zorgcontext code.~~ Settled by E3: there is none, and there cannot be. `List.code`
  binds to `nl-gf-zorgcontext-vs`, whose 28 codes come from `nl-gf-data-categories-cs` and are data categories at FHIR
  resource granularity. A patient summary is registered as the set of categories it contains, one List each. The
  `MEDAFSPRAAK` the e2e vector uses is not a member of that value set at all.
- Demo-lock details (section 5.7 sketches the intent): the exact lock trigger (record open versus share), the TTL, and
  whether the global-reset override needs more friction than a confirmation dialog.
- The BGZ retrieval contract: the PDP's BGZ policy authorizes resource-level FHIR searches (and is itself marked "to be
  formalized", including a temporary BSN workaround); pin down the exact query set, aggregation behavior and the Nuts
  policy/presentation definition for the BGZ use case at the start of E4.
