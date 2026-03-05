# HAPI FHIR Pseudonymization Interceptor

This interceptor provides automatic pseudonymization for BSN tokens in FHIR List resources. It converts BSN tokens to pseudonyms for storage and back to audience-specific tokens for retrieval.

It handles two identifier fields:
- `List.subject.as(Identifier)` — the patient, stored as a pseudonym
- `List.source.as(Identifier)` — the source device, stored as a direct reference (no pseudonymization)

## Overview

The interceptor handles the conversion between:
- **BSN Tokens** (transport format): `token-{audience}-{transformedBSN}-{nonce}`
- **Pseudonyms** (storage format): `ps-{audience}-{transformedBSN}`

All List resources are stored with pseudonyms internally, but clients interact using BSN tokens.

## Configuration

The following environment variables can be configured:

| Variable | Default | Description |
|----------|---------|-------------|
| `PSEUDO_BSN_SYSTEM` | `http://example.com/pseudoBSN` | System URL for pseudonyms (internal) |
| `BSN_TOKEN_SYSTEM` | `http://example.com/BSNToken` | System URL for BSN tokens (client-facing) |
| `NVI_AUDIENCE` | `nvi-1` | Default audience for pseudonym storage |

## HAPI FHIR :identifier Modifier Workaround

### The Problem

HAPI FHIR does not natively support the `:identifier` modifier for reference search parameters. This means searches like:
```
GET /List?subject:identifier=system|value
```
are not supported out of the box.

### Our Workaround

Instead of storing `List.subject` or `List.source` as an Identifier, we **convert them to References** with a specially formatted ID:

**What the client sends (POST — a Bundle containing a List):**
```json
{
  "resourceType": "Bundle",
  "type": "transaction",
  "entry": [{
    "resource": {
      "resourceType": "List",
      "subject": {
        "identifier": {
          "system": "http://example.com/BSNToken",
          "value": "token-hospital-abc123-def456"
        }
      },
      "source": {
        "identifier": {
          "system": "http://example.com/deviceSystem",
          "value": "device-id-xyz"
        }
      }
    },
    "request": { "method": "POST", "url": "List" }
  }]
}
```

**What gets stored internally:**
```json
{
  "resourceType": "List",
  "subject": {
    "reference": "http://example.com/pseudoBSN/Patient/ps-nvi-1-abc123"
  },
  "source": {
    "reference": "http://example.com/deviceSystem/Device/device-id-xyz"
  }
}
```

The pseudonym is embedded as a Patient reference ID, and the source identifier as a Device reference ID, allowing HAPI to perform efficient reference searches.

Note: For this to work, we need to enable external references with `hapi.fhir.allow_external_references: true`.

**Search Translation:**

When a client searches using:
```
GET /List?subject:identifier=http://example.com/BSNToken|token-hospital-abc123-def456
```

The interceptor:
1. Intercepts the search parameter
2. Extracts the BSN token from the identifier
3. Converts it to a pseudonym
4. Replaces the search parameter with a reference search:
   ```
   http://example.com/pseudoBSN/Patient/ps-nvi-1-abc123
   ```

For `source`, no pseudonymization happens — the identifier is passed through as a Device reference:
```
GET /List?source:identifier=http://example.com/deviceSystem|device-id-xyz
→ source=http://example.com/deviceSystem/Device/device-id-xyz
```

This happens transparently - the client thinks it's using `:identifier` modifier, but internally we convert it to a reference search that HAPI supports natively.

## Supported API Operations

All operations require `X-Requester-URA` header. This value needs to match the List extension URA.

### 1. POST Bundle (Create)

Creating a List is done by posting a FHIR transaction Bundle. The Bundle contains the List resource. The interceptor processes each List entry within the Bundle.

**Request:**
```http
POST /
X-Requester-URA: hospital
Content-Type: application/fhir+json

{
  "resourceType": "Bundle",
  "type": "transaction",
  "entry": [{
    "resource": {
      "resourceType": "List",
      "subject": {
        "identifier": {
          "system": "http://example.com/BSNToken",
          "value": "token-hospital-abc123-def456"
        }
      },
      "source": {
        "identifier": {
          "system": "http://example.com/deviceSystem",
          "value": "device-id-xyz"
        }
      },
      ...
    },
    "request": { "method": "POST", "url": "List" }
  }]
}
```

**Behavior:**
1. `List.subject` token is converted to a pseudonym and stored as a Patient reference
2. `List.source` identifier is stored as a Device reference (no pseudonymization)
3. Response returns the created resource with the subject token converted back to the requester's audience

**Response:**
```json
{
  "resourceType": "List",
  "id": "123",
  "subject": {
    "identifier": {
      "system": "http://example.com/BSNToken",
      "value": "token-hospital-xyz789-newNonce"
    }
  },
  ...
}
```

### 2. GET List/{id} (Read Single Resource)

**Request:**
```http
GET /List/123
X-Requester-URA: clinic-west
```

**Behavior:**
1. Interceptor retrieves the resource with pseudonym reference
2. Converts pseudonym to a token for the requester's audience (`clinic-west`)
3. Returns resource with audience-specific token

**Response:**
```json
{
  "resourceType": "List",
  "id": "123",
  "subject": {
    "identifier": {
      "system": "http://example.com/BSNToken",
      "value": "token-clinic-west-def789-nonce123"
    }
  },
  ...
}
```

**Without X-Requester-URA Header:**
```http
GET /List/123
```

**Behavior:** Returns `IllegalArgumentException`:
```
'X-Requester-URA' header is mandatory.
```

### 3. GET List?subject:identifier=... or List?patient:identifier=... (Search by Subject or Patient)

**Request:**
```http
GET /List?subject:identifier=http://example.com/BSNToken|token-hospital-abc123-def456
X-Requester-URA: clinic-west
```

**Behavior:**
1. Interceptor intercepts search
2. Extracts BSN token from the identifier parameter
3. Converts token to pseudonym
4. Replaces search parameter with reference: `subject=http://example.com/pseudoBSN/Patient/ps-nvi-1-abc123`
5. HAPI executes the reference search
6. Found resources are converted back to tokens for the requester's audience

**Response:**
```json
{
  "resourceType": "Bundle",
  "type": "searchset",
  "entry": [{
    "resource": {
      "resourceType": "List",
      "id": "123",
      "subject": {
        "identifier": {
          "system": "http://example.com/BSNToken",
          "value": "token-clinic-west-xyz789-nonce456"
        }
      },
      ...
    }
  }]
}
```

### 4. GET List?source:identifier=... (Search by Source)

**Request:**
```http
GET /List?source:identifier=http://example.com/deviceSystem|device-id-xyz
X-Requester-URA: clinic-west
```

**Behavior:**
1. Interceptor intercepts search
2. Extracts system and value from the identifier parameter
3. Replaces search parameter with a Device reference: `source=http://example.com/deviceSystem/Device/device-id-xyz`
4. HAPI executes the reference search
5. Found resources are converted back to tokens for the requester's audience

No pseudonymization is applied to the source — the identifier is passed through as-is.

### Search Requirements

At least one search parameter (`subject`, `patient`, or `source`) must be provided. If none is provided, returns `IllegalArgumentException`: "You have to search by 'patient' or 'subject' (patient) or 'source'."

## Architecture

### Components

1. **BsnUtil** - Handles token ↔ pseudonym conversions using XOR encoding
2. **PseudonymInterceptor** - HAPI FHIR interceptor with hooks:
   - `STORAGE_PRESTORAGE_RESOURCE_CREATED` - Converts tokens to pseudonyms before storage
   - `STORAGE_PRESHOW_RESOURCES` - Converts pseudonyms to tokens before presentation
   - `STORAGE_PRESEARCH_REGISTERED` - Converts search parameters from tokens/identifiers to references

### Data Flow

#### Storage (POST Bundle):
```
Client sends Bundle with List entries containing BSN tokens
    ↓
[Interceptor: STORAGE_PRESTORAGE_RESOURCE_CREATED]
    ↓
List.subject: Convert BSN token → Pseudonym, store as Patient reference
List.source:  Store identifier as Device reference (no pseudonymization)
    ↓
Database
```

#### Retrieval (GET):
```
Database
    ↓
Pseudonym Reference
    ↓
[Interceptor: STORAGE_PRESHOW_RESOURCES]
    ↓
Convert to Token (audience-specific, from X-Requester-URA)
    ↓
Client Token (BSNToken)
```

#### Search by subject/patient:
```
Client Search: ?subject:identifier=BSNToken|token-xxx
    ↓
[Interceptor: STORAGE_PRESEARCH_REGISTERED]
    ↓
Convert Token → Pseudonym
    ↓
Replace: ?subject={baseUrl}/Patient/{pseudonym}
    ↓
HAPI Search Engine
    ↓
Results with Pseudonyms
    ↓
[Interceptor: STORAGE_PRESHOW_RESOURCES]
    ↓
Convert Pseudonyms → Tokens (audience-specific)
    ↓
Client Results
```

#### Search by source:
```
Client Search: ?source:identifier=system|value
    ↓
[Interceptor: STORAGE_PRESEARCH_REGISTERED]
    ↓
Replace: ?source=system/Device/value
    ↓
HAPI Search Engine
    ↓
Results
    ↓
[Interceptor: STORAGE_PRESHOW_RESOURCES]
    ↓
Convert subject pseudonyms → tokens (audience-specific)
    ↓
Client Results
```

## Test

Run tests with:
```bash
mvn test
```


## Build & Run with docker

```bash
docker build . -t nvi
docker run -p 8080:8080 nvi
```

### Interact
```http request
POST /fhir/
GET /fhir/List/{id}
GET /fhir/List?subject:identifier=http://example.com/BSNToken|token-nvi-38bf96b43cbb92b830-e60a7ad0
GET /fhir/List?source:identifier=http://example.com/deviceSystem|device-id-xyz
POST /fhir/List/_search
```
