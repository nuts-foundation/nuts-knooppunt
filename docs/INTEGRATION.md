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

This chapter describes how to use the Knooppunt to authenticate users through GF Authentication.
The Knooppunt acts as OpenID Connect (OIDC) Provider, abstracting the complexity of Dezi integration:

- Decrypting the envelope containing the Dezi token
- Performing revocation checking
- Validating the token according to the business rules of Dezi
- Providing an OIDC `id_token` that follows standard OIDC claims

This OIDC Provider supports the following OIDC features:

- [Authorization Code Flow](https://openid.net/specs/openid-connect-core-1_0.html#CodeFlowAuth)
- [Discovery using well-known metadata](https://openid.net/specs/openid-connect-discovery-1_0.html) (on internal API: `http://localhost:8081/.well-known/openid-configuration`)
- [Client authentication using `client_secret`](https://openid.net/specs/openid-connect-core-1_0.html#ClientAuthentication)
- [PKCE using S256](https://www.rfc-editor.org/rfc/rfc7636)

To use the Knooppunt as OIDC Provider:

1. Register your client (e.g. EHR) in the Knooppunt configuration (see below).
2. Configure your Dezi client in the Knooppunt (coming later).

### Client Authentication

Clients to the Knooppunt OIDC Provider must:

- be authenticated using `client_secret`
- have its redirect URLs registered for the authorization code flow.

Configure clients and their redirect URLs in the Knooppunt configuration (`knooppunt.yml`).

### ID Tokens

The `id_token` returned by the Knooppunt wraps the Dezi token, providing standard OIDC claims as well as Dezi-specific claims;

- The decoded Dezi token claims can be found in the `dezi_claims` field.
- The original Dezi token is available in the `dezi_token` field.

The `id_token` can later be used to acquire GF Authentication access tokens.

```json
{
  "at_hash": "aEW-FO1Kv6b--LGpu707uA",
  "aud": [
    "local"
  ],
  "auth_time": 1763560632,
  "azp": "local",
  "c_hash": "zUQ0iEUjJo_U7tVg2_gy6Q",
  "client_id": "local",
  "dezi_claims": {
    "abonnee_naam": "Zorgaanbieder",
    "abonnee_nummer": "123456789",
    "achternaam": "Zorgmedewerker",
    "dezi_nummer": "123456789",
    "loa_dezi": "http://eidas.europe.eu/LoA/high",
    "rol_code": "01.000",
    "rol_code_bron": "https://auth.dezi.nl/revocatie/058d13ce-9b33-41b4-955f-f22a154b8a2d",
    "rol_naam": "Arts",
    "verklaring_id": "058d13ce-9b33-41b4-955f-f22a154b8a2d",
    "voorletters": "A.B.",
    "voorvoegsel": ""
  },
  "dezi_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJhYm9ubmVlX25hYW0iOiJab3JnYWFuYmllZGVyIiwiYWJvbm5lZV9udW1tZXIiOiIxMjM0NTY3ODkiLCJhY2h0ZXJuYWFtIjoiWm9yZ21lZGV3ZXJrZXIiLCJkZXppX251bW1lciI6IjEyMzQ1Njc4OSIsImxvYV9kZXppIjoiaHR0cDovL2VpZGFzLmV1cm9wZS5ldS9Mb0EvaGlnaCIsInJvbF9jb2RlIjoiMDEuMDAwIiwicm9sX2NvZGVfYnJvbiI6Imh0dHBzOi8vYXV0aC5kZXppLm5sL3Jldm9jYXRpZS8wNThkMTNjZS05YjMzLTQxYjQtOTU1Zi1mMjJhMTU0YjhhMmQiLCJyb2xfbmFhbSI6IkFydHMiLCJ2ZXJrbGFyaW5nX2lkIjoiMDU4ZDEzY2UtOWIzMy00MWI0LTk1NWYtZjIyYTE1NGI4YTJkIiwidm9vcmxldHRlcnMiOiJBLkIuIiwidm9vcnZvZWdzZWwiOiIifQ.TyIT6yJ7lJK1LyDa_48XgAMC3-xD_QtFDs3Pf1B2hNTPJSVG232j18VS8QOVyqv7d1lcPj4A6tp_39mA5I2azc-U-kuRgVV1-fAKw9ARByO_WAiNR3SFKDqYtfBMSy-Ry4ge0ZOpCxZQ5md40OqiqdQ063We5qbuNKDWRMhlqldfkutCduLvAS7F2xwHg08IQGXty95o2S1jellwbXy-k6cR_0H0Zwo3XqaJpgaqeVacWKhIvlxDNtNvzFZSzI8ndUSpXe-kvELkneP3mer2-ITbR07xq0O7IPdStSDtCdAYX2DuHoRjxZloVZvdiMncKgj8MByuWIoEH9qWAhyu3A",
  "exp": 1763564252,
  "family_name": "Zorgmedewerker",
  "given_name": "A.B.",
  "iat": 1763560632,
  "iss": "http://localhost:8080/auth",
  "name": "A.B. Zorgmedewerker",
  "sub": "123456789"
}
```

## Authorization

This chapter describes how to use the Knooppunt to support in making authorization decisions using its policy decision point.

The PDP is a single endpoint that requires the following input:

- A valid client qualification
- Information about the subject
- HTTP request properties
- Contextual information

The endpoint can be reached on the internal port of Knooppunt.

````http request
POST http://someaddress:8081/pdp/v1/data/knooppunt/authz
Content-Type: application/json
````

After parsing this data the PDP will check the input against two policies.

- Conformance to a capability statement
- Conformance to a rego policy 

The client qualification is used to determine which policy is applied. Knooppunt currently ships with the following policies:

- `bgz`
- `mcsd_update`
- `mcsd_query`

An example requests looks like this:

```json
{
  "input": {
    "subject": {
      "properties": {
        "subject_id": "000095254",
        "subject_role": "01.015",
        "subject_organization_id": "00000666",
        "subject_facility_type": "Z3",
        "client_qualifications": ["bgz"]
      }
    },
    "request": {
      "method": "GET",
      "protocol": "HTTP/1.0",
      "path": "/Patient?"
    },
    "context": {
      "data_holder_organization_id": "00000659",
      "data_holder_facility_type": "Z3"
    }
  }
}
```

The file [pdp.http](/docs/test-scripts/pdp.http) in the repository contains additional examples.

[The type declaration](/component/pdp/shared.go) `PDPInput` lists the full range of supported options.

### Integration a Policy Information Point

To come to a policy decision the PDP might need additional information from a policy information point.

Read our [configuration guide](/docs/CONFIGURATION.md) to see the options for configuring this endpoint.

Currently, this method is used to exchange a patientID for a BSN by looking up a patient record. 

### Answering _de gesloten vraag_ using the PDP

Some policies like `bgz` will attempt to answer _de Mitz gesloten vraag_.

For this to work you will need to configure the Mitz module of Knooppunt and integrate a policy information point (see above).
