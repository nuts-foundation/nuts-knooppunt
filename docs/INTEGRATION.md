# Knooppunt Integration Guide

This document describes how to integrate with the Knooppunt.

## Table of Contents

- [Addressing](#addressing)
- [Localization (NVI)](#nvi)
- [Online Consent (MITZ)](#consent-mitz)
- [Authentication](#authentication)
- [Authorization](#authorization)
  - [Prerequisites](#prerequisites)
  - [Evaluation](#evaluation)
  - [Explicit consent using MITZ](#explicit-consent-using-mitz-de-gesloten-vraag)
  - [Policy Information Point](#policy-information-point)
  - [Security Considerations](#security-considerations)

---

## Addressing

This chapter describes how to integrate with the addressing generic function of the Knooppunt,
based on the mCSD (Mobile Care Services Discovery) profile. It provides the following:

- Synchronization from remote mCSD Administration Directories to your local mCSD Query Directory, so it can be used to
  find organizations, endpoints, etc.
- Optional: an embedded mCSD Administration Directory web application to manage your local mCSD Administration
  Directory.

### Pre-requisites

You need to provide an mCSD Administration Directory, which is typically:

- A FHIR façade over an existing database or API, or
- a FHIR server (e.g. HAPI FHIR) in which mCSD resources are managed, either:
    - manually, e.g. using the embedded mCSD Admin web application (configure `mcsdadmin.fhirbaseurl`),
    - synchronized from another source in some way.

You also need to provide a FHIR server as mCSD Query Directory, to which mCSD resources are synchronized, e.g. HAPI
FHIR.

Then, configure:

- the Root Administration Directory to synchronize from (`mcsd.admin.<key>.fhirbaseurl`),
- the local Query Directory to synchronize to (`mcsd.query.fhirbaseurl`), and
- (optional) directories to exclude from synchronization (`mcsd.adminexclude`), which is useful to prevent
  self-referencing loops when your own query directory appears as a discovered Endpoint.

### Triggering synchronization

To synchronize remote mCSD Directories to your local query directory, use the following endpoint to trigger a
synchronization:

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

The Knooppunt contains a web-application to manually manage the mCSD Administration Directory entries (e.g. create
organizations and endpoints).

You can find the mCSD Admin application at:

```http
http://localhost:8080/mcsdadmin
```

## NVI

This chapter describes how to integrate with the NVI (Nederlandse VerwijsIndex) service using the Knooppunt.

The Knooppunt handles BSN pseudonymization transparently: BSNs in `subject.identifier` and search/delete parameters are
converted to NVI transport tokens before forwarding to NVI. You always work with plain BSNs on the Knooppunt side.

### Prerequisites

Before you can use the NVI integration, you need to request the following from iRealisatie:

1. **UZI/mTLS certificate** — used to authenticate to the MinVWS OAuth2 token endpoint. iRealisatie issues this
   certificate for the proeftuin environment.
2. **Test URA number** — a fake URA number assigned to your organization for use in the proeftuin environment.

The test URA must be included in every `List` resource you register, as the `nl-gf-localization-custodian` extension:

```json
{
  "resourceType": "List",
  "extension": [
    {
      "url": "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian",
      "valueReference": {
        "identifier": {
          "system": "http://fhir.nl/fhir/NamingSystem/ura",
          "value": "<your-test-ura>"
        }
      }
    }
  ],
  "..."
}
```

This extension identifies your organization as the custodian of the registration in the NVI.

### Configuration

Three sections in `knooppunt.yml` are required:

```yaml
# NVI service endpoint and audience (URA of the NVI service provider)
nvi:
  baseurl: "https://nvi.proeftuin.gf.irealisatie.nl/v1-poc/fhir"
  audience: "90000901"

# mTLS client certificate used to authenticate to the MinVWS OAuth2 token endpoint
authn:
  minvws:
    tlscertfile: "/path/to/uzi-certificate.pfx"
    tlskeypassword: "<pfx-password>"
    tokenendpoint: "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"

# Pseudonymization service used to convert BSNs to NVI transport tokens
pseudo:
  prsurl: "https://pseudoniemendienst.proeftuin.gf.irealisatie.nl"
```

| Property                      | Description                                                              |
|-------------------------------|--------------------------------------------------------------------------|
| `nvi.baseurl`                 | Base URL of the NVI FHIR endpoint                                        |
| `nvi.audience`                | URA of the NVI service provider, used as the pseudonymization audience   |
| `authn.minvws.tlscertfile`    | Path to the UZI/mTLS certificate (PFX or PEM) for OAuth2 authentication  |
| `authn.minvws.tlskeypassword` | Password for the PFX certificate (omit when using separate PEM key file) |
| `authn.minvws.tokenendpoint`  | OAuth2 token endpoint of the MinVWS authorization server                 |
| `pseudo.prsurl`               | Base URL of the pseudonymization service (PRS)                           |

### Registering a List (via Bundle)

To register a `List` resource wrapped in a transaction `Bundle` (see for example: [nvi.http](/docs/test-scripts/nvi.http)):

```http
POST http://localhost:8081/nvi
Content-Type: application/fhir+json
```

The bundle must contain a `List` resource. The `subject.identifier` BSN is pseudonymized before forwarding.

### Registering a List directly

To register a `List` resource directly without wrapping it in a Bundle (see for example: [nvi.http](/docs/test-scripts/nvi.http)):

```http
POST http://localhost:8081/nvi/List
Content-Type: application/fhir+json
```

### Reading a List by ID

```http
GET http://localhost:8081/nvi/List/{id}
```

### Searching for List resources

```http
GET  http://localhost:8081/nvi/List?<params>
POST http://localhost:8081/nvi/List/_search
Content-Type: application/x-www-form-urlencoded
```

At least one of the following parameters is required:

| Parameter            | Description                                  |
|----------------------|----------------------------------------------|
| `patient:identifier` | Patient BSN (`<system>\|<value>`)            |
| `subject:identifier` | Subject BSN (`<system>\|<value>`)            |
| `source:identifier`  | Source device identifier (not pseudonymized) |

Additional optional parameters:

| Parameter | Description                                        |
|-----------|----------------------------------------------------|
| `code`    | List category code (`http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-data-categories-cs\|<value>`)           |

BSN values in `patient:identifier` and `subject:identifier` are pseudonymized before forwarding to NVI.

### Deleting a List by ID

```http
DELETE http://localhost:8081/nvi/List/{id}
```

Returns `204 No Content` on success.

### Deleting List resources by parameters

```http
DELETE http://localhost:8081/nvi/List?<params>
```

At least one of the following parameters is required:

| Parameter            | Description                                  |
|----------------------|----------------------------------------------|
| `patient:identifier` | Patient BSN (`<system>\|<value>`)            |
| `subject:identifier` | Subject BSN (`<system>\|<value>`)            |
| `source:identifier`  | Source device identifier (not pseudonymized) |

Additional optional parameters:

| Parameter | Description                                        |
|-----------|----------------------------------------------------|
| `code`    | List category code (`http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-data-categories-cs\|<value>`)           |

BSN values in `patient:identifier` and `subject:identifier` are pseudonymized before forwarding to NVI.
Returns `204 No Content` on success.

The file [nvi.http](/docs/test-scripts/nvi.http) in the repository contains additional examples.


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

**Recommendation**: Always configure `notify_endpoint` to ensure subscriptions work without requiring clients to specify
endpoints.

### Subscription Behavior

1. **Validation**: Knooppunt validates the subscription meets MITZ requirements
2. **Extension Addition**: Automatically adds gateway and source system OIDs
3. **Endpoint Setting**: Uses configured `notify_endpoint` if no endpoint provided in request (
   see [Notification Endpoint Configuration](#notification-endpoint-configuration))
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

- Perform access token request at remote EHRs for outbound data exchanges (Nuts node).
    - If user is involved: take the attestation ("verklaring") the Dezi OIDC UserInfo object, to include in the access token request
- Validate the organization credentials and end-user credential (Dezi) for inbound data exchanges (Nuts node).
    - Note: make sure the Nuts node trusts the right Dezi JKU using the `vcr.dezi.allowedjku` property. See the [Nuts configuration guide](https://nuts-node.readthedocs.io/en/project-gf/pages/deployment/configuration.html#server-options) for more information.

For more information on the Knooppunt's authentication endpoints, see
the [Nuts node API reference](https://nuts-node.readthedocs.io/en/project-gf/pages/integrating/api.html).

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
  "user_id": "87654321",
  "user_initials": "J.",
  "user_surname_prefix": "van der",
  "user_surname": "Broek",
  "user_role": "01.041"
}
```

Note that the returned fields depend on the Nuts Access Policy that was loaded in the Knooppunt.

## Authorization

This chapter describes how to use the Knooppunt to support in making authorization decisions for data exchanges using its policy decision
point (PDP). The PDP is called by the Policy Enforcement Point (PEP) on every inbound request.
The PEP passes the [token introspection result](#verifying-access-tokens) together with details about the incoming
request to the PDP, which then returns an allow/deny decision.

The supported policies are embedded, and can be found in `/component/pdp/policies`.

### Prerequisites

- **Policy enforcement point (PEP)**: a reverse proxy (e.g. NGINX or HAProxy) that introspects incoming access tokens and calls the PDP. A reference NGINX implementation is available in the [/pep](/pep) directory.
- **Policy information point (PIP)**: a FHIR R4 REST-compatible API for:
  - policies that use implicit consent (e.g. eOverdracht)
  - policies that use explicit consent from Mitz, which require looking up the patient BSN from the FHIR resource ID.
- **MITZ**: the MITZ module must be configured for policies that require explicit patient consent (implemented through MITZ' _gesloten vraag_)

### Evaluation

The PDP evaluates the request against the policies associated with the provided scopes. The checks performed depend
on the policy; examples include FHIR Capability Statement conformance, required search parameters, explicit patient
consent, and implicit consent (_veronderstelde toestemming_).

The endpoint can be reached on the internal port (`:8081`) of Knooppunt on `/pdp/v1/data/knooppunt/authz`.

#### PDP Request

The request must be in JSON format, and can contain the following properties:

| Field | Required | Description | Example |
|---|---|---|---|
| `subject.scope` | Required | Space-separated OAuth scopes that determine which policies are evaluated | `"bgz"` |
| `subject.client_id` | Optional | Client identifier from the access token | `"https://example.com/oauth2"` |
| `subject.organization_ura` | Required | URA of the requesting organization | `"00000666"` |
| `subject.organization_name` | Optional | Name of the requesting organization | `"Hospital West"` |
| `subject.organization_facility_type` | Optional | Facility type code of the requesting organization | `"Z3"` |
| `subject.user_id` | Optional | Identifier of the end user; omit if no user is involved | `"000095254"` |
| `subject.user_role` | Optional | Role code of the end user; omit if no user is involved | `"01.015"` |
| `request.method` | Required | HTTP method | `"GET"` |
| `request.protocol` | Required | HTTP protocol version | `"HTTP/1.0"` |
| `request.path` | Required | Request path without query string | `"/Patient"` |
| `request.query_params` | Optional | Query parameter names mapped to arrays of values | `{"_include": ["Patient:general-practitioner"]}` |
| `context.connection_type_code` | Required | Type of connection; use `hl7-fhir-rest` for FHIR REST APIs | `"hl7-fhir-rest"` |
| `context.data_holder_organization_id` | Required | URA of the organization that holds the data being requested | `"00000659"` |
| `context.data_holder_facility_type` | Optional | Facility type code of the data-holding organization | `"Z3"` |
| `context.patient_bsn` | Optional | BSN of the patient; may be omitted if the PIP is configured and the patient ID can be derived from the request path | `"900186021"` |

The `subject` can be set directly to the claims from the token introspection response (see [Verifying access tokens](#verifying-access-tokens)) — no transformation needed.

An example requests looks like this:

```http request
POST http://localhost:8081/pdp/v1/data/knooppunt/authz
Content-Type: application/json

{
  "input": {
    "subject": {
      "user_id": "000095254",
      "user_role": "01.015",
      "organization_ura": "00000666",
      "organization_facility_type": "Z3",
      "scope": "bgz"
    },
    "request": {
      "method": "GET",
      "protocol": "HTTP/1.0",
      "path": "/Patient",
      "query_params": {
        "_include": ["Patient:general-practitioner"]
      }
    },
    "context": {
      "data_holder_organization_id": "00000659",
      "data_holder_facility_type": "Z3",
      "connection_type_code": "hl7-fhir-rest"
    }
  }
}
```

The file [pdp.http](/docs/test-scripts/pdp.http) in the repository contains additional examples.

#### PDP Response

The response is a JSON object. The PEP must use the root `allow` field to allow or deny access.

The `policies` field is informational (e.g. for logging) and shows the result per policy.

| Field | Description |
|---|---|
| `allow` | `true` if the request is allowed, `false` otherwise |
| `policies` | Object containing the result per evaluated policy |
| `policies.<name>.allow` | Whether this policy allowed the request |
| `policies.<name>.reasons` | Array of reasons explaining the decision |
| `policies.<name>.reasons[].code` | Reason code (e.g. `not_allowed`, `pip_error`, `info`) |
| `policies.<name>.reasons[].description` | Human-readable explanation |

Example response:

```json
{
  "allow": true,
  "policies": {
    "bgz": {
      "allow": true,
      "reasons": [
        {
          "code": "info",
          "description": "MITZ consent granted"
        }
      ]
    }
  }
}
```

### Explicit consent using MITZ (_de gesloten vraag_)

Some policies like `bgz` query MITZ (_de gesloten vraag_) to verify whether the patient has given consent for this exchange.

For this to work you will need to configure the Mitz module of Knooppunt and integrate a policy information point (see
above). Otherwise, access will be rejected.

### Policy Information Point

To come to a policy decision the PDP might need additional information from a policy information point (PIP).

Read our [configuration guide](/docs/CONFIGURATION.md) to see the options for configuring this endpoint.

The PIP is a FHIR R4 REST-compatible API used for two purposes:

1. **Patient BSN lookup**: to find a patient's BSN given their FHIR resource ID patient.
2. **Implied consent lookup**: to find consents giving access to the requested FHIR resource. 

#### Patient BSN Lookup

The PDP performs the following call to resolve a patient resource ID to a BSN:

```http
GET /Patient/{id}?_elements=identifier
```

The PDP looks for an identifier with system `http://fhir.nl/fhir/NamingSystem/bsn` and uses its value as the BSN.

Example response:

```json
{
  "resourceType": "Patient",
  "id": "3E439979-017F-40AA-594D-EBCF880FFD97",
  "identifier": [
    {
      "system": "http://fhir.nl/fhir/NamingSystem/bsn",
      "value": "176286603"
    }
  ]
}
```

#### Implied consent lookup

Some policies use **implied consent** to control access to specific FHIR resources. Rather than
querying MITZ for population-level consent, the PDP searches the EHR's PIP for `Consent` resources that
explicitly permit or deny access to an individual resource, using the `data` search parameter:

```http
GET /Consent?data={ResourceType}/{resourceId}
```

This returns all Consent resources whose `provision.data.reference` refers to the given resource.
The PDP follows pagination links automatically, so the PIP may return results across multiple pages.

This is triggered whenever the PDP input includes a resource ID and type with connection type `hl7-fhir-rest`.
The `eoverdracht_sender` policy is an example that relies on this mechanism: it checks that a Consent with
scope `eoverdracht` permits the requesting organization to access the specific resource.

##### Resource Requirements

For a Consent resource to influence the policy decision, it must satisfy all of the following conditions:

| Field | Required value |
|---|---|
| `status` | `active` |
| `scope.coding[0].code` | The policy scope (e.g. `eoverdracht`) |
| `scope.coding[0].system` | `http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-consent-scope` |
| `organization[].identifier.system` | `http://fhir.nl/fhir/NamingSystem/ura` |
| `organization[].identifier.value` | URA of the data-holding organization (`context.data_holder_organization_id`) |
| `provision.type` | `permit` or `deny` |
| `provision.action[].coding` | Must include code `access` from system `http://terminology.hl7.org/CodeSystem/consentaction` |
| `provision.actor[].reference.identifier.system` | `http://fhir.nl/fhir/NamingSystem/ura` |
| `provision.actor[].reference.identifier.value` | URA of the requesting organization (`subject.organization.ura`) |
| `provision.data[].reference.reference` | Reference to the governed resource (e.g. `Task/12AF22F3-...`) |

Only Consent resources where **both** the data holder's URA matches `context.data_holder_organization_id`
**and** the actor's URA matches `subject.organization.ura` are applied to the policy decision.

##### Ruling Precedence

When multiple matching Consent resources exist for the same scope:
- **`deny` supersedes `permit`**: if any Consent has `provision.type = deny` for a given scope, that scope is
  denied even if a `permit` Consent with equal specificity also exists.

##### Example

The following Consent permits organization `00000040` to access a specific `Task` and `Composition` resource held by
organization `00000030` under the `eoverdracht` scope:

```json
{
  "resourceType": "Consent",
  "status": "active",
  "scope": {
    "coding": [
      {
        "system": "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-consent-scope",
        "code": "eoverdracht"
      }
    ]
  },
  "organization": [
    {
      "identifier": {
        "system": "http://fhir.nl/fhir/NamingSystem/ura",
        "value": "00000030"
      }
    }
  ],
  "provision": {
    "type": "permit",
    "action": [
      {
        "coding": [
          {
            "system": "http://terminology.hl7.org/CodeSystem/consentaction",
            "code": "access"
          }
        ]
      }
    ],
    "actor": [
      {
        "reference": {
          "identifier": {
            "system": "http://fhir.nl/fhir/NamingSystem/ura",
            "value": "00000040"
          }
        }
      }
    ],
    "data": [
      {
        "reference": {
          "reference": "Task/12AF22F3-2DE5-47E1-B3CB-B053C8621F84",
          "type": "Task"
        }
      },
      {
        "reference": {
          "reference": "Composition/21ef0423-018b-40e7-adfd-7f4317f01c8f",
          "type": "Composition"
        }
      }
    ]
  }
}
```

Note that some fields required by FHIR R4 but ignored by the PDP are omitted for brevity.

When the PDP evaluates a request for `Task/12AF22F3-2DE5-47E1-B3CB-B053C8621F84`, it calls
`GET /Consent?data=Task/12AF22F3-2DE5-47E1-B3CB-B053C8621F84` on the PIP, finds this Consent,
and uses it to evaluate the `eoverdracht_sender` policy, which then allows the request.

### Security Considerations

- **Use token introspection for identity claims**: Never use identity claims provided by the client directly as
  `subject` input. Always derive identity claims from the token introspection response.
- **Deny on error**: If the PDP is unreachable or returns an error, the PEP must deny access. Never default to allow.
- **PIP access**: The PIP should only be accessible from the Knooppunt, not from external parties, as it exposes
  patient data used for authorization decisions.
- **Minimize PIP data exposure**: The PIP should honor the `_elements` parameter so that only the requested fields
  are returned, minimizing the patient data sent to the PDP.

