# POC TTA Notifications v0.6 — assignment (Nuts wiki)

> Source: https://wiki.nuts.nl/books/kring-techniek/page/poc-tta-notifications-v06 (translated from Dutch)
>
> Status: in execution · Client: Steven van der Vegt, coordinator Kring Techniek · Execution: Gasper (COT)
> Type: operational assignment from the coordinator, announced at the circle meeting

## What

Build a proof of concept of TTA Notifications v0.6 in the node on a separate branch. Focus is on
answering research questions, not code quality. Implementation covers both roles:

- **Subscription Server**: subscriptions (in-band and out-of-band), handshake notifications, event
  notifications, heartbeats, `$status` and `$events` operations, exponential backoff retry
- **Subscription Client**: notification endpoint, event-number continuity check, heartbeat detection,
  idempotent processing
- **Payload modes**: `empty` and `id-only` (`full-resource` excluded)
- **Example**: SubscriptionTopic on Task for eOverdracht alignment
- **Logging**: sent and received subscriptions and notifications

Permutations of optional features (in-band vs out-of-band, heartbeat yes/no, `$events` yes/no) should be
tested with server–client pairs making different choices.

## Why

1. Feedback on the specification to Twiin for TA Notified Pull harmonisation
2. Input for the impact analysis by applications (particularly eOverdracht)

## Research questions

1. Is the specification complete and correct? Where are gaps or contradictions?
2. How much implementation effort is required?
3. How much does a reference implementation in the node abstract away? What does the node manage vs the application?
4. Which aspects get concrete help from a node implementation?
5. Do authentication/authorization fit our model? mTLS alignment?
6. What happens when server and client choose different optional features?
7. FHIR version compatibility (R4/R4B vs STU3)?
8. Do current application identifiers meet the id-only requirements?
9. How does the node resolve notification and Subscription endpoints via GF Adressing?
10. What are the differences with the current eOverdracht notification and the Nuts TA NP variant?

## When

- Start: September 7, 2026
- Duration: 5 workdays (3 code, 2 reporting)
- Delivery: September 21
- Evaluation: at the demo

## Who

- Execution: Gasper
- Assignment owner: Steven van der Vegt
- Feedback to: Kring Techniek, TA NP subworking group, Werkgroep Technische Convergentie

## Where

- Code: separate branch in the node repository
- Reporting: shareable documents
- Demo: COT with subworking groups

## Results

1. Working POC on a branch, both roles, demonstrable
2. Demo presentation
3. Findings document with answers to the research questions (spec issues for Twiin; implementation
   impact for applications)
4. Brief conclusion for the Werkgroep Technische Convergentie

## Relationships with other initiatives

- TA Notified Pull harmonisation (source working group)
- Twiin fall release 1.6 (consultation venue)
- Werkgroep Technische Convergentie (impact-analysis location)
- Generieke Functies (node and integration-phase funding)
