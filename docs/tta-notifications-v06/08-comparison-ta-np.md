# Comparison: TA Notified Pull v1.0.1 (2024) vs TTA Notifications v0.6 + COW workflow

> Sources: https://www.twiin.nl/tanp (TA NP v1.0.1 overview),
> https://ontwikkelsupplement.twiin.nl/actueel/10-3-1-tta-fhir-harmonized-notified-pull-v2-x
> (Harmonized NP v2.x umbrella), plus the mapping tables in [06-clinical-workflow.md](06-clinical-workflow.md).
> This answers POC research question 10 (differences with current eOverdracht notification and the
> Nuts TA NP variant).

## The old TA NP v1.0.1 in one paragraph

Developed in co-creation Apr 2022 – May 2023; v1.0.0 March 2024, v1.0.1 patch June 2025; part of
Twiin Afsprakenstelsel release 1.2 §10.2.3. The notification **is a FHIR STU3 Task** POSTed directly
to the receiver's Task endpoint. That single Task fuses three concerns:

1. **Notification** — `Task.code = pull-notification` ("something is available")
2. **Workflow** — reference to the sender's workflow Task; status `requested`/`cancelled`
3. **Retrieval + authorization** — `Task.input` slices: `query-available-resources` (FHIR queries to
   run), `read-available-resource` (specific instances), `get-workflow-task`, and an opaque
   `authorization-base` token the receiver plays back on access-token requests (OAuth 2.0)

There is no standing channel: every exchange is an ad-hoc, sender-initiated push of this Task stub.
The Nuts eOverdracht variant solved the same problem differently (empty POST to the Task endpoint,
STU3 Nictiz Task profile, BSN kept off the Task, Nuts Authorization Credential) — two incompatible
ecosystems, which is why the harmonisation working group exists.

## The structural gaps in the old pattern

1. **No reliability semantics.** A bare POST: no sequence numbers, no continuity detection, no
   heartbeat, no status query, no replay, no defined retry/idempotency. A lost notification is
   silently lost for both sides.
2. **Bespoke and on a dead FHIR version.** STU3 with custom Task codes and input slices — no
   international vendor implements it out of the box. Because notification and workflow are fused,
   only "sender pushes a work order" is expressible; receiver-initiated interest (follow a patient's
   BgZ, lab results) cannot be modelled.
3. **Sensitive material on the wire.** The authorization-base token, the FHIR queries (revealing what
   data exists and why), and potentially the BSN travel inside the notification. The privacy
   properties of the notification itself were never formalized (v0.99 added "stricter identifiers"
   but nothing like the v0.6 SHALL-requirements).

## What the new stack does instead

The harmonisation splits the fused Task into layers; TTA Notifications v0.6 is only the bottom one.

| Aspect | TA NP v1.0.1 (2024) | New stack (TTA Notifications v0.6 + COW profile) |
|--------|---------------------|---------------------------------------------------|
| FHIR version | STU3 | R4/R4B (Subscriptions R5 Backport IG); forward-portable to R5/R6 |
| Transport | Ad-hoc Task POST per exchange | Standing Subscription on a pre-agreed SubscriptionTopic; rest-hook `history` Bundles |
| Standards basis | Bespoke Dutch pattern | International HL7 FHIR Subscription framework |
| Reliability | None | Handshake, monotonic event-numbers + continuity check, optional heartbeat, `$status`/`$events` recovery, exponential backoff, mandatory idempotency |
| Workflow | Fused into the notification Task | Separate COW **Coordination Task that stays at the Placer** (receiver pulls it), persisted Request resource, full Task state machine, CancellationRequest Task, multi-fulfiller solicitation, mandatory business identifier |
| Payload | Task with queries + token + metadata | Formal modes `empty`/`id-only`/`full-resource`; SHALL-level id-only identifier requirements (stable, opaque, unpredictable, consumer-opaque) |
| Retrieval instructions | `query-available-resources` / `read-available-resource` in the notification | Dataset named by `Request.code` (+ optional canonical PlanDefinition); per-order additions as `Task.input` supplemental-query/-resource — pulled, not pushed |
| Authorization | Opaque `authorization-base` token in `Task.input` | Nothing on the wire; basis recorded at the Placer, verified before activation and before **every** send (trial `notification-authorization-hint` exists but is not in the profile) |
| BSN | On Task or OAuth patient claim | `Task.for.identifier`, populated per phase; never in notifications, no OAuth claim |
| Direction | Sender-initiated only | Source-initiated (out-of-band) **and** receiver-initiated (in-band) |
| Cancellation | Flat status change / conditional cancellation notification | Direct cancel before `in-progress`; negotiated CancellationRequest Task after |
| Delta updates | `Task.groupIdentifier` correlation | Same-Task update fires a new notification |

Conceptually: the old TA NP told you *what to pull and gave you the key*; the new notification layer
only tells you *that something happened* — meaning, retrieval, and permission move to the workflow
layer and the sender-side authorization record.

## Migration impact (from the COW profile mapping tables)

For TA NP implementations: STU3→R4; a new persisted Request resource (TA NP has none); mandatory
business identifier on Task; `Task.businessStatus`; post-`in-progress` CancellationRequest workflow;
notifications now also fire on the **Fulfiller's own** status writes (TA NP never notified status changes).

For Nuts eOverdracht implementations: STU3→R4; explicit Request resources (eOverdracht embeds
everything in `Task.input`); Coordination Task at the Placer + Subscription notifications instead of
Task-push; formalized BSN timing; CancellationRequest workflow.

## Gaps that remain in the new stack

- v0.6 covers only the notification layer; the COW workflow profile is a **draft** and the umbrella
  Harmonized NP v2.x is unfinished, with an open escalation on I&A&A (Verifiable Credentials vs GIS-A
  alignment; secure-authentication consensus expected Q3 2026).
- **Intent problem**: the old Task told you *why* you were notified (code + queries); a bare id-only
  Task reference does not — flagged as an open issue by the harmonisation group.
- Topic granularity (L1–L4) undecided; authorization declared in-scope but unelaborated.
- Underspecified spots found while planning the POC: `$events` parameters, the out-of-band creation
  transaction, heartbeat-period agreement mechanics (candidates for the FINDINGS document).
