# Knooppunt Integration Guide

This document describes how to integrate with the Knooppunt.

## Table of Contents

- [Addressing](#addressing)
- [NVI](#nvi)
- [Consent (MITZ)](#consent-mitz)
- [Authentication](#authentication)

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

- the Root Administration Directory to synchronize from (`mcsd.admin.<key>.fhirbaseurl`),
- the local Query Directory to synchronize to (`mcsd.query.fhirbaseurl`), and
- (optional) directories to exclude from synchronization (`mcsd.adminexclude`), which is useful to prevent self-referencing loops when your own query directory appears as a discovered Endpoint.

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

Make sure you've configured the `authn.minvws` properties to allow the Knooppunt to authenticate to the NVI service.

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
  mitzbase: "https://tst-api.mijn-mitz.nl/tst-us/mitz"
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

## Authentication

This chapter describes how to use the Knooppunt to perform data exchanges, leveraging GF Authentication.

The EHR will need to:

- Integrate with Dezi to have its end-users log in
- Store the decrypted ID token from Dezi for use in data exchanges
- Use the Knooppunt to request access tokens for data exchanges

The Knooppunt will:

- Perform access token request at remote EHRs (Nuts node) for outbound data exchanges.
  - If user is involved: take the decrypted ID token (Dezi) to include in the access token request
- Validate the organization credentials and end-user credential (Dezi ID token) for inbound data exchanges.

For more information on the Knooppunt's authentication endpoints, see the [Nuts node API reference](https://nuts-node.readthedocs.io/en/stable/pages/integrating/api.html).

### Getting an access token

Use the Nuts node's "request service access token" endpoint to get an access token:

```http
POST http://localhost:8081/nuts/auth/v2/{subjectID}/request-service-access-token
Content-Type: application/json

{
  "authorization_server": "https://example.com/oauth2",
  "scope": "some-scope",
  "id_token": "eyJhbGci..."
}
```

To provide an end-user identity, include the `id_token` field with the decrypted ID token from Dezi.
If no end-user identity is required, you may omit the `id_token` field.

Note that to successfully negotiate an access token, the local Nuts node must have been loaded with the right credentials.
Which credentials are required, depends on the use case.


### Verifying access tokens

Use the token introspection endpoint to verify the access token:

```http
POST http://localhost:8081/nuts/auth/v2/accesstoken/introspect
Content-Type: application/x-www-form-urlencoded

token=eyJhbGciOi...
```

The response contains claims about the requesting party and if provided, claims about the end-user (Dezi).

Example:

```json
{
  "active": true,
  "client_id": "https://nodeB/oauth2/vendorB",
  "cnf": {
    "jkt": "ESawEozHRACsFtrnGysiMwUu2vz9jpeWNToSBEBa9CQ"
  },
  "exp": 1771330157,
  "iat": 1771329257,
  "iss": "https://nodeA/oauth2/vendorA",
  "scope": "test",
  "organization_ura": "12345678",
  "organization_name": "Hospital East",
  "organization_city": "Amsterdam",
  "employee_identifier": "87654321",
  "employee_initials": "J.",
  "employee_surname_prefix": "van der",
  "employee_surname": "Broek",
  "employee_roles": ["01.041", "30.000", "01.010", "01.011"]
}
```

Note that the returned fields depend on the Nuts Access Policy that was loaded in the Knooppunt.