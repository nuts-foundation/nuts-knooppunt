# Knooppunt Integration Guide

This document describes how to integrate with the Knooppunt.

## Table of Contents

- [Addressing](#addressing)
- [NVI](#nvi)
- [Consent (MITZ)](#consent-mitz)

---

## Addressing

This chapter describes how to integrate with the addressing generic function of the Knooppunt,
based on the mCSD (Mobile Care Services Discovery) profile. It provides the following:

- Synchronization from remote mCSD Administration Directories to your local mCSD Query Directory, so it can be used to find organizations, endpoints, etc.
- Optional: an embedded mCSD Administration Directory web application to manage your local mCSD Administration Directory.

### Pre-requisites

You need to provide an mCSD Administration Directory, which is typically:

- A FHIR façade over an existing database or API, or
- a FHIR server (e.g. HAPI FHIR) in which mCSD resources are managed, either:
    - manually, e.g. using the embedded mCSD Admin web application (configure `mcsdadmin.fhirbaseurl`),
    - synchronized from another source in some way.

You also need to provide a FHIR server as mCSD Query Directory, to which mCSD resources are synchronized, e.g. HAPI FHIR.

Then, configure:

- the Root Administration Directory to synchronize from (`mcsd.admin.<key>.fhirbaseurl`), and
- the local Query Directory to synchronize to (`mcsd.query.fhirbaseurl`).

### Triggering synchronization

To synchronize remote mCSD Directories to your local query directory, use the following endpoint to trigger a synchronization:

```http
POST http://localhost:8081/mcsd/update
```

It will return a JSON report of the update per mCSD Administration Directory that was synchronized from, e.g.:

```json
{
  "https://example.com/mcsd": {
    "created": 1,
    "updated": 5,
    "deleted": 0,
    "warnings": [
      "Some-warning-message"
    ],
    "errors": [
      "Some-error-message"
    ]
  }
}
```

### Using the mCSD Administration Application

The Knooppunt contains a web-application to manually manage the mCSD Administration Directory entries (e.g. create organizations and endpoints).

You can find the mCSD Admin application at:

```http
http://localhost:8080/mcsdadmin
```

## NVI

This chapter describes how to integrate with the NVI (Nederlandse VerwijsIndex) service using the Knooppunt.

You can create or search for DocumentReference resources using the following endpoints:

- Registration endpoint: [POST http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)
- Search endpoint:
    - [POST http://localhost:8081/nvi/DocumentReference/_search](http://localhost:8081/nvi/DocumentReference/_search)
    - [GET http://localhost:8081/nvi/DocumentReference](http://localhost:8081/nvi/DocumentReference)

These endpoints need the URA of the requesting care organization. You provide this URA using the `X-Tenant-ID` HTTP header:

```http
X-Tenant-ID: http://fhir.nl/fhir/NamingSystem/ura|<URA>
```

## Consent MITZ

The Knooppunt acts as a gateway that simplifies MITZ integration by:

- **Abstracting complexity**: Handles technical details like mTLS authentication and FHIR validation
- **Providing unified APIs**: Offers consistent FHIR-based endpoints
- **Managing authentication**: Handles client certificates and service-to-service authentication
- **Configuration-based endpoints**: Uses configured notification endpoints for subscriptions

### Architecture

```
┌──────────────┐         HTTP/FHIR          ┌─────────────┐       HTTPS/mTLS      ┌──────────────┐
│              │  ────────────────────────► │             │  ───────────────────► │              │
│  Your EHR/   │                            │  Knooppunt  │                       │     MITZ     │
│  XIS System  │  ◄──────────────────────── │             │  ◄─────────────────── │  (Consent)   │
│              │                            │             │                       │              │
└──────────────┘                            └─────────────┘                       └──────────────┘
```

### Endpoints on the knooppunt

| Endpoint             | Method | Purpose                                   |
|----------------------|--------|-------------------------------------------|
| `/mitz/Subscription` | POST   | Create a consent subscription             |
| `/mitz/notify`       | POST   | Receive consent notifications (from MITZ) |

### Creating a Consent Subscription

Subscribe to consent notifications.

#### Request

```bash
curl -X POST http://localhost:8081/mitz/Subscription \
  -H "Content-Type: application/fhir+json" \
  -d '{
    "resourceType": "Subscription",
    "status": "requested",
    "reason": "OTV",
    "criteria": "Consent?_query=otv&patientid=123456789&providerid=00000001&providertype=Z3",
    "channel": {
      "type": "rest-hook",
      "payload": "application/fhir+json"
    }
  }'
```

#### Request Fields

**Required**:

- `status`: Must be `"requested"`
- `reason`: Must be `"OTV"` (Ontvangen Toestemmingen Vraag)
- `criteria`: Query string with:
    - `patientid`: Patient BSN (9 digits)
    - `providerid`: Provider URA (8 digits)
    - `providertype`: Healthcare provider type (e.g., `Z3` for hospitals)
- `channel.type`: Must be `"rest-hook"`

**Optional**:

- `channel.endpoint`: Notification callback URL (uses configured `notify_endpoint` if omitted)
- `channel.payload`: Content type (defaults to `"application/fhir+json"`)

#### Response

```json
{
  "resourceType": "Subscription",
  "id": "8904A5ED-713A-4A63-9B24-954AC7B7052D",
  "status": "requested",
  "reason": "OTV",
  "criteria": "Consent?_query=otv&patientid=123456789&providerid=00000001&providertype=Z3",
  "channel": {
    "type": "rest-hook",
    "endpoint": "https://platform.example.com/mitz/notify",
    "payload": "application/fhir+json"
  }
}
```

**HTTP Status**: 201 Created

### Notification Endpoint Configuration

The Knooppunt must be configured with a notification endpoint URL where MITZ will send consent change notifications.

#### Configuration

Set the `notify_endpoint` in your Knooppunt configuration (`knooppunt.yml`):

```yaml
mitz:
  mitzbase: "https://tst-api.mijn-mitz.nl"
  notify_endpoint: "https://your-platform.example.com/mitz/notify"
  # ... other MITZ settings
```

#### Endpoint Requirements

- **URL**: The endpoint URL where MITZ should send consent change notifications
- **Publicly accessible**: Must be reachable from MITZ infrastructure
- **Allow-listed**: Must be allow-listed by the MITZ team.
- **HTTPS recommended**: Use HTTPS for secure communication

#### Endpoint Precedence

1. **Explicit endpoint in request**: If `channel.endpoint` is provided in the subscription request, it takes precedence
2. **Configured endpoint**: If no endpoint is provided in the request, the configured `notify_endpoint` is used
3. **Missing endpoint**: If neither is provided, a warning is logged and the subscription may fail at MITZ

**Recommendation**: Always configure `notify_endpoint` to ensure subscriptions work without requiring clients to specify endpoints.

### Subscription Behavior

1. **Validation**: Knooppunt validates the subscription meets MITZ requirements
2. **Extension Addition**: Automatically adds gateway and source system OIDs
3. **Endpoint Setting**: Uses configured `notify_endpoint` if no endpoint provided in request (see [Notification Endpoint Configuration](#notification-endpoint-configuration))
4. **Forwarding**: Sends subscription to MITZ with mTLS authentication
5. **Response**: Returns created subscription with ID

### Notification Handling

When consent changes occur, MITZ sends notifications to the configured endpoint.
