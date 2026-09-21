# GF IG — Notification (concept, branch build)

> Source: https://build.fhir.org/ig/nuts-foundation/nl-generic-functions-ig/branches/split-notification-and-workflow-page/notification.html
> Netherlands Generic Functions IG (fhir.nl.gf, draft), Stichting Nuts. English extraction.

## Overview

Specifies how FHIR-based event notifications are exchanged between Dutch healthcare organisations,
built on the **FHIR Subscription Framework implemented in FHIR R4 via the Subscriptions R5 Backport**.

Core principles:
- Adhere to international HL7 FHIR standards
- Server-managed subscription style (**out-of-band creation**)
- "Notifications carry no clinical content. The sender remains the source of truth"

## Subscription Server (sending organisation)

- Create and manage Subscription resources
- Transmit handshake, heartbeat, and event notifications with retry logic
- Maintain monotonically increasing event numbers
- Support `$status` and `$events` operations
- Enable Subscription resource queries with filters for status, criteria, endpoint, type, and payload

## Subscription Client (receiving organisation)

- Receive notification Bundles at designated endpoints
- Check each notification Bundle to ensure no events were missed, using the highest event-number
- Detect heartbeat failures and query subscription status
- Track subscription state and resolve communication errors

## Data models

**Subscription**: `status` active/off; references SubscriptionTopic via canonical URL;
`channel.type = rest-hook` with the receiver endpoint; `application/fhir+json` payload with `id-only` content.

**Notification Bundle**: conforms to `backport-subscription-notification`; Parameters with
subscription reference, status, type (event/handshake/heartbeat), monotonically increasing
event-number (receiver uses it to detect missed events), timestamp, and focus reference.

**SubscriptionTopic**: defines triggering events and applicable resource shapes; owned per use case
by the respective Implementation Guides.

## Security

- Notification endpoints MUST require **mTLS**
- Authentication and authorization follow the **GF Authorization** specification
- Clinical content is only disclosed during the subsequent FHIR retrieval, not in notifications

## Example use case

A sending organisation creates a long-lived Subscription for a partner organisation when a Task names
that partner as owner (identified by **URA**). Upon Task creation, an event-notification alerts the
partner via its notification endpoint to retrieve full details.

## Related IG pages (same branch build)

index, care-services (Care Services Directory / addressing), routing, localization, consent,
identification, authentication, authorization, building-blocks, **notified-pull**,
**subscription-topics-exploration**, **clinical-workflow**, credential-catalog, artifacts.

GitHub: https://github.com/nuts-foundation/nl-generic-functions-ig/
