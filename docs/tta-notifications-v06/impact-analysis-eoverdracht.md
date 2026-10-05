# Impact analysis: TTA Notifications v0.6 for vendors supporting eOverdracht today

## 1. Bottom line

1. **The notification itself gets simpler; everything around it gets heavier.** The message
   on the wire shrinks from a bespoke STU3 Task to a small standard Bundle.
2. **v0.6 alone is a moderate change. The package it arrives in is a large one.** The
   harmonised Notified Pull pairs v0.6 with the COW workflow profile: STU3 → R4, a new
   persisted Request resource, a new Task profile, a new cancellation model.
3. **A node (knooppunt) can absorb almost all of the notification layer.**

## 2. What changes, concern by concern

| Concern                           | Today                                                                           | With v0.6 (+ COW profile)                                                                                                                      | Impact                                                                                             |
|-----------------------------------|---------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| Notification message              | STU3 Task with code, `basedOn`, sender URA, inputs                              | R4 `history` Bundle with SubscriptionStatus: event number, timestamp, opaque Task reference                                                    | Simpler payload                        |
| Reliability                       | None                                                                            | Event numbers + continuity check, `$status` (required), `$events` (optional), heartbeat (optional), exponential backoff, mandatory idempotency | **New** on both sides; the largest part of the notification-layer work                             |
| Sender identity                   | In the notification (`requester.onBehalfOf`) and the access token               | Not in the notification; expected from the transport (mTLS) or token                                                                           | Receiver's "who sent this → where do I pull" logic must change                                     |
| Authorization of the notification | Access token with scope `ta-np-notification`, PDP at receiver                   | Precondition: mutual TLS + GF Authorization; sender verifies authorization and consent **before every send**                                   | Sender gains a per-send check; transport security model changes                                    |
| Identifiers                       | Workflow Task id sent as-is in `basedOn`                                        | Task id in id-only mode SHALL be stable, opaque, unpredictable                                                                                 | Most systems' ids fail; alias, re-key, or fall back to `empty` mode                                |
| FHIR version                      | STU3 throughout                                                                 | R4/R4B for the notification layer; COW profile is R4                                                                                           | STU3 → R4 migration of Task and data endpoints                                                     |
| Workflow model                    | Nictiz eOverdracht Task; payload referenced from `Task.input`                   | COW Coordination Task + separate persisted Request (ServiceRequest); dataset named by `Request.code`                                           | New resources and profile; mapping in doc 06                                                       |
| Status updates                    | Receiver updates Task at sender; no notification back                           | Every Task change fires a notification, **including the receiver's own writes**                                                                | Receiver must handle self-triggered events idempotently                                            |
| Cancellation                      | Status change                                                                   | Direct cancel before `in-progress`; negotiated CancellationRequest Task after                                                                  | New workflow path                                                                                  |
| Routing to department             | Notification endpoint chosen via HealthcareService; extensions on workflow Task | One notification endpoint per organisation per subscription; routing info only after pulling the Task                                          | Internal routing moves entirely to after the pull; routing model in the COW profile is not settled |

## 3. Impact per role

Sizing is relative (S/M/L), for a vendor implementing it themselves. 

### 3.1 Sending system — notification layer

| Work item                                                                                         | Size    | Notes                                                                                         |
|---------------------------------------------------------------------------------------------------|---------|-----------------------------------------------------------------------------------------------|
| Subscription store and lifecycle (`requested → active → error/off`), handshake with retry         | M       | New persistent state                                                                          |
| Event detection on Task create/update, topic + owner filter matching                              | S–M     | Most systems already know when they change a Task                                             |
| Per-subscription monotonic event numbers, durable and concurrency-safe                            | M       | Must survive restart and restore; a rolled-back counter silently drops events at the receiver |
| Notification Bundle construction per payload mode; delivery with exponential backoff              | S       |                                                                                               |
| `$status` (required); `$events` + event log + retention (optional)                                | S / M   |                                                                                               |
| Heartbeat sending (optional)                                                                      | S       |                                                                                               |
| Authorization and consent verification before each send                                           | M       | Depends on GF Authorization outcome                                                           |
| Identifier compliance                                                                             | **S–L** | S if ids are already random UUIDs; L if an alias layer and de-aliasing pull path are needed   |

### 3.2 Receiving system — notification layer

| Work item                                                                                                 | Size | Notes                                                                                |
|-----------------------------------------------------------------------------------------------------------|------|--------------------------------------------------------------------------------------|
| Notification endpoint for Bundles (replaces the Task endpoint); classify handshake / heartbeat / event    | S    |                                                                                      |
| Subscription bookkeeping per partner; accept or refuse handshakes against agreed topics and payload modes | M    | Refusal requires being able to fetch the sender's Subscription — see FINDINGS RQ 1.6 |
| Continuity check, gap recovery via `$events`, fallback to `$status` + resync                              | M    | Durable "highest processed event" per subscription                                   |
| Idempotent processing (dedupe on subscription + event number)                                             | S    |                                                                                      |
| Heartbeat-miss detection (optional)                                                                       | S    |                                                                                      |
| In-band subscription creation (only if receiver-initiated is used)                                        | S    |                                                                                      |

### 3.3 Both roles — workflow and data layer (COW profile, R4)

| Work item                                                                                                                                   | Size  | Notes                                              |
|---------------------------------------------------------------------------------------------------------------------------------------------|-------|----------------------------------------------------|
| STU3 → R4 for Task and the pulled data (Composition and referenced resources)                                                               | **L** | The dominant cost; independent of v0.6 itself      |
| COW Coordination Task profile: two codings on `Task.code`, mandatory business identifier, `businessStatus`, `Task.for` with BSN timing rule | M     |                                                    |
| New persisted Request resource (ServiceRequest) as `Task.focus`                                                                             | M     | eOverdracht has no equivalent today                |
| CancellationRequest Task workflow after `in-progress`                                                                                       | M     |                                                    |


## 4. With a knooppunt versus without

|                                               | Vendor implements v0.6 directly | Vendor integrates via a knooppunt                                                                                                                                                                             |
|-----------------------------------------------|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Notification layer (§4.1, §4.2)               | All of it                       | Report Task changes (or expose pollable Task history); register the authorization basis; poll and acknowledge a normalised inbox; state subscription intent. Plain REST, no Subscriptions-framework knowledge |
| Durable protocol state                        | Vendor's database               | Knooppunt's — **which today has no persistence layer**; a FHIR store partition per organisation would do                                                                                                      |
| Identifier compliance                         | Vendor's problem                | Knooppunt can alias, but then sits in the pull path                                                                                                                                                           |
| Transport security, logging                   | Vendor                          | Knooppunt / PEP (inbound mTLS is not available in the knooppunt today)                                                                                                                                        |
| Workflow and data layer (§4.3)                | All of it                       | **Unchanged — all of it.** A node does not migrate STU3 data or implement the COW Task lifecycle                                                                                                              |

