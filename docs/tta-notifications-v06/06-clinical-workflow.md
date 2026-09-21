# GF IG — Clinical Workflow (COW profile, draft, branch build)

> Source: https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/clinical-workflow.html
> English extraction. v0.3.0 CI build, draft. Profiles HL7 Clinical Order Workflows (COW) IG
> v1.0.0-ballot for the Dutch Notified Pull pattern; harmonizes Twiin TA NP v1.0.1 and Nuts
> eOverdracht leveranciersspecificatie. **This is where the POC's "SubscriptionTopic on Task for
> eOverdracht alignment" comes from.**

## Architecture — four layers

1. **Request layer**: FHIR Request resource (ServiceRequest, DeviceRequest, MedicationRequest,
   NutritionOrder, SupplyRequest, VisionPrescription) at the Placer; clinical payload lives here.
2. **Workflow layer**: COW **Coordination Task** at the Placer, `Task.focus` → Request; tracks lifecycle.
3. **Notification layer**: **TTA Notifications v0.6** events inform the Fulfiller when the
   Coordination Task changes. Notifications carry no clinical content.
4. **Data layer**: Fulfiller pulls from the Placer's FHIR endpoint using queries derived from the Request.

COW Pattern 2: Task stays at the Placer; R4 Subscription Backport events notify the Fulfiller.

Terminology: Placer = sending organisation/bronhouder; Fulfiller = receiving organisation;
Coordination Task = workflow Task; Request = opdracht/verwijzing; Subscription = abonnement.

## Notification integration

- A **broad, long-lived Subscription per partner** against an NL SubscriptionTopic narrowed on `Task`
- Payload mode **`id-only`** (carries only the Task reference)
- `backport-subscription-notification` Bundle format
- Topic MUST match both the use-case code (e.g. referral) and `abort` for cancellations
- **No FHIR queries, dataset content, workflow status, or authorization tokens in notifications**
- Task logical id must be "stable, opaque and unpredictable" per the TTA identifier requirements
- Each Task change at the Placer generates a notification; the Fulfiller also receives notifications
  for its own writes and must handle them idempotently

## Happy path

1. Pre-coordination: Fulfiller registers a broad long-lived Subscription at the Placer
2. Placer creates Request + Coordination Task (`status = requested`)
3. Notification Bundle POSTed to Fulfiller
4. Fulfiller GETs the Coordination Task (with includes for `Task.focus`, `Task.for`)
5. Fulfiller PUTs Task `status = in-progress`
6. Fulfiller executes FHIR searches per dataset spec and Task inputs
7. Fulfiller PUTs Task `status = completed`

## NlCowCoordinationTask (key elements)

| Element | Card. | Rule |
|---------|-------|------|
| `Task.identifier` | 1..* | Business identifier issued by Placer |
| `Task.status` | 1..1 | Full FHIR Task state machine |
| `Task.intent` | 1..1 | Fixed `order` |
| `Task.code` | 1..1 | **Two codings**: `task-code#fulfill` + use-case code (SNOMED/local) — required because R4B SubscriptionTopics cannot follow `Task.focus` references |
| `Task.focus` | 1..1 | The Request resource |
| `Task.for` | 1..1 | Patient reference at Placer |
| `Task.for.identifier` | 0..1 | BSN (`http://fhir.nl/fhir/NamingSystem/bsn`); MUST be present before `in-progress` (invariant `nl-cow-bsn-before-in-progress`); populated at latest on selection/acceptance |
| `Task.requester` | 1..1 | Placer organisation + practitioner |
| `Task.owner` | 1..1 | Fulfiller organisation |
| `Task.businessStatus` | 0..1 | Domain progress; profile defines `selected` for multi-Fulfiller solicitation |
| `Task.restriction.period` | 0..1 | Data availability window |
| `Task.input` supplemental-resource | 0..* | Reference to a specific Placer resource instance (maps to TA NP `read-available-resource`) |
| `Task.input` supplemental-query | 0..* | Ad-hoc FHIR search string extending the dataset (maps to TA NP `query-available-resources`) |
| `Task.output` | 0..* | Fulfiller outcomes (hosted at Fulfiller) |

## State machine

```
requested → received (optional ack) → accepted → in-progress → completed
requested/received → rejected (Fulfiller decline)
in-progress → failed (Fulfiller abandonment)
any pre-in-progress → cancelled (Placer, direct)
```

- Boundary: at `in-progress` the Placer loses unilateral cancellation; afterwards a separate
  **CancellationRequest Task** (`Task.code = abort`, `focus` = Coordination Task, `status = requested`)
  is required; Fulfiller answers `accepted` (→ Placer sets Coordination Task `cancelled`) or `rejected`.
- Shortcut `requested → in-progress` only when BSN is already populated.

## Multi-Fulfiller solicitation

One ServiceRequest; N Coordination Tasks (same `Task.focus`, different `Task.owner`); each Task fires
its own notification. Selection: `businessStatus = selected` on the winner; `status = cancelled` +
`statusReason = "not selected"` on losers. Access-control MUST limit pullable data during solicitation
(minimal/de-identified) vs after selection. Correlation key is `Task.focus`.

## Data retrieval

- `Request.code` names the dataset (BgZ, eOverdracht), bound to a Dutch value set.
- Optional `Request.instantiatesCanonical` → versioned **PlanDefinition** enumerating the dataset's
  FHIR queries (e.g. `http://nictiz.nl/fhir/PlanDefinition/bgz|1.2.0`); published once, cached by Fulfiller.
- Per-order additions go on **Task.input** (not the Request) because Task updates notify and each
  candidate has its own Task. Changes to the Request itself do NOT trigger notifications.
- **"The Task is the only resource a Fulfiller has to watch."** Fulfillers diff `Task.input` against
  cached copies; Task history not required.
- On completion, Placer either sets `Request.status = completed` or keeps it active and creates a new
  Coordination Task.

## Authorization basis

Many NP use cases rely on presumed consent under WGBO (the referral itself), not explicit consent.
The Placer records the basis locally when creating the order (which Fulfiller, which resources, which
purpose, until when); this feeds the GF Authorization policy (a local consent record on the resource,
`resource.consents` with a scope). Withdrawal = removing the record (Task cancelled,
`restriction.period` expired). **Nothing on the wire** — unlike TA NP v1.0.1's `authorization-base`
token. The unpublished Backport IG 1.2.0-ballot has a trial `notification-authorization-hint`
extension if a separate authorization server is ever needed.

## Deviations from COW

1. CancellationRequest Task `focus` narrowed to the Coordination Task (Placer-initiated only).
2. Two-coding requirement on `Task.code`.

## Mapping: Twiin TA NP v1.0.1 → this profile

| TA NP v1.0.1 | This profile |
|--------------|--------------|
| Notification Task (STU3, POSTed) | Notification Bundle |
| `Task.code = pull-notification` | SubscriptionTopic canonical URL |
| Task.status requested/cancelled | Coordination Task status / CancellationRequest Task |
| Workflow Task | Coordination Task (COW-profiled) |
| `Task.input:authorization-base` | Basis recorded at Placer |
| `Task.input:query-available-resources` | Task.input supplemental-query + canonical dataset |
| `Task.input:read-available-resource` | Task.input supplemental-resource |
| `Task.input:get-workflow-task` | Obsolete |
| BSN on Task or OAuth patient claim | `Task.for.identifier` per phase; no OAuth claim |
| `Task.groupIdentifier` (delta correlation) | Not used; delta = same Task update |

Impact on TA NP implementations: STU3→R4 migration; new persisted Request resource; mandatory business
identifier; `businessStatus`; post-in-progress CancellationRequest Task; notifications on Fulfiller writes.

## Mapping: Nuts eOverdracht → this profile

| eOverdracht | This profile |
|-------------|--------------|
| Empty POST to Task endpoint | Notification referencing the Coordination Task |
| Task (STU3, Nictiz profile) | Coordination Task + Request (COW-profiled, R4) |
| `Task.input:nursingHandoff` (doc ref) | Dataset from `Request.code` + supplemental-resource |
| One Task per candidate | One Coordination Task per candidate (correlated on `Task.focus`) |
| BSN kept off the Task | `Task.for` references Patient; identifier absent until selection |
| Task.status gates access | Task status + Placer access record |
| Nuts Authorization Credential | Basis recorded at Placer |

Impact on eOverdracht implementations: STU3→R4; explicit Request resources; Coordination Task at
Placer + Subscription notifications; formalized BSN timing; CancellationRequest workflow.

## Example artifacts referenced

- Referral (single candidate): `ServiceRequest-np-referral-servicerequest`,
  `Task-np-referral-coordination-task`, `PlanDefinition-np-bgz-plandefinition`
- Transfer of care (multi candidate): `ServiceRequest-np-eov-servicerequest`,
  `Task-np-eov-coordination-task-a/-b`, `PlanDefinition-np-eov-plandefinition`

## Open working-group questions

1. Canonical query list on the Task as well? How do information standards publish it?
2. SubscriptionTopic ownership: per dataset or per use case, under which canonical base URL?
3. Should `notification-authorization-hint` carry opaque tokens (currently not in profile)?
4. BSN placement alternatives (OAuth patient claim, Patient reference without identifier, BSN in
   notification) — decision pending.
