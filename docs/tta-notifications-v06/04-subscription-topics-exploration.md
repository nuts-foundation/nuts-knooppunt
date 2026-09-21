# GF IG — Subscription Topics Exploration (working-group exploration, not specification)

> Source: https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/subscription-topics-exploration.html
> English extraction. Examines granularity levels (L0–L4) for SubscriptionTopics in
> subscriber-initiated Notified Pull without workflow context. FHIR R4 + Subscriptions R5 Backport.

## What a SubscriptionTopic actually is (in R4 backport)

A topic is a **design-time contract, not a runtime object**:

- On the wire only a **canonical URL** appears, in `Subscription.criteria`
- R4 has no native SubscriptionTopic resource; topic definitions live in IG documentation plus an
  optional R4B/R5-format JSON reference
- Vendors implement trigger logic manually from the published specification
- Servers advertise supported topic URLs via the CapabilityStatement SubscriptionTopic Canonical extension
- Subscribers can only filter on `canFilterBy` parameters declared by the topic, carried in the
  `backport-filter-criteria` extension

Consequences: a **finite national topic set is an architectural necessity**, and topic granularity
directly determines vendor implementation cost. Granularity has two independent axes: **event
granularity** (which changes trigger events) and **topic-artifact granularity** (narrow topics vs one
broad topic with filters).

## Topic matching vs access evaluation

Two sequential gates control delivery:

1. **Topic matching** — does the change match triggers + filters? Nationally defined, uniform.
2. **Access evaluation** — may *this* subscriber be notified about *this* event? Even id-only
   notifications constitute disclosure requiring authorization evaluation. Mandatory. Suppression is
   silent — subscribers cannot distinguish "nothing occurred" from "withheld".

Recommended practice: scope defined by topic; full stream delivered; access re-enforced at pull time.
Lapsed legal basis ends the Subscription via `status = off` (final-notification convention unspecified).

## The national patient filter

A national `SearchParameter` (`http://fhir.nl/SearchParameter/patient-identifier`, code `patient`,
type token) matches BSN identifiers carried on subject references across resource types, e.g.
`Patient.identifier | Observation.subject.identifier | Condition.subject.identifier | ...`.

## Anchor use case

Hospital EHR (URA 22222222, `https://hospital-ehr.example.org/fhir`) as Subscription Server; GP system
(URA 33333333, notification endpoint `https://gp-his.example.org/notifications`) as Subscription
Client; patient BSN 999911120. GP keeps its **BgZ** copy current through pure Notified Pull.

## Scenario L0 — catch-all topic (calibration baseline)

- `http://fhir.nl/SubscriptionTopic/patient-record-changed`; any CRUD on any resource in the patient
  compartment; filter: `patient`; payload `id-only`.
- Requires enumerating ~60+ resource types as `resourceTrigger`s (no wildcard/compartment triggers in any FHIR version).
- Subscription example carries `backport-filter-criteria: patient=http://fhir.nl/fhir/NamingSystem/bsn|999911120`
  and `backport-heartbeat-period: 86400`.
- Observations: complete but maximal noise; privacy leak from resource-type existence (e.g. a
  Condition from a psychiatric department); trivial governance; poor forward compatibility.

Notification example (as in TTA spec) uses profile
`backport-subscription-notification-r4` on the Bundle and `backport-subscription-status-r4` on the
Parameters resource; focus is an absolute URL at the server.

Client processing per notification: match Bundle to stored Subscription and verify sender → check
event-number continuity (recover via `$events`) → read focus URL to learn resource type → decide
whether to pull → update local record or discard.

## Scenario L1 — one topic per resource type

- Topics like `observation-changed`, `condition-changed`, …; filters: `patient` + standard search
  parameters (`category`, `code`, `class`).
- BgZ coverage requires **19 subscriptions per patient** (patient-changed, consent-changed,
  observation-changed with category filter, 3 medication topics, procedure-changed, encounter-changed
  with class filter IMP/ACUTE/NONAC, servicerequest-changed, plus ~10 more).
- Multiple filter extensions AND together; commas OR within one parameter.
- Observations: complete; noise below L0; 19× bookkeeping (1,000 patients = 19,000 subscriptions and
  heartbeats); intent invisible; no authorization anchor; good forward compatibility (comma-OR
  undefined in R5/R6).

## Scenario L2 — one topic per data category (GF zorgcontext, 28 codes)

Value set: https://minvws.github.io/generiekefuncties-docs/ValueSet-nl-gf-zorgcontext-vs.html

- **L2a** single topic `patient-data-changed` with `data-category` filter: one subscription per
  patient, but no per-type narrowing, category classification custom per source, comma-OR undefined,
  BgZ gaps (no category for payment details/Coverage, vaccinations/Immunization, functional status).
- **L2b** 28 topics (one per category), generated from a mapping table, expressing categories as
  ordinary triggers and reusing type search params as filters: per-type narrowing returns, R5/R6
  portable, but L1-style bookkeeping (~16 subscriptions/patient for BgZ).
- Authorization binding per category mirrors the localization-index vocabulary.
- Governance: VWS owns categories, Nictiz owns datasets; the dataset↔category mapping needs an owner.

## Scenario L3 — one topic per dataset (e.g. `bgz-changed`)

- **L3a change events**: every write to a resource belonging to the dataset is an event. The trigger
  list is the dataset's **PlanDefinition query list recast as write-time predicates** (generated, not
  hand-maintained), e.g. Observation trigger with
  `queryCriteria.current = "category=...|laboratory"`, Encounter trigger with `class=IMP,ACUTE,NONAC`.
  One subscription, one event stream, one heartbeat per patient.
- **L3b snapshot events**: `bgz-published` — source publishes/updates a **container** (`List`, per the
  Notified Pull container pattern); publication is the event. Continuous (update per member change) or
  moment-based (discharge, episode end). Focus URL leaks no resource type — best privacy. Container is
  a materialized dataset view maintained only while subscriptions are active.
- Observations: no noise by construction; explicit intent ("follow this patient's BgZ"); direct
  authorization binding (one rule per dataset, systeemrol like "BgZAbonnerend"); best forward
  compatibility (`queryCriteria` is runtime configuration; R6 renames `resourceTrigger`→`trigger`).

## Scenario L4 — per-case curated set (sender-initiated)

Not a subscription granularity for the standing-subscriber case. Source curates resources for one
exchange and notifies the recipient: generic topic `curated-set-published` filtered on recipient;
subscription is **partner-wide and permanent**; per-case scope lives in the container. This is the
Notified Pull container pattern with the COW workflow above it. "The topic names the exchange pattern,
the sender names the data."

## Interest direction

- **Receiver-asserted (L1–L3)**: interest declared in the Subscription (topic + patient filter); one
  subscription per patient+dataset; no recipient designation on the data; partner-wide filters not
  expressible (cross-resource join).
- **Sender-addressed (L4, workflow)**: interest declared in the data (`Task.owner`, recipient on the
  container); one permanent subscription per partner; plain filter on the focal resource.
- Directions do not mix; one exchange has one interest holder. A COW fulfiller may additionally hold a
  receiver-asserted dataset subscription with workflow-bounded lifetime.

## Scoring matrix

| Axis | L0 | L1 | L2a (1 topic) | L2b (28 topics) | L3a | L3b |
|------|----|----|----|----|-----|-----|
| BgZ completeness | ++ | ++ | − | − | ++ | ++/0 |
| Noise | − | 0 | − | 0 | ++ | ++ |
| Subscriber cost | − | − | + | − | + | ++ |
| Source cost | − | 0 | − | 0 | 0 | − |
| Intent | − | − | 0 | 0 | ++ | ++ |
| Authorization binding | − | − | + | + | ++ | ++ |
| Governance | ++ | + | 0 | 0 | 0 | 0 |
| Privacy | − | 0 | 0 | 0 | 0 | ++ |
| Forward compatibility | − | + | − | ++ | ++ | ++ |

Reading: L0 is calibration only. L1 eliminated as complete-dataset strategy (viable for
single-concern subscribers). **L3a dominates both L2 forms** in the author's scoring; L3 (dataset
topics) is the recommended direction. Anchor case is dataset-shaped, which flatters dataset topics.

R5/R6 rule of thumb: **semantics in triggers are portable** (run on standard search, ship as
configuration); **semantics in filters are not** (anything not resource-readable needs custom code).

## Open questions

1. Will the data-category value set be completed with normative dataset↔category mapping?
2. Is the dataset topic auto-generated from the dataset's PlanDefinition; who owns the pipeline (Nictiz)?
3. Change events (L3a) vs snapshot publication (L3b): fixed by standard or source choice?
4. Does "abonneren" become a Nictiz activity verb so systeemrollen/autorisatierichtlijn rows can name subscriptions?
5. Lifecycle convention for lapsed legal basis (`status = off`, final notification) — needed everywhere, defined nowhere.
6. Do resource-type topics coexist with dataset topics for single-concern subscribers?
7. Who publishes/maintains the national cross-resource `patient` SearchParameter?
8. Does COW reuse dataset topics unchanged for its data stream, or need case-scoped subscriptions?
