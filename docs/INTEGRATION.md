# Knooppunt MITZ Integration Guide

This document describes how vendors can integrate with the Knooppunt to access MITZ (Mijn Informatie Toestemmingen Zorg) - the Dutch national consent management system for healthcare.

## Table of Contents

- [Overview](#overview)
- [MITZ Integration](#mitz-integration)
- [Testing Your Integration](#testing-your-integration)

---

## Overview

The Knooppunt acts as a gateway that simplifies MITZ integration by:

- **Abstracting complexity**: Handles technical details like mTLS authentication and FHIR validation
- **Providing unified APIs**: Offers consistent FHIR-based endpoints
- **Managing authentication**: Handles client certificates and service-to-service authentication
- **Auto-discovery**: Automatically discovers notification endpoints from mCSD (see [mCSD Endpoint Configuration](#mcsd-endpoint-configuration))

### Architecture

```
┌──────────────┐         HTTP/FHIR          ┌─────────────┐       HTTPS/mTLS      ┌──────────────┐
│              │  ────────────────────────► │             │  ───────────────────► │              │
│  Your EHR/   │                            │  Knooppunt  │                       │     MITZ     │
│  XIS System  │  ◄──────────────────────── │             │  ◄─────────────────── │  (Consent)   │
│              │                            │             │                       │              │
└──────────────┘                            └─────────────┘                       └──────────────┘
```

---

## MITZ Integration

MITZ is the Dutch national consent management system for healthcare.

### Endpoints on the knooppunt

| Endpoint | Method | Purpose                                   |
|----------|--------|-------------------------------------------|
| `/mitz/Subscription` | POST | Create a consent subscription             |
| `/mitz/notify` | POST | Receive consent notifications (from MITZ) |



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
- `channel.endpoint`: Notification callback URL (auto-discovered from mCSD if omitted - see [mCSD Endpoint Configuration](#mcsd-endpoint-configuration))
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

### mCSD Endpoint Configuration

For automatic notification endpoint discovery, an Endpoint resource must be configured in mCSD (Mobile Care Services Discovery).

#### Required Endpoint Resource

The mCSD administration directory must contain an Endpoint resource with the following characteristics:

**PayloadType**:
- Must include a coding with:
  - `system`: `http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities`
  - `code`: `consent-notify`

**Address**:
- The URL where MITZ should send consent change notifications
- Must be publicly accessible from MITZ
- Must be whitelisted by the MITZ team, **OR** use the endpoint already whitelisted by Knooppunt (contact Rein for details)

#### Example Endpoint Resource

```json
{
  "resourceType": "Endpoint",
  "id": "consent-notify-endpoint",
  "status": "active",
  "connectionType": {
    "system": "http://terminology.hl7.org/CodeSystem/endpoint-connection-type",
    "code": "hl7-fhir-rest"
  },
  "name": "Consent Notification Endpoint",
  "payloadType": [
    {
      "coding": [
        {
          "system": "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities",
          "code": "consent-notify"
        }
      ]
    }
  ],
  "address": "https://your-platform.example.com/mitz/notify"
}
```

#### Behavior

- **If Endpoint exists in mCSD**: Knooppunt automatically uses this address for `channel.endpoint` in the subscription
- **If Endpoint not found**: You must provide `channel.endpoint` explicitly in your subscription request
- **If both provided**: The endpoint in your request takes precedence over the mCSD-discovered endpoint

**Note**: Contact your Knooppunt administrator to verify the mCSD Endpoint configuration.

### Subscription Behavior

1. **Validation**: Knooppunt validates the subscription meets MITZ requirements
2. **Extension Addition**: Automatically adds gateway and source system OIDs
3. **Endpoint Discovery**: Auto-discovers notification endpoint from mCSD if not provided (see [mCSD Endpoint Configuration](#mcsd-endpoint-configuration))
4. **Forwarding**: Sends subscription to MITZ with mTLS authentication
5. **Response**: Returns created subscription with ID


### Notification Handling

When consent changes occur, MITZ sends notifications to the configured endpoint.

**Note**: For development/testing, the NGINX proxy handles notifications (returns 201 without processing). Using this proxy url, you don't need to worry about whitelisting your endpoint with the MITZ team. Contact Rein for endpoint details.

---

## Testing Your Integration

### Test MITZ Subscription Creation

```bash
# Create a subscription
curl -X POST http://localhost:8081/mitz/Subscription \
  -H "Content-Type: application/fhir+json" \
  -d '{
    "resourceType": "Subscription",
    "status": "requested",
    "reason": "OTV",
    "criteria": "Consent?_query=otv&patientid=999999990&providerid=00000001&providertype=Z3",
    "channel": {
      "type": "rest-hook"
    }
  }'
```

### Troubleshooting

#### MITZ Errors

**Error**: `Failed to create subscription at MITZ endpoint`
- **Cause**: MITZ connection issue or invalid subscription
- **Solution**:
  - Verify Knooppunt is running and MITZ is configured
  - Check Knooppunt logs for connection errors
  - Validate subscription criteria format

**Error**: `Subscription.status must be 'requested'`
- **Cause**: Invalid subscription status
- **Solution**: Always use `"status": "requested"` for new subscriptions

**Error**: `No consent notify endpoint found in mCSD`
- **Cause**: mCSD not configured with notification endpoint
- **Solution**: Either provide `channel.endpoint` in request or configure mCSD to include Endpoint with the right payload-type (see [mCSD Endpoint Configuration](#mcsd-endpoint-configuration))

**Error**: `Connection refused`
- **Cause**: Knooppunt not running or wrong port
- **Solution**: Verify Knooppunt is running on the expected port

**Error**: `403 Forbidden` from MITZ
- **Cause**: Client certificate not whitelisted or mTLS authentication failed
- **Solution**: Contact Knooppunt administrator to verify MITZ configuration

### Logs and Debugging

View Knooppunt logs to debug integration issues. Look for:
- **MITZ component logs**: Messages related to subscription creation and MITZ communication
- **Validation errors**: Details about why a subscription was rejected
- **Connection errors**: Network or authentication issues with MITZ

Contact your Knooppunt administrator for access to logs.

---

## See Also

- [MITZ Component Documentation](../component/mitz/README.md) - Technical implementation details
- [MITZ Prerequisites](../component/mitz/README.md#prerequisites) - Certificate and endpoint whitelisting requirements
