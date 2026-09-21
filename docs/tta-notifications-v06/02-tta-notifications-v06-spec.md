# TTA Notifications (v0.6) — specification extract

> Source: https://ontwikkelsupplement.twiin.nl/actueel/10-3-1-1-tta-notifications-v0-6
> (Twiin ontwikkelsupplement, published 2026-08-27). English extraction — check source for normative wording.
>
> This technical agreement (TTA) specifies how Dutch healthcare organisations exchange notifications
> using the FHIR R5 Subscription framework applied to FHIR R4/R4B via the **Subscriptions R5 Backport IG**.

## 1. Notification payload model

Three payload modes determine what travels in the notification Bundle vs what requires separate retrieval:

| Mode | Content | Use |
|------|---------|-----|
| `empty` | No identifying metadata or clinical data; only "a matching event occurred" | Notified Pull when identifiers can't meet id-only requirements |
| `id-only` | Reference to the affected resource (`notification-event.focus`), no clinical content | Notified Pull, preferred when identifier requirements are met |
| `full-resource` | Complete FHIR resource instance | Direct push — "sending it is the disclosure, not a precursor to one" |

**Full-resource** does not participate in Notified Pull: there is no separate pull step; the applicable
consent basis must cover direct disclosure of the clinical content and must be verified at the exact
moment the notification is sent. (Out of scope for this POC.)

**Payload selection for Notified Pull**: id-only vs empty depends on whether "the identifier itself
gives nothing away".

### Requirements for using id-only

Resource identifiers in id-only notifications must satisfy four binding requirements:

1. **Stability** — a source SHALL return the same identifier for the same resource on every request.
2. **Opacity** — an identifier SHALL NOT reveal creation order, volume, creation time, or content.
3. **Unpredictability** — an identifier SHALL NOT be guessable, and SHALL NOT be computable or
   confirmable from real-world entity identifiers.
4. **Consumer opacity** — a consumer SHALL treat received identifiers as opaque strings: it SHALL NOT
   parse them, derive meaning from them, or assume a format.

"An identifier that fails the opacity or unpredictability requirement leaks something about the
resource to anyone who merely sees the notification."

- For **single-receiver resources** whose ids meet the requirements, id-only **SHOULD** be used in
  preference to empty: a direct read of a compliant reference discloses exactly one resource, whereas
  searching (needed under empty mode) "can expose a broader set of resources than the notification ever
  intended to point to".
- For **broadly accessible resources**, whether id-only remains appropriate — or per-recipient
  identifiers or empty mode should be used — is a decision for the information standard governing the exchange.
- **Empty mode with a leaked id is pseudo id-only**: exposing the resource id through other channels
  carries the same disclosure as id-only without meeting or governing its requirements.

### Identifier schemes that fail opacity/unpredictability

- **Ordered** (sequential counters): reveal creation order and volume; guessable
- **Time-encoding** (UUIDv1, UUIDv7): reveal creation moment
- **Concatenated**: reveal content directly
- **Derived** (unkeyed hash, UUIDv5): reverse-engineerable by brute force
- **Low-entropy**: guessable when access control fails
- **Volatile**: not stable across requests; break deduplication

Resources identified only by failing schemes should be exchanged under **empty** mode, not id-only.

### Identifier examples (compliant)

| Scheme | Description | Example |
|--------|-------------|---------|
| Random (UUIDv4) | Random value created once and stored with the resource | `5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d` |
| Keyed transformation (HMAC) | Computed from the internal reference with a keyed transform; stable but not reversible without the source's key | `Task/4711` → `j5xw6z3vnfxgk4ttmvwgc3dj` |
| Per-recipient | Keyed transform where the key is derived per receiver; receivers can't link ids across views | `Task/4711`+URA `12104037` → `k2mdpq7fbzxtc4qslmvwgy3e`; `Task/4711`+URA `54783291` → `r8ntgh5xowzslb9cqfj47x2n` |

### Payload mode agreement and enforcement

- Payload modes SHALL be explicitly defined in the governing exchange agreement before a subscription
  is created. Systems SHALL NOT accept arbitrary subscription requests — only agreed subscriptions and modes.
- The mode is locked at subscription creation and SHALL only be changed via an explicit PATCH; every
  notification sent afterward SHALL adhere to the registered mode.
- **In-band creation** (receiver creates the Subscription): the receiver SHALL only specify a
  pre-approved mode; the sender is bound once it accepts and SHALL NOT accept modes outside the agreement.
- **Out-of-band creation** (sender creates the Subscription): the receiver MAY inspect the payload
  mode by fetching the Subscription resource, but that check is not part of the handshake, and
  acknowledging the handshake does not constitute agreement on the mode. If the mode is outside the
  agreement, the receiver SHALL reject/refuse the handshake; even after a successful handshake it
  retains the right to reject later deviating notifications.

## 2. Notification patterns

- **Source-initiated**: the sending provider decides the receiver should be informed of events, even
  though the receiver did not request it; the sender establishes and manages the relationship
  (e.g. referral updates, consult reports). Associated with **out-of-band** creation.
- **Receiver-initiated**: the receiving provider explicitly requests to be informed (e.g. lab results,
  changes to specific patient information). Associated with **in-band** creation.
- These concepts are related but distinct: the pattern describes which party initiates the
  relationship; in-band/out-of-band describes the technical mechanism for establishing the Subscription.

Heartbeats "are less valuable for source-initiated notifications".

## 3. Preconditions

1. All accepted notifications and subscriptions (in-band and out-of-band) MUST be well-defined by
   **SubscriptionTopics agreed between all parties**.
2. All consent requirements (GF Toestemming/EHDS) for the disclosure type associated with the
   Subscription's payload mode MUST be satisfied **before a Subscription is activated**.
3. The receiver's authorization to access the SubscriptionTopic for a specific patient MUST be
   established via **GF Authorization** before a Subscription is activated.
4. All endpoints MUST adhere to Twiin network security level standards (10.4.7 Network level
   security); **mutual TLS MUST be applied**.

## 4. FHIR technical specification

### Conformance

Implementations MUST conform to the **HL7 FHIR R4/R4B Subscriptions R5 Backport IG**.

### Sequence (summary of the diagram)

1. Subscription registration (in-band or out-of-band) for a pre-agreed SubscriptionTopic
2. Handshake — server verifies channel reachability
3. Event notifications per matching event
4. Heartbeats at pre-agreed intervals (if used)
5. Catch-up after disruption via `$status` / `$events`
6. Follow-up: receiver retrieves content via the sender's FHIR endpoint

### Resource definitions

**Notification Bundle** — FHIR `Bundle` of type `history`, conforming to profile
`backport-subscription-notification`. Contains a `Parameters` resource (SubscriptionStatus) with:

| Field | Requirement |
|-------|-------------|
| `subscription` | Reference to the registered Subscription |
| `status` | `active` or `off` |
| `type` | `event-notification`, `handshake-notification`, or `heartbeat-notification` |
| `notification-event.event-number` | Event-notifications only; monotonically increasing, used to detect missed events |
| `notification-event.timestamp` | Event-notifications only; moment the event occurred at the server |
| `notification-event.focus` | Present only in id-only mode |
| Clinical content | Present only in full-resource mode |

**SubscriptionTopic**:

| Element | Requirement |
|---------|-------------|
| `url` | MUST carry a unique canonical URL naming the topic |
| `resourceTrigger.resource` / `resource` | MUST specify the single monitored resource type |
| `resourceTrigger.supportedInteraction` | MUST specify triggering interactions (create, update, …) |
| `canFilterBy` | MAY define filterable fields |
| `queryCriteria` | MAY narrow via coded criteria |

**Subscription** (when created in-band, MUST satisfy):

| Element | Requirement |
|---------|-------------|
| `criteria` + `backport-topic-canonical` ext | MUST reference exactly one pre-agreed SubscriptionTopic |
| `backport-filter-criteria` ext | MAY carry a scoping filter narrowing events |
| `channel.type` | MUST be `rest-hook` |
| `channel.endpoint` | Subscription Client notification endpoint |
| `channel.payload` | MUST be `application/fhir+json` |
| `backport-payload-content` ext | MUST specify the payload mode: `empty`, `id-only`, or `full-resource` |

### Transactions

#### Creating a Subscription (in-band)

`POST /Subscription`, `Content-Type: application/fhir+json`

| Status | Meaning |
|--------|---------|
| 201 Created | Subscription created; status set to `requested` |
| 400 Bad Request | Parse/FHIR validation failure |
| 422 Unprocessable Entity | SubscriptionTopic unknown or business rules violated |

Rules: status becomes `active` only after a successful handshake; on retirement set to `off` (not deleted).

Example request (id-only with filter):

```json
{
  "resourceType": "Subscription",
  "status": "requested",
  "criteria": "SubscriptionTopic/task-status-change",
  "_criteria": {
    "extension": [
      {
        "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-topic-canonical",
        "valueCanonical": "https://example.org/fhir/SubscriptionTopic/task-status-change"
      },
      {
        "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-filter-criteria",
        "valueString": "Task.owner=Organization/receiver-org"
      }
    ]
  },
  "channel": {
    "type": "rest-hook",
    "endpoint": "https://receiver.example/fhir/notifications",
    "payload": "application/fhir+json",
    "_payload": {
      "extension": [
        {
          "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-payload-content",
          "valueCode": "id-only"
        }
      ]
    }
  }
}
```

#### Handshake notification

Trigger: Subscription created with status `requested`.
`POST {Subscription.channel.endpoint}`, `Content-Type: application/fhir+json`

| Status | Meaning |
|--------|---------|
| 200 OK | Handshake received; `Subscription.status` → `active` |
| 4xx/5xx | Retry with exponential backoff; persistent failure → status `error` |

```json
{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "requested" },
          { "name": "type", "valueCode": "handshake-notification" }
        ]
      }
    }
  ]
}
```

#### Event notification

Trigger: event matches topic + filter criteria.
`POST {Subscription.channel.endpoint}`, `Content-Type: application/fhir+json`

| Status | Meaning |
|--------|---------|
| 200 OK | Notification received and accepted |
| 422 Unprocessable Entity | Business rules violated; OperationOutcome returned |

Rules: event-number MUST be assigned concurrency-safely and be monotonically increasing; the Bundle
MUST NOT carry clinical content beyond the agreed payload mode.

Example (id-only, Task focus):

```json
{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "fullUrl": "urn:uuid:c3a5d8f1-9b2e-4d67-8a4c-5e1f7b9d2a36",
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "active" },
          { "name": "type", "valueCode": "event-notification" },
          { "name": "notification-event",
            "part": [
              { "name": "event-number", "valueString": "42" },
              { "name": "timestamp", "valueInstant": "2026-07-16T09:15:00Z" },
              { "name": "focus",
                "valueReference": { "reference": "Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d" } }
            ] }
        ]
      }
    },
    {
      "fullUrl": "https://sender.example/fhir/Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d",
      "request": { "method": "GET", "url": "Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d" },
      "response": { "status": "200" }
    }
  ]
}
```

#### Heartbeat notification (optional)

Trigger: pre-agreed heartbeat interval elapses.
`POST {Subscription.channel.endpoint}`; 200 OK on receipt; 4xx/5xx → retry with exponential backoff.
Optional per Subscription, agreed at creation. Receiver resets its missed-heartbeat timer on receipt
and SHOULD query `$status` if a heartbeat is missed.

```json
{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "active" },
          { "name": "type", "valueCode": "heartbeat-notification" }
        ]
      }
    }
  ]
}
```

#### `$status` operation (required)

`GET /Subscription/[id]/$status` — query current subscription state, typically after a missed heartbeat.
200 OK → SubscriptionStatus Parameters; 404 → unknown id; OperationOutcome on error.

#### `$events` operation (optional)

`GET /Subscription/[id]/$events` — retrieve a specific event-number or range, typically after a
sequence gap. 200 OK → notification-event Bundle(s); 404 → unknown id; 422 → range invalid or
unavailable. Recovery-only, not the regular flow. Retention of replayable events is negotiated per use case.

## 5. System roles and responsibilities

**Subscription Server** (sending organisation):
- Create Subscription resources for agreed topics; accept in-band creation for pre-agreed topics only
- Send handshake notifications, with retry on failure
- Send event-notification Bundles with concurrency-safe incrementing event-number
- Support `$status` and the specified search parameters
- Verify authorization and consent before each notification send

**Subscription Client** (receiving organisation):
- Receive notification Bundles at the notification endpoint and forward them internally
- Accept handshake-notification Bundles
- Check incoming Bundles for continuity using the highest processed event-number
- Optionally check for missed heartbeat notifications

## 6. Implementation obligations

1. **Logging** — both parties MUST log the sending and receipt of subscriptions and notifications
   (including handshake and, where used, heartbeat), with sufficient detail for incident
   reconstruction and substantiating authorization/consent verification.
2. **Authorization verification** — the server MUST verify, before activating a Subscription and again
   immediately before sending each notification, that the receiver is authorized.
3. **Consent verification** — the server MUST verify, immediately before sending each notification,
   that the consent conditions applicable to that payload mode are still met.
4. **Error reporting** — all transactions MUST return a FHIR OperationOutcome with an appropriate HTTP
   status code in case of errors.
5. **Retry** — clients MUST retry transient errors (5xx, network errors) with exponential backoff.
6. **Idempotency** — processing MUST be idempotent: re-delivering a previously processed notification
   MUST NOT cause duplicate side effects.

## 7. Using this TA

This TA defines the generic notification mechanism; it does not itself activate any exchange. Using it
for a specific case means agreeing, beyond this document: the SubscriptionTopic involved, the payload
mode for that topic, and the applicable authorization and consent basis.

## 8. Changes since v0.5

- Identifier construction examples rewritten (property description + resulting id)
- Examples section scoped to identifier construction; event-notification example relocated
- Example payloads added to subscription-creation, handshake, event and heartbeat transactions
- "Notification Content" renamed to "Resource Definitions" and repositioned; SubscriptionTopic
  definition aligned with the Subscription structure; Notification definition trimmed
- Duplicate content rules consolidated
- Transaction headings promoted one level
- "Error Handling" folded into "Implementation Obligations" with three additional requirements
- Sequence diagram added (mandatory handshake, correct operation naming, both creation methods)
- Table of contents introduced

## Related Twiin documents

- 10.3.1 TTA FHIR — Harmonized Notified Pull (v2.x): https://ontwikkelsupplement.twiin.nl/actueel/10-3-1-tta-fhir-harmonized-notified-pull-v2-x
- Previous versions v0.2–v0.5 under the same section
- eOverdracht Notifications Implementation Guide v0.1 / v0.2 (notification model only)
- 10.3.3 TTA FHIR — Pull: https://ontwikkelsupplement.twiin.nl/actueel/10-3-3-tta-fhir-pull
