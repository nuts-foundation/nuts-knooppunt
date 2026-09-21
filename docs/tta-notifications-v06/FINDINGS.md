# FINDINGS — POC TTA Notifications v0.6

Status: living document, updated as the POC progresses. Collected from implementing both
roles in `component/subscription` + `lib/fhirsubscription` on branch `tta-notifications-v06`,
and from the permutation e2e suite in `test/e2e/subscription`. Organised by research
question (see [01-poc-assignment.md](01-poc-assignment.md)); spec-facing findings are
marked **[SPEC]** (candidate input for the Twiin 1.6 consultation), implementation-facing
ones **[IMPL]**.

## RQ 1 — Is the specification complete and correct?

**Verdict:** the core is implementable as written. The payload model, the notification
Bundle shape, the in-band creation transaction, the handshake, event numbering and the
recovery operations were built from the text plus the backport IG it mandates, and the
four example payloads (creation, handshake, event, heartbeat) were used verbatim as
unit-test vectors in `lib/fhirsubscription` — they parse and round-trip without
adjustment.

An important framing point for the consultation: v0.6 says "implementations MUST conform
to the HL7 FHIR R4/R4B Subscriptions R5 Backport IG". Much of what v0.6 *restates* is
therefore already fully defined upstream, and several apparent gaps in the v0.6 text are
not gaps at all once the IG is read as normative. The findings below are tagged by where
the fix belongs:

- **[EDIT]** — v0.6 restates something the backport IG defines, incompletely or
  inconsistently; fix by referencing the IG (or restating faithfully).
- **[v0.6]** — a gap in what v0.6 adds on top of the IG (its obligations, agreements,
  Dutch context); only Twiin can fix it.
- **[ROOT]** — a gap or property of the FHIR/backport layer itself; not fixable in v0.6,
  but v0.6 may need to say how it copes.
- **[DOC]** — not a defect; a consequence worth stating explicitly.

> All findings are against the English extraction in
> [02-tta-notifications-v06-spec.md](02-tta-notifications-v06-spec.md); each should be
> re-verified against the normative Dutch text before submission to Twiin.

### Recovery operations

1. **[EDIT] `$events` is restated loosely instead of referenced.** The backport IG
   defines `$events` completely: parameters `eventsSinceNumber`, `eventsUntilNumber`,
   `content`; reply a notification Bundle with SubscriptionStatus type `query-event`.
   v0.6 only says it "retrieves a specific event-number or range" returning
   "notification-event Bundle(s)", without pointing at the IG's OperationDefinition. The
   *original* PLAN and first cut of this POC read that as "unspecified" and invented
   kebab-case parameter names — which is precisely the divergence a loose restatement
   invites. Corrected: the POC now uses the IG's names. Recommendation: replace the
   prose with a normative reference to the IG operation (and say whether `content` is
   supported; this POC ignores it and always replays in the registered payload mode).

2. **[EDIT] `$status` reply shape and fields are restated incompletely.** The IG's
   `$status` returns a **searchset Bundle** of SubscriptionStatus Parameters, and the
   SubscriptionStatus profile carries `events-since-subscription-start` — the field a
   client needs after silence — and the `type` code `query-status`. v0.6's text says the
   reply is "SubscriptionStatus Parameters" (no Bundle) and its SubscriptionStatus table
   lists only subscription/status/type/notification-event and three notification types.
   Nothing is missing upstream; v0.6's partial restatement is the risk (this POC's first
   cut returned a bare Parameters, i.e. was non-conformant to the IG — corrected).
   Recommendation: reference the IG profile and operation rather than restating.

3. **[DOC] Empty-mode gap recovery cannot tell you *what* you missed.** Inherent to
   the FHIR `empty` payload mode: a replayed event carries a number and a timestamp,
   nothing else. So `$events` under empty mode only confirms *how many* events were
   missed; the only recovery is "re-search everything", which is exactly the
   broader-exposure cost v0.6 already attributes to empty mode. Not a defect in either
   spec, but since v0.6 positions empty mode as the Notified Pull fallback for
   non-compliant ids *and* mandates the continuity check, it should say what continuity
   buys you there. The POC surfaces it to the vendor as a `gap` inbox entry.

4. **[v0.6] Duplicate delivery has no defined response.** Neither the IG nor v0.6 says
   what a receiver answers to a re-delivered notification. v0.6 is the one that
   introduced the idempotency obligation (obligation 6), so it should complete it:
   acknowledge with 200 and ignore — which the POC does. Left open, a receiver answering
   422 "already processed" would loop against a sender that retries on failure.

5. **[EDIT/v0.6] No status transition for persistent *event* delivery failure.** Base
   FHIR defines Subscription.status `error` as "the server has an error executing the
   notification" — generically, for any notification. v0.6's handshake table applies it
   ("persistent failure → error"), but its event-notification table has **no 5xx row at
   all** (only 200 and 422) and never says what happens to the subscription when event
   delivery keeps failing after backoff. The POC keeps it `active` and retains the event
   for replay. [EDIT] for the missing 5xx/`error` row; [v0.6] for the 422 case, which
   v0.6 itself defines for events (retry is pointless — is the subscription now
   `error`? does the event count as sent?). The inherited R4B value set already supplies
   the semantics to adopt: `error` = "the server has an error executing the
   notification", `off` = "too many errors have occurred or the subscription has
   expired" — i.e. transient failure → `error`, retries exhausted → `off`.

### Out-of-band creation

6. **[v0.6] The receiver cannot honour its SHALL with the spec's own examples.** The
   IG's server-managed (out-of-band) mode needs no transaction: the sender creates its
   own resource, which is all that matters. What v0.6 *adds* is a receiver obligation:
   it *MAY* inspect the payload mode "by fetching the Subscription resource" and *SHALL*
   refuse the handshake if the mode is outside the agreement. But every v0.6 example
   carries a **relative** subscription reference (`Subscription/7f3e…`), and the
   notification carries no sender identity or base URL. A receiver confronted with a
   handshake for a subscription it never created has nothing to dereference — the
   optional inspection is impossible, so the mandatory rejection cannot be performed.
   The POC emits **absolute** references, which makes bootstrap work (permutation 2). A
   sender following the example form would leave a spec-conformant receiver unable to
   comply. Fix options: mandate absolute references in SubscriptionStatus.subscription,
   or define sender identification (see RQ 5 on mTLS) plus endpoint resolution so the
   receiver can locate the resource.

7. **[EDIT] Handshake example contradicts the status table.** The SubscriptionStatus
   table says `status` is "`active` or `off`"; the handshake example carries
   `"status": "requested"`. The IG allows requested/active/error/off; the POC follows the
   example (it is the true state at handshake time). The table should list the IG's set.

### Agreement mechanics

8. **[EDIT/v0.6] Heartbeat: no resource shape named, no failure mode defined.**
   [EDIT] the IG defines `backport-heartbeat-period` as a channel extension; v0.6's
   Subscription table lists no element for heartbeat although heartbeats are "agreed at
   creation" — the POC uses the IG extension. [v0.6] "agreed" is not defined: if an
   in-band request asks for a heartbeat the server will not send, what happens? This
   server strips the extension (the created resource shows no heartbeat); the client is
   not told to re-read the created resource, keeps its timer, and issues a `$status`
   query per missed window, forever (permutation 4 — harmless busywork, only because
   this client tolerates it). Verified against the IG: it only says a server "may send"
   heartbeats and is silent on unmet requests, so this is v0.6's to define. The IG does
   have the model to copy — for payload content it says servers "SHALL reject the
   subscription request if a client asks for a content level the server does not intend
   to support". Recommendation: the same rule for heartbeat — reject with 422 rather than
   strip (the POC strips, which is what exhibits the problem).

9. **[v0.6] Filter criteria: syntax conformant, NL value convention undefined.**
   Verified against the IG (components page): `[resourceType].[filterParameter]=[value]`
   is a valid form, so v0.6's example `Task.owner=Organization/receiver-org` is
   syntactically fine (`owner` is a Task search parameter) — an earlier version of this
   finding wrongly called it non-conformant. What is undefined is the *value*: in the
   Dutch context the partner is identified by URA, not by a reference the sender can
   resolve. The POC accepts `Task.owner=<system>|<URA>` and matches on the URA. Since the
   filter is the only per-partner scoping mechanism on a broad topic, the NL value form
   needs pinning down (v0.6 or the GF IG).

10. **[v0.6/ROOT] Retirement and payload-mode change: mechanism exists, actors don't.**
    The FHIR interactions exist (update/PATCH of the Subscription). What is missing is
    *who*: "on retirement set to `off`" and "the mode SHALL only be changed via an
    explicit PATCH" — by which party, and for an out-of-band subscription that lives at
    the sender, may the receiver PATCH it at all? [v0.6]. Whether a final notification
    goes out when a subscription is turned `off` is defined in neither the IG nor v0.6
    (doc 04 open question 5) [ROOT]. Neither is built in the POC.

11. **[EDIT] Profile canonical vs FHIR version.** The Bundle is said to conform to
    `backport-subscription-notification`; in the IG that is the R4B profile, the R4 one
    is `backport-subscription-notification-r4` (the GF IG example in doc 04 uses the
    `-r4` form). For a spec targeting "R4/R4B", state the profile URL per version. The
    POC declares the `-r4` profiles.

### What was *not* a problem

For balance: the payload-mode rules and identifier requirements were unambiguous and
directly testable; the in-band creation transaction (status codes, required elements,
the four extensions) needed no interpretation; event-number monotonicity and
concurrency-safety were clear enough to test (`TestEventNumbering`); the retry and
logging obligations were implementable as stated; and everything the IG defines
(operations, profiles, extensions) worked as documented once we read the IG instead of
v0.6's paraphrase of it.

**Net for Twiin:** the strongest single recommendation is editorial — stop restating the
backport IG and reference it. Of the genuinely v0.6-level items, the out-of-band
reference/identity problem (6) is the one that breaks interoperability; the rest
(duplicates, event-failure status, heartbeat agreement, NL filter form, actors for
retirement/PATCH) are one-paragraph clarifications.

## RQ 2 — Implementation effort

Measured on this POC (one developer, existing knooppunt infrastructure):

| Part | Effort |
|---|---|
| Backport types + notification bundle build/parse (`lib/fhirsubscription`) | ~½ day, R4-library gap made this hand-written (see RQ 7) |
| Server engine (validation, handshake, event pipeline, heartbeat, $status/$events, retry) | ~1 day |
| Client engine (endpoint, continuity, idempotency, recovery, miss detection, bootstrap) | ~1 day |
| Vendor-facing internal API + wiring + OpenAPI | ~½ day |
| Permutation e2e suite (6 cases, two in-process nodes) | ~½ day |

The protocol machinery is the bulk; none of it is use-case-specific. A vendor building
this *without* a node does all of the above minus the internal API; a vendor building
*against* a node does roughly none of it (see RQ 3/4).

## RQ 3 / RQ 4 — What does the node abstract away; where does node help land?

The dividing line proposed in [09-knooppunt-vendor-split.md](09-knooppunt-vendor-split.md)
held up in implementation. The knooppunt absorbed the entire FHIR technical chapter and
five of six implementation obligations; the vendor-facing contract came out as exactly
the proposed internal API:

- `POST /subscription/events` (event feed), `POST /subscription/authorizations`
  (authorization basis), `POST /subscription/subscriptions` (out-of-band intent),
  `POST /subscription/intents` (in-band intent), `GET /subscription/inbox` +
  `POST /subscription/inbox/{id}/ack` (normalised consumption), `GET /subscription/log`.

**Effort ledger (the measurable claim):** the e2e permutation suite and the demo script
play the vendor on both sides. They read *zero* sections of the TTA spec — only the
internal API — with one deliberate exception (the demo's "what travelled" step, which
shows the wire on purpose). The knooppunt implementation needed essentially every
normative section. That delta is the abstraction value.

**Honest limits confirmed in code:**

- **[IMPL] Durability.** Event counters, the `$events` log, the client's processed-set
  and the inbox are in-memory. A restart breaks the continuity guarantees the node
  claims to own. Adopting this for real requires a persistence layer the knooppunt
  currently doesn't have at all. (Also: the HMAC alias key is random per start unless
  configured — aliases wouldn't survive a restart, violating id *stability*.)
- **[IMPL] The internal hop re-creates delivery one level down.** The inbox has ack
  semantics, but knooppunt→vendor webhook push (the alternative to polling) was not
  built; at-least-once on the internal hop is the vendor's polling discipline.
- **[IMPL] Heartbeat-miss recovery stops at `$status`.** After a missed heartbeat the
  client queries `$status`, logs the server's `events-since-subscription-start` and
  adopts the server's status, but does not yet compare that number with its highest
  processed event to trigger `$events` replay — only an *arriving* event triggers gap
  recovery today. Closing this is a small change (reuse `recoverGap`); left open as a
  known POC gap.
- **[IMPL] Topic evaluation fidelity.** Generic `queryCriteria`/`canFilterBy`
  evaluation was deliberately not built: the single agreed topic's semantics (Task,
  create/update, owner-URA filter) are hard-coded, ~40 lines. Every additional topic is
  code, not configuration, until someone builds a criteria evaluator — the vendor-cost
  axis the L1–L4 exploration (doc 04) predicts. A minimal ingest event
  (`{resourceType, id, interaction, ownerUra, useCaseCode}`) was enough for this topic;
  richer topics would force fatter ingest payloads or knooppunt read-back.

## RQ 5 — AuthN/AuthZ fit, mTLS

- **[IMPL] Inbound mTLS does not exist anywhere in the knooppunt** (neither the Go
  servers nor the checked-in nginx PEP terminate client-certificate TLS). Spec
  precondition 4 ("mutual TLS MUST be applied") is therefore not satisfiable with
  today's knooppunt — the POC runs plain HTTP and records this as the finding rather
  than solving it. Consequence chain: without mTLS there is *no sender identity at all*
  on a notification (the bundle carries none — see RQ 1.3), so the receiving side
  cannot even authenticate who is notifying it.
- **Hook placement demonstrated:** authorization verification runs before activation
  and again immediately before every send; consent verification before every send
  (`component/subscription/authz.go` — stubs that log and consult the vendor-registered
  bases). The vendor registers the basis at order creation via
  `POST /subscription/authorizations`, matching the COW "nothing on the wire" model —
  the knooppunt PDP is the natural enforcement point, and the call sites are now
  identified in code. The dedicated endpoint is a POC device; the intended production
  shape reuses the existing GF Authorization path (local consent record in the PIP,
  evaluated by the PDP, the same record that governs the follow-up pull) — both variants
  are drawn in [ARCHITECTURE.md](ARCHITECTURE.md#authorization-basis-two-ways-to-supply-it).
- **[SPEC]** The spec requires verification "immediately before sending each
  notification" including heartbeats-by-implication ("each notification"); for
  heartbeats there is no disclosure, so per-heartbeat consent checks look like
  obligation inflation. Worth clarifying scope to event notifications.

## RQ 6 — Differing optional features (permutation matrix)

All six planned permutations pass as e2e tests (`test/e2e/subscription`):

| # | Server | Client | Observed |
|---|--------|--------|----------|
| 1 | in-band, heartbeat, $events, id-only | heartbeat check on | Full happy path; suppressed event recovered via `$events`; events arrive in order 1,2,3; duplicate redelivery acknowledged and ignored |
| 2 | out-of-band, no heartbeat, **no $events** | — | Client bootstraps from the unknown subscription (fetch + agreement check); gap degrades to `$status` + a `gap` inbox entry telling the vendor to resync |
| 3 | heartbeats on, empty mode | heartbeat check **off** | Heartbeats acknowledged (200) and otherwise ignored — harmless noise; empty-mode events carry no focus anywhere |
| 4 | heartbeat **not supported** | client requested one | Server strips it from the created resource; client's miss-timer fires and `$status` reports all-is-well, repeatedly — protocol busywork, no failure (see RQ 1.8) |
| 5 | **no $events** | client asks anyway | 404 OperationOutcome handled gracefully; degrades as permutation 2 |
| 6 | out-of-band, mode **outside client's agreement** | — | Client fetches the Subscription, refuses the handshake (422); server transitions to `error`; no subscription tracked client-side |

**The normalisation claim holds:** in every permutation the vendor-facing inbox entries
have the identical shape (asserted by `assertUniformInboxEvents`) — subscription,
event number, focus-if-id-only, in order, deduplicated, with unrecoverable ranges as
explicit `gap` entries. Feature permutations are invisible to the vendor.

## RQ 7 — FHIR version compatibility

- **[IMPL] The Go R4 model library is a real obstacle.** `zorgbijjou/golang-fhir-models`
  cannot express primitive-element extensions (`_criteria`, `_payload`), and has no
  SubscriptionTopic or SubscriptionStatus. All backport wire shapes are hand-written in
  `lib/fhirsubscription` (~350 lines + tests). Any Go implementer faces the same; the
  JavaScript/Java ecosystems are better off. The R4B/R5 gap is felt exactly where the
  backport IG says it is: in the primitive extensions.
- The notification `Parameters` and `Bundle` themselves are plain R4 — the stock
  library types worked unchanged there.
- STU3 relation: today's eOverdracht/TA NP notification is an STU3 Task push; nothing
  in it maps 1:1 onto the new layer (see RQ 10 / doc 08's tables). The migration is a
  re-implementation, not a conversion.

## RQ 8 — Do current application identifiers meet the id-only requirements?

**No.** HAPI (as deployed here) uses sequential ids by default — ordered, low-entropy,
guessable: they fail opacity and unpredictability outright. Even the UUID strategy the
compose file enables (`server_id_strategy: UUID`) is only compliant if it's v4.
The POC demonstrates the consequence chain in full:

1. Focus references are exposed under an HMAC alias (keyed transformation, the spec's
   own compliant example): stable, opaque, 24-char base32 — `component/subscription/alias.go`.
2. **Aliasing pulls the knooppunt into the data path**: the follow-up pull must come
   back through something that can de-alias — implemented as
   `GET /fhir/Task/{alias}` on the public mux, which resolves the alias, reads the
   tenant store, and rewrites the resource id so the internal id doesn't leak in the
   response body.
3. This is a deployment-model decision, not a detail: "is the knooppunt in the pull
   path or not?" For vendors whose ids can't comply and who won't accept the node in
   the data path, the honest fallback is **empty mode**, with the spec-documented cost
   that a search exposes more than the notification pointed at.
4. Remaining leak surface even with aliases: references *inside* pulled resources
   still carry internal ids unless the node rewrites those too — full id-translation is
   a much bigger claim (not built; finding).

## RQ 9 — Endpoint resolution via GF Adressing

- Implemented: partner URA → Organization (by URA identifier) in the mCSD Query
  Directory → its `Organization.endpoint` references → first active `Endpoint` whose
  `payloadType` matches — `component/subscription/resolve.go`, with per-partner
  configuration as fallback (used by the e2e tests and compose demo).
- **[SPEC/GF] No payload-type vocabulary exists for this.** The POC had to invent
  `tta-notification` (rest-hook delivery endpoint) and `tta-subscription` (where
  `POST /Subscription`, `$status`, `$events` live) in the knooppunt's own code system.
  GF Adressing needs to standardise these codes — *two* of them, because the two
  surfaces are distinct and only the server role has the FHIR base.
- Note the interlock with RQ 1.3: out-of-band bootstrap needs the *reverse* resolution
  (notification → sender), which mCSD does not give you; absolute references do.

## RQ 10 — Differences vs current eOverdracht / Nuts TA NP

Answered in detail in [08-comparison-ta-np.md](08-comparison-ta-np.md) (structural
comparison) and the mapping tables of [06-clinical-workflow.md](06-clinical-workflow.md).
What this POC adds from implementation experience:

- The old pattern's missing reliability layer is real work: continuity, recovery,
  idempotency and retry were roughly half the engine code (see RQ 2). That is what TA
  NP v1.0.1 silently didn't have.
- The new split (notification says only *that*, workflow/data layers say *what/how*)
  showed up concretely: the notification pipeline needed zero knowledge of eOverdracht
  semantics beyond the topic's owner filter; everything use-case-specific stayed on the
  Task/vendor side of the internal API.
- The intent problem flagged by the harmonisation group is visible in the POC too: an
  id-only focus tells the receiver nothing about *why* until it pulls; the topic
  canonical is the only intent carrier on the wire.

## POC limitations (deliberate, recorded)

- In-memory everything (see RQ 3/4 durability finding); single hard-coded topic;
  `full-resource` excluded by assignment; no PATCH of payload mode; no subscription
  search parameters; auth/consent hooks are logging stubs; no inbound mTLS; the demo
  vendor is the e2e suite + `docs/test-scripts/tta-notifications.http`, not a demo-ehr
  UI integration.

## Draft conclusion for the Werkgroep Technische Convergentie

TTA Notifications v0.6 is implementable as specified: both roles, all transactions and
all six optional-feature permutations run end-to-end in this POC in roughly the planned
three days, and the spec's examples were usable verbatim as test vectors. The findings
that matter for the 1.6 consultation split in two. Editorially, v0.6 should stop
paraphrasing the Subscriptions Backport IG it mandates and reference it: the
paraphrases of `$events`, `$status` and the SubscriptionStatus fields are where this
POC first diverged from the standard. Substantively, the v0.6-level gaps are the
unresolvable relative subscription references that make the out-of-band mode check
impossible, the heartbeat-agreement mechanics, and a handful of one-paragraph
clarifications (duplicates, event-delivery failure, the NL filter form, actors for
retirement and PATCH) — plus two ecosystem gaps the spec presumes solved: an addressing vocabulary for notification/subscription endpoints
(GF Adressing) and mTLS-based sender identity (absent from today's knooppunt). The
id-only identifier requirements are the sharpest adoption constraint: typical EHR ids
fail them, and the alias escape hatch drags the node into the data path — a deployment
decision Twiin and the applications group should take explicitly. A node implementation
can absorb essentially the whole normative transport layer and reduce vendor work to a
handful of plain REST touchpoints, provided the node gets durable state.
