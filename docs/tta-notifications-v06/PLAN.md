# Implementation plan — POC TTA Notifications v0.6 in nuts-knooppunt

Branch: `tta-notifications-v06` (currently identical to `main`, green-field).
Budget: 3 code days + 2 reporting days, delivery 21 Sep 2026. POC quality: answer the research
questions, not production code.

## Scope (from the assignment)

- **Subscription Server**: in-band + out-of-band subscription creation, handshake, event
  notifications, heartbeats, `$status`, `$events`, exponential-backoff retry
- **Subscription Client**: notification endpoint, event-number continuity check, heartbeat-miss
  detection, idempotent processing
- **Payload modes**: `empty` and `id-only` only (`full-resource` excluded)
- **Example topic**: SubscriptionTopic on `Task` for eOverdracht alignment (COW profile: broad
  long-lived per-partner subscription, id-only, Task filtered on owner/use-case code)
- **Logging**: all sent/received subscriptions and notifications
- **Permutation tests**: server–client pairs choosing different optional features

## Key constraints found in this repo

1. **No R5-backport FHIR types.** `zorgbijjou/golang-fhir-models` is R4-only: `fhir.Subscription` has
   no topic/content/heartbeat; `SubscriptionTopic` and `SubscriptionStatus` don't exist. → hand-write
   Go types + the backport extension constants (precedent: `component/mitz/xacml/types.go`).
2. **Working template exists**: `component/mitz` (R4 Subscription validation, rest-hook receiver,
   config-driven endpoints) + `test/e2e/mitz/subscription_test.go` + `test/mitzmock/subscriptions.go`.
3. **Two registration styles**: generated strict server from `openapi.yaml` (internal mux, via
   `cmd/apiserver.go` forwarders + `api/errors.go` Visit methods) and a second spec for the public mux
   (`mitz-public.openapi.yaml` → `api/mitzpublic`). Cross-org FHIR endpoints belong on the
   **public** mux.
4. **No persistence layer** — components are in-memory (mcsd/lrza pattern) or delegate to a HAPI FHIR
   tenant via `go-fhir-client`.
5. **No inbound mTLS** anywhere in Go or the checked-in nginx PEP → spec precondition "mutual TLS MUST
   be applied" cannot be met as-is (this is itself a finding, research question 5).
6. **No mCSD endpoint-resolution helper** ("URA → Endpoint with payloadType X") — new code, but
   `coding.CodablesIncludesCode` + `fhirclient` search on the Query Directory suffice (RQ 9).
7. **Single knooppunt in docker-compose**; two orgs (Plataan URA 00000010, Zonnebloem URA 00000020)
   exist as tenants/PEPs of one instance. The e2e harness starts knooppunt in-process via
   `cmd.Start`, so two instances in one test are easy.
8. Existing eOverdracht/BgZ Notified Pull (STU3 Task POST) is documented in
   `docs/bgz-notified-pull-sd.puml` / `docs/eoverdracht-sd.puml` and implemented in
   `mock-components/demo-ehr` — the comparison baseline for RQ 10.

## Architecture decisions

**D1 — One new component, two roles.** `component/subscription` implementing `component.Lifecycle`,
with `server` and `client` sub-packages and independent config toggles
(`subscription.server.enabled`, `subscription.client.enabled`). One knooppunt can run either or both —
this is exactly what the permutation tests need, and mirrors how the spec assigns roles per organisation.

**D2 — Hand-written backport types in `lib/fhirsubscription`.**
- `Subscription` (R4 base + `backport-topic-canonical`, `backport-filter-criteria`,
  `backport-payload-content`, `backport-heartbeat-period` extensions; marshal/unmarshal helpers that
  read/write the `_criteria`/`_payload` extension shapes)
- `SubscriptionTopic` (R4B/R5-format JSON as design-time artifact; loaded from config/testdata, per
  the IG: "a topic is a design-time contract, not a runtime object")
- Notification `Bundle` builder + parser (`history` type, `Parameters`/SubscriptionStatus with
  subscription/status/type/notification-event parts) for handshake, heartbeat, and event notifications
- Extension/profile URL constants from the backport IG

**D3 — Storage: in-memory with a small store interface.** POC goal is spec feedback, not durability.
`SubscriptionStore` and `EventLog` interfaces with mutex-guarded in-memory implementations
(mcsd `lastUpdateTimes` pattern). The event log keeps every sent notification per subscription —
it serves `$events` replay, the continuity story, and the logging obligation in one structure.
Non-durability across restarts is recorded as a POC limitation, not solved.

**D4 — Event detection: poll the FHIR store, plus a manual trigger.** The server watches the org's
HAPI tenant for Task changes matching the topic via `_history?_since` polling (reuse
`fhirutil.DeduplicateHistoryEntries`; same idiom as mcsd/lrza). Additionally an internal endpoint
`POST /subscription/trigger` fires an event directly — invaluable for demos and permutation tests.
Evaluating `resourceTrigger.queryCriteria`/`canFilterBy` in Go against polled Tasks is deliberately
minimal: topic hard-coded to the eOverdracht Task topic semantics (owner-URA filter, use-case code),
with the general problem written up as a finding (RQ 3/4: what should the node abstract?).

**D5 — Public FHIR surface via a new OpenAPI spec.** `subscription-public.openapi.yaml` → generated
strict server package `api/subscriptionpublic` (copy the `mitzpublic` wiring):
- Server side: `POST /fhir/Subscription` (in-band create), `GET /fhir/Subscription/{id}`,
  `GET /fhir/Subscription/{id}/$status`, `GET /fhir/Subscription/{id}/$events` (with
  the backport IG's `eventsSinceNumber`/`eventsUntilNumber` params — the plan originally
  proposed kebab-case names on the mistaken premise that v0.6 leaves them undefined; see
  FINDINGS RQ 1.1)
- Client side: `POST /fhir/notifications` (rest-hook notification endpoint)
- All error paths return `OperationOutcome` (reuse `lib/fhirapi`)
Internal mux (main `openapi.yaml`): out-of-band subscription creation
(`POST /subscription/subscriptions`, the org's own system asks its knooppunt to set up a subscription
towards a partner), list subscriptions, trigger endpoint, and client-side status inspection — with the
`cmd/apiserver.go` forwarder + `api/errors.go` Visit methods.

**D6 — Server engine.**
- In-band create: validate against pre-agreed topic list + allowed payload modes (spec: "SHALL NOT
  accept arbitrary subscription requests"), store as `requested`, 201/400/422 with OperationOutcome
- Out-of-band create: server constructs the Subscription itself, resolving the partner's notification
  endpoint from the mCSD Query Directory (new `payloadType` code, e.g. `tta-notification`; falls back
  to config) — answers RQ 9
- Handshake goroutine: POST handshake bundle → 200 ⇒ `active`; retry with exponential backoff
  (bounded attempts) ⇒ persistent failure sets `error`
- Event pipeline: event → per-subscription mutex increments event-number (concurrency-safe per spec) →
  append to event log → build bundle per payload mode (`empty`: no focus; `id-only`: focus reference)
  → deliver with backoff retry
- Heartbeat: one ticker per subscription with `backport-heartbeat-period`, only if agreed at creation
- Authorization/consent verification before activation and before each send: a stub hook that logs its
  invocation (real GF Authorization/Mitz integration is out of POC scope; the *hook placement* is the
  research answer for RQ 5)
- **Id-only identifiers**: expose focus references under an opaque alias (HMAC of the internal id, per
  the spec's keyed-transformation example) instead of the HAPI sequential id — HAPI's default ids fail
  the opacity/unpredictability requirements, which is precisely the RQ 8 finding to demonstrate

**D7 — Client engine.**
- Notification endpoint parses the bundle, classifies handshake/heartbeat/event, always logs
- Continuity: track highest processed event-number per subscription; on gap, call the server's
  `$events` (if available) else `$status`, and record the recovery in the log
- Idempotency: processed-set of (subscription, event-number); duplicates acknowledged with 200 but not
  re-processed
- Heartbeat detection: per-subscription timer at ~1.5× the agreed period; on miss, call `$status`
- Out-of-band bootstrap: on first notification referencing an unknown Subscription, fetch the
  Subscription resource from the server to learn mode/topic (per spec, receiver MAY inspect; reject
  handshake if mode not in the agreement)

**D8 — Logging.** Structured slog entries (`direction`, `type`, `subscription`, `event-number`,
`payload-mode`, `outcome`) via the existing `lib/logging` setup + the queryable in-memory `EventLog`
exposed on the internal API for the demo UI/tests.

**D8b — The vendor-facing contract is a first-class deliverable.** Per
[09-knooppunt-vendor-split.md](09-knooppunt-vendor-split.md): the internal-mux API (event ingest,
authorization-basis registration, subscription intents, normalised-event inbox with ack) *is* the
knooppunt's simplification claim. demo-ehr plays the vendor and implements only the irreducible
minimum against it; the POC records which spec sections the vendor integration needed to read
(target: none) vs which the knooppunt needed (nearly all) — that delta is the evidence for research
questions 3/4, and inbox-payload uniformity across the permutation matrix is the evidence for
research question 6.

**D9 — Demo: second knooppunt in docker-compose.** Add a `knooppunt-zonnebloem` service (KNPT_* env
config, own ports) so Plataan (Subscription Server / sender) and Zonnebloem (Subscription Client /
receiver) are truly separate nodes; seed a Task in Plataan's tenant, show the notification arriving at
Zonnebloem, then the follow-up pull. Permutation testing itself runs in-process in e2e tests (two
`cmd.Start` instances with `http.TestConfig()` ports), which is cheaper than compose.

## The example SubscriptionTopic (eOverdracht alignment)

Per the COW profile (doc 06): canonical
`http://fhir.nl/SubscriptionTopic/nl-task-notified-pull` (placeholder), monitoring `Task`,
`supportedInteraction: [create, update]`, `canFilterBy`: owner (URA), Task use-case code
(`task-code#fulfill` + eOverdracht code); payload mode `id-only`; long-lived per-partner
subscription. Stored as R4B-format JSON in `test/testdata` + served read-only at
`GET /fhir/SubscriptionTopic/{id}` for discoverability.

## Day plan (3 code days)

**Day 1 — foundations + happy path in-band.**
`lib/fhirsubscription` types + bundle builder/parser (with unit tests against the spec's example
payloads from doc 02); `component/subscription` skeleton + config + store; public spec + codegen +
wiring; in-band `POST /Subscription` with validation; handshake send/receive; client endpoint
logging + classifying. *Checkpoint: e2e test — client subscribes in-band at server, handshake
completes, subscription `active`.*

**Day 2 — events, recovery, out-of-band.**
Event-number allocation + event log; event bundle build for `empty`/`id-only` (+ HMAC alias ids);
delivery with exponential backoff; Task `_history` poller + internal trigger; client continuity +
idempotency; `$status`; out-of-band creation incl. mCSD endpoint resolution. *Checkpoint: e2e test —
Task create → notification → client detects gap after a dropped event → recovers via `$status`.*

**Day 3 — optional features, permutations, demo.**
Heartbeat send + miss detection; `$events` replay; auth/consent hook stubs; permutation e2e matrix
(see below); second compose service + seed wiring; demo script in `docs/test-scripts/`; polish
logging. *Checkpoint: demo runs end-to-end from compose.*

Reporting days 4–5 (findings document per research question, demo presentation, conclusion for the
Werkgroep Technische Convergentie) — findings collected continuously in
`docs/tta-notifications-v06/FINDINGS.md` from day 1.

## Permutation matrix (research question 6)

Server × client feature choices, each as an e2e case with two in-process knooppunts:

| # | Creation | Heartbeat (S/C) | `$events` (S) | Payload | Expected |
|---|----------|-----------------|---------------|---------|----------|
| 1 | in-band | on/on | yes | id-only | full happy path + gap recovery via `$events` |
| 2 | out-of-band | off/off | no | id-only | client bootstraps from unknown subscription; gap recovery degrades to `$status` + full re-pull |
| 3 | in-band | on/**off** | yes | empty | server sends heartbeats client ignores — harmless? (finding) |
| 4 | in-band | **off**/on | yes | id-only | client's heartbeat timer never satisfied — spec says agreed at creation; what if not honoured? (finding) |
| 5 | in-band | off/off | **no**, client asks anyway | id-only | 404/OperationOutcome handling on `$events` |
| 6 | out-of-band, mode outside agreement | — | — | id-only | client rejects handshake (spec §payload-mode enforcement) |

## Mapping to the research questions

| RQ | Where the answer comes from |
|----|------------------------------|
| 1 spec complete/correct | FINDINGS.md, fed by every ambiguity hit while coding (e.g. heartbeat-period agreement mechanics; unresolvable relative subscription references in out-of-band mode). Two premises of this plan turned out wrong and are corrected there: `$events` parameters and the out-of-band creation "transaction" are both covered by the backport IG v0.6 mandates conformance to — the finding is that v0.6 paraphrases the IG instead of referencing it |
| 2 effort | day-plan actuals vs estimate |
| 3/4 node abstraction | the knooppunt/vendor responsibility split ([doc 09](09-knooppunt-vendor-split.md)): effort ledger of spec sections needed by knooppunt vs by the demo-ehr vendor integration |
| 5 authn/authz fit, mTLS | stub-hook placement + the inbound-mTLS gap (constraint 5) + PEP/PDP relation |
| 6 differing optional features | permutation matrix |
| 7 FHIR versions | R4 backport types vs STU3 eOverdracht (doc 06 mappings); the R4-only Go model gap |
| 8 id-only identifier fitness | HAPI sequential ids fail opacity/unpredictability → HMAC alias layer needed |
| 9 endpoint resolution via GF Adressing | mCSD Query Directory lookup by payloadType (new helper) |
| 10 diff vs current eOverdracht/TA NP | doc 06 mapping tables + `docs/bgz-notified-pull-sd.puml` baseline comparison |

## Out of scope (recorded, not built)

- `full-resource` payload mode (excluded by assignment)
- Durable storage / restart survival; PATCH of payload mode; subscription search parameters beyond a
  basic list; real GF Authorization / Mitz consent evaluation (stub hooks only); inbound mTLS
  (finding, not implementation); generic `queryCriteria` evaluation engine (single hard-wired topic)
