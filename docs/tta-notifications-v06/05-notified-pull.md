# GF IG — Notified Pull (draft, branch build)

> Source: https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/notified-pull.html
> English extraction. Page status: draft. Package fhir.nl.gf#0.3.0, FHIR 4.0.1.

## Definition

"Notified Pull is a data-availability exchange pattern: a Sender publishes a notification saying that
one or more resources are available, and the Receiver pulls those resources on its own terms."
Fire-and-forget: the Sender does not require confirmation that the Receiver accepted, rejected, or
acted on the data.

This page specifies: (1) semantics of `notification-event.focus` for data-availability occurrences,
(2) patient (subject) identification, (3) use of `notification-authorization-hint` to scope pulls.

**Fits when**: sender makes data accessible to a known receiver without workflow coordination;
receiver controls whether/when/how to retrieve (data minimization); server-side audit is the
verification mechanism; dataset definition needs no negotiation.

**Does not fit when**: acceptance/rejection responses are needed; receiver-side processing state must
be tracked; cancellation negotiation is required after retrieval begins; multiple candidate receivers
must be solicited. (Those need the COW workflow profile.)

Trade-offs: no protocol-level retry (sender learns "never pulled" only from access logs); no
application-level confirmation; withdrawal-only cancellation (receiver discovers it via HTTP codes).

## Single-resource notifications

One notification references one clinical resource via `notification-event.focus`
(e.g. `Observation/abc123`, `DocumentReference/def456`, `MedicationDispense/ghi789`). Receiver does a
standard FHIR read at the sender; patient identity comes from the resource's `.subject`.

## Multi-resource notifications (container pattern)

`notification-event.focus` references a **container resource**:

| Container | Best for |
|-----------|----------|
| `List` | Generic collection — **recommended default**; `List.subject` = patient, `List.entry.item` = members, `List.code` labels the collection |
| `Composition` | Document-style aggregates with narrative structure (e.g. transfer-of-care summary) |
| `DocumentReference` | Document/binary payload (PDF/A) |

Example:

```
List
  status: current
  mode: snapshot
  code: <use-case-bound code, e.g. "BgZ snapshot">
  subject: Reference(Patient with identifier = BSN)
  date: 2026-05-11T10:30:00+02:00
  entry:
    - item: Reference(Observation/...)
    - item: Reference(Condition/...)
    - item: Reference(MedicationStatement/...)
```

The container lives at the sender's FHIR endpoint; receiver may pull the container alone, entries on
demand, or everything at once via `_include`. Multiple focus references without a container remain
valid for small ad-hoc groups.

## Updates to previously notified sets

Sender SHOULD send a new notification on the same Subscription with incremented `event-number` and the
**same focus**. Optimizations: compare `meta.lastUpdated` / use `_lastUpdated=ge<timestamp>` searches;
use `_history` on the container to detect membership changes (removals don't update remaining members).

## Subject identification

BSN is **not carried on the notification wire**. Two mechanisms:
1. On the focus resource at the sender (`Resource.subject.identifier` / `List.subject.identifier`, …);
   receiver learns BSN upon reading the focus.
2. Encoded inside the opaque `notification-authorization-hint` token using a sender-only key; the
   sender's authorization server decodes it and constrains the pull.

## Authorization base (`notification-authorization-hint`)

Opaque token from sender to receiver; receiver plays it back in access-token requests; sender's
authorization server decodes and authorizes the pull. Practical pattern: encode BSN, resource scope,
and validity window with a sender-only key. Benefits: no clear-text BSN, sender keeps scope control,
audit ties token usage to the originating notification. Costs: receiver cannot route internally on BSN
before pulling; sender must manage encode/decode and keys; semantics are sender-defined. Wire format
is out of scope (partners' access-control agreement); inclusion is optional.

## Data unavailability (withdrawal)

Sender removes the resource or revokes access; the next pull's HTTP response **is** the signal:
- **410 Gone** — existed, deliberately removed (confirms withdrawal)
- **404 Not Found** — does not disclose whether it ever existed
- **403 Forbidden** — authorization rejected; implies nothing about existence

No `cancel` operation, no follow-up withdrawal notification. Acknowledged cancellation requires COW.

## Sender intent / receiver-side state tracking (non-normative, under discussion)

1. **Server-side audit only** — derive "was it pulled" from access logs; no receiver obligation.
2. **Receiver-updated status on the focus resource** — e.g. `processing-status` extension on the List
   (`pending` → `viewed`/`processed`); sender validates updates.
3. **Separate Task carrying acknowledgement** — sender creates Task (`focus` = data,
   `status = requested`); receiver advances to `received`/`completed`. "COW Lite"; risks conceptual
   drift toward Coordination Tasks.

In all approaches the sender's tracking intent should be discoverable from the SubscriptionTopic.

## Relationship to other layers

- **Transport**: Notification page (Subscription, SubscriptionTopic, Bundle, notification-event) used unchanged.
- **Workflow**: COW profile; there focus points at a Coordination Task instead of clinical data.
- **Endpoint discovery**: notification endpoint resolved per receiver organisation via the addressing
  function (mCSD-based Care Services Directory).
- **Internal routing**: TA Routing using `HealthcareService`, `Location`, optionally
  `ActivityDefinition` published in the mCSD directory.
- **Access control**: mTLS, OAuth, JWT assertions, hint token format — out of scope; partners' agreement.

## Open questions

1. Confirm `List` as default container (Composition/DocumentReference as alternatives)?
2. Should the spec define an explicit internal routing label (department/mailbox/specialism)?
3. Should the spec recommend minimum authorization-base token structure or leave it fully opaque?
4. Multiple focuses/containers per notification, or restrict to one focus per occurrence?
