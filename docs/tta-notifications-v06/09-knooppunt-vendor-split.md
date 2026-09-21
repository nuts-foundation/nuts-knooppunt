# Proposal: knooppunt vs vendor responsibility split for TTA Notifications v0.6

> Answers POC research questions 3 ("what does the node manage vs the application?") and 4 ("which
> aspects get concrete help from a node implementation?"). Premise: the knooppunt exists to make it
> cheap for XIS/EHR vendors to support this spec — but vendors will always have to do *something*.
> This document proposes where the line goes, what the irreducible vendor minimum is, and how the POC
> can produce evidence for whether the knooppunt genuinely simplifies adoption.

## The dividing principle

Everything in the spec splits cleanly along one axis:

- **Application-agnostic protocol machinery** — behaviour that is identical for every use case and
  contains zero clinical semantics (subscription lifecycle, handshake, event numbering, heartbeats,
  recovery operations, retries, payload-mode enforcement, mTLS, transport logging). The knooppunt can
  own 100% of this. Notably, this is almost exactly the spec's entire "FHIR Technical Specification"
  chapter plus five of the six Implementation Obligations.
- **Application semantics** — knowing that an event happened, what it means, who may learn of it, and
  what to do about it. This cannot leave the vendor/care organisation, no matter how good the
  knooppunt is.

The vendor-facing surface is the knooppunt's **internal mux** (existing pattern); the network-facing
surface is the **public mux** behind the PEP. The proposal is that a vendor never touches the
backport IG, the wire format, or the remote party's optional-feature choices at all.

## Responsibility matrix

### Subscription Server role (sending organisation)

| Spec obligation | Owner | Notes |
|---|---|---|
| Public `POST /Subscription` (in-band), validation against pre-agreed topics/modes, 201/400/422 + OperationOutcome | **Knooppunt** | Agreed topic list + allowed modes are knooppunt configuration; vendor never sees the endpoint |
| Out-of-band Subscription creation, partner notification-endpoint resolution (mCSD/GF Adressing) | **Knooppunt** | Trigger is application intent (e.g. Task names a partner as owner) — see "event feed" below |
| Handshake send + exponential-backoff retry, `requested→active/error` transitions | **Knooppunt** | Pure protocol |
| Concurrency-safe, monotonic event-number allocation | **Knooppunt** | ⚠ requires durable state (see limits) |
| Notification Bundle construction per payload mode (`empty`/`id-only`); payload-mode enforcement per registered mode | **Knooppunt** | Vendor never builds a Bundle |
| Delivery + exponential-backoff retry | **Knooppunt** | |
| Heartbeat timers and bundles | **Knooppunt** | Pure timer, no semantics |
| `$status` | **Knooppunt** | |
| `$events` replay + event log + retention | **Knooppunt** | Retention period is use-case configuration |
| Authorization verification before activation and before **each** send | **Knooppunt** (PDP) — **vendor supplies the basis** | The check runs in knooppunt's PDP/PEP; but *who may receive what* originates at the vendor when the order/relationship is created (the Placer access record). Vendor registers the basis via an internal API; knooppunt enforces it |
| Consent verification before each send | **Knooppunt** (Mitz component) | Consent capture itself remains a care-org/vendor process |
| mTLS on all endpoints | **Knooppunt/PEP infra** | Vendor is entirely shielded from network-level security (currently a knooppunt gap — inbound mTLS doesn't exist yet) |
| Transport logging (sent/received subscriptions + notifications) | **Knooppunt** | Vendor keeps only its own care-process logging |
| **Event detection** — knowing a Task/resource changed | **Shared — the first irreducible vendor obligation** | Knooppunt can match/filter events against topics, but the events originate in the vendor's system. Vendor must either (a) call a simple internal ingest API on relevant changes, (b) expose a pollable FHIR R4 endpoint with `_history`, or (c) host the workflow data in a knooppunt-adjacent FHIR store (today's demo model). Something must flow from vendor to knooppunt |
| **Id-only identifier compliance** (stable, opaque, unpredictable) | **Shared — the second irreducible obligation** | The ids are the vendor's ids. Either the vendor's ids already comply (rare — most EHRs use sequential ids), or the knooppunt aliases them (HMAC) — but then the knooppunt must also sit in the **pull path** to de-alias reads, which pulls it into data serving, a bigger architectural claim than notifications alone |
| Serving the actual clinical data for the follow-up pull | **Vendor** (behind knooppunt's PEP for authz) | Notified Pull's whole point: the sender remains the source of truth |

### Subscription Client role (receiving organisation)

| Spec obligation | Owner | Notes |
|---|---|---|
| Public notification endpoint (mTLS), Bundle parsing, handshake acceptance | **Knooppunt** | |
| Event-number continuity check, gap recovery via `$events`/`$status` | **Knooppunt** | The single strongest simplification: fiddly, stateful, zero application semantics |
| Heartbeat-miss detection + `$status` follow-up | **Knooppunt** | |
| Transport-level idempotency (dedupe on subscription + event-number) | **Knooppunt** | Vendor's application-level idempotency burden shrinks to handling at-least-once delivery on one trusted internal hop |
| **Permutation normalisation** | **Knooppunt** | Whatever the remote server chose (in-band/out-of-band, heartbeat or not, `$events` or not, empty or id-only), the knooppunt presents one uniform internal event: "subscription X, event N, focus Y (if any), in order, deduplicated". This directly neutralises research question 6 for vendors |
| Out-of-band bootstrap (fetch unknown Subscription, check mode against agreement, reject handshake if outside) | **Knooppunt** | Agreements are knooppunt configuration |
| **Notification consumption** | **Shared — the third irreducible obligation** | Knooppunt must hand events to the vendor: vendor registers a webhook callback and/or polls an inbox API with acknowledgement. Something must flow from knooppunt to vendor |
| Subscription intent (which topic, which patient/partner, when to end) | **Vendor decides, knooppunt executes** | One internal API call replaces the whole in-band creation transaction |
| The follow-up pull: whether, when, what to fetch; ingesting the data | **Vendor** | Data-minimisation decision is application logic by design |

### Irreducible vendor minimum (what a vendor must build regardless)

1. **Event feed** (server role): report relevant changes to the knooppunt, via ingest call or
   pollable FHIR history.
2. **Authorization-basis registration** (server role): tell the knooppunt/PDP who may be notified of
   and pull what, when the care relationship/order is created.
3. **Compliant or aliasable resource ids + a pullable FHIR endpoint** (server role).
4. **Notification consumption** (client role): webhook or inbox, plus acting on events.
5. **Subscription intent management** (client role): create/end subscriptions via internal API.
6. **All application semantics**: topic/use-case meaning, the pull decision, ingestion, the COW Task
   lifecycle (status updates are the vendor's clinical workflow), consent capture.

That is a short list of plain internal REST interactions — no FHIR backport knowledge, no wire
format, no reliability machinery, no remote-feature permutations.

## Proposed vendor-facing contract (internal mux)

To make the split concrete, the POC should treat this as a first-class deliverable, not demo plumbing:

```
# Server role
POST /subscription/events           # ingest: {resourceType, id, interaction, use-case hints}
POST /subscription/subscriptions    # out-of-band: set up a subscription towards a partner
POST /subscription/authorizations   # register the basis: {receiver URA, topic, patient, validity}

# Client role
POST /subscription/intents          # in-band: subscribe at a partner {partner URA, topic, mode, heartbeat}
GET  /subscription/inbox?since=...  # pull-based delivery of normalised events
POST /subscription/inbox/{id}/ack   # acknowledgement (at-least-once semantics)
# alternative: configured webhook per tenant, same normalised event payload

# Both
GET  /subscription/subscriptions    # state inspection
GET  /subscription/log              # transport log (demo/audit)
```

## Where the simplification claim is weak (honest limits)

1. **Durability.** Event numbers, the `$events` log, and the client's processed-set must survive
   restarts, or the knooppunt *undermines* the continuity guarantees it claims to own. The knooppunt
   currently has no persistence layer at all — adopting this proposal for real means adding one
   (HAPI tenant or embedded DB). The POC's in-memory store is acceptable only because it's a POC;
   this must be an explicit finding.
2. **The internal hop re-creates the problem one level down.** Knooppunt→vendor delivery needs its
   own retry/ack. It's a far easier problem (trusted LAN hop, one counterpart, vendor-chosen
   mechanism) — but "the knooppunt handles reliability" is only true up to its internal edge.
3. **Id-only pulls the knooppunt into the data path.** If vendors' ids don't comply and the knooppunt
   aliases them, it must proxy (or at least rewrite) FHIR reads. If that's unacceptable, the honest
   answer for those vendors is **`empty` mode**, with the spec-documented cost that the client's
   search may expose more than the notification intended. This trade-off is itself a key finding for
   Twiin (research question 8).
4. **Topic filter fidelity depends on the event feed.** Generic `queryCriteria` evaluation requires
   the knooppunt to see enough of the resource. A minimal ingest API forces either fat ingest
   payloads, a knooppunt read-back of the resource at event time, or coarse topics. This is exactly
   the vendor-cost dimension the L1–L4 topic-granularity exploration (doc 04) predicts.
5. **Agreements are still organisational.** Pre-agreed topics, payload modes, and heartbeat terms are
   configuration the care organisation/vendor must supply; the knooppunt enforces but cannot invent them.

## How the POC should test the claim

Make "does the knooppunt simplify this?" measurable instead of rhetorical:

1. Build the vendor contract above; make **demo-ehr play the vendor** and implement only the
   irreducible minimum against it.
2. Record two effort ledgers: (a) which spec sections the knooppunt implementation needed
   (expected: nearly all of the FHIR technical chapter), and (b) which spec sections the demo-ehr
   integration needed to *read at all* (target: none — only the internal API doc). The delta is the
   abstraction value, reported under research questions 3/4.
3. Run the permutation matrix (plan §permutations) and verify the vendor-side inbox payload is
   **identical across all permutations** — that's the proof of the normalisation claim for research
   question 6.
4. Report the weak spots above (durability, id-only data-path coupling, event-feed fidelity) as
   findings, with the `empty`-mode fallback as the pragmatic vendor path where ids don't comply.

**Expected verdict** (to be validated, not assumed): yes — the knooppunt can absorb roughly the
entire normative transport spec and reduce vendor work to five plain REST touchpoints plus
application semantics; the genuine caveats are durable state in the knooppunt and the id-only/data
path coupling, which should be put to Twiin and the applications group as the deployment-model
question ("is the knooppunt in the pull path or not?").
