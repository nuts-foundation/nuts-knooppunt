# HAPI FHIR Pseudonymization Interceptor

This interceptor provides automatic pseudonymization for BSN tokens in FHIR DocumentReference resources (`DocumentReference.subject.as(Identifier)`). It converts BSN tokens to pseudonyms for storage and back to audience-specific tokens for retrieval.

## Overview

The interceptor handles the conversion between:
- **BSN Tokens** (transport format): `token-{audience}-{transformedBSN}-{nonce}`
- **Pseudonyms** (storage format): `ps-{audience}-{transformedBSN}`

All DocumentReference resources are stored with pseudonyms internally, but clients interact using BSN tokens.

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
GET /DocumentReference?subject:identifier=system|value
```
are not supported out of the box.

### Our Workaround

Instead of storing DocumentReference.subject as an Identifier, we **convert it to a Reference** with a specially formatted ID:

**What the client sends (POST):**
```json
{
  "resourceType": "DocumentReference",
  "subject": {
    "identifier": {
      "system": "http://example.com/BSNToken",
      "value": "token-hospital-abc123-def456"
    }
  }
}
```

**What gets stored internally:**
```json
{
  "resourceType": "DocumentReference",
  "subject": {
    "reference": "http://example.com/pseudoBSN/Patient/ps-nvi-1-abc123"
  }
}
```

The pseudonym is embedded as a Patient reference ID, allowing HAPI to perform efficient reference searches.

Note: For this to work, we need to enable external references with `hapi.fhir.allow_external_references: true`.

**Search Translation:**

When a client searches using:
```
GET /DocumentReference?subject:identifier=http://example.com/BSNToken|token-hospital-abc123-def456
```

The interceptor:
1. Intercepts the search parameter
2. Extracts the BSN token from the identifier
3. Converts it to a pseudonym 
4. Replaces the search parameter with a reference search :
   ```
   http://example.com/pseudoBSN/Patient/ps-nvi-1-abc123
   ```

This happens transparently - the client thinks it's using `:identifier` modifier, but internally we convert it to a reference search that HAPI supports natively.

## Supported API Operations

### 1. POST DocumentReference (Create)

#### With X-Requester-URA Header

**Request:**
```http
POST /DocumentReference
X-Requester-URA: hospital
Content-Type: application/fhir+json

{
  "resourceType": "DocumentReference",
  "subject": {
    "identifier": {
      "system": "http://example.com/BSNToken",
      "value": "token-hospital-abc123-def456"
    }
  },
  ...
}
```

**Behavior:**
1. Token is converted to pseudonym and stored as a reference
2. Resource is created successfully
3. Response returns the created resource with the token converted back to the requester's audience

**Response:**
```json
{
  "resourceType": "DocumentReference",
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

#### Without X-Requester-URA Header

**Request:**
```http
POST /DocumentReference
Content-Type: application/fhir+json

{
  "resourceType": "DocumentReference",
  "subject": {
    "identifier": {
      "system": "http://example.com/BSNToken",
      "value": "token-hospital-abc123-def456"
    }
  },
  ...
}
```

**Behavior:**
1. Token is converted to pseudonym and stored
2. Resource is created successfully
3. **Response is replaced with an OperationOutcome warning** (cannot return the token without knowing the audience)

**Response:**
```json
{
  "resourceType": "OperationOutcome",
  "issue": [{
    "severity": "warning",
    "code": "security",
    "details": {
      "text": "Resource was created (DocumentReference/123, see Location header), but can not be presented as no audience has been supplied. Do a GET with X-Requester-URA header to retrieve the Resource."
    }
  }]
}
```

The `Location` header contains the URL to retrieve the resource.

### 2. GET DocumentReference/{id} (Read Single Resource)

**Request:**
```http
GET /DocumentReference/123
X-Requester-URA: clinic-west
```

**Behavior:**
1. Interceptor retrieves the resource with pseudonym reference
2. Converts pseudonym to a token for the requester's audience (`clinic-west`)
3. Returns resource with audience-specific token

**Response:**
```json
{
  "resourceType": "DocumentReference",
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
GET /DocumentReference/123
```

**Behavior:** Returns `IllegalArgumentException`:
```
Resource can not be retrieved due to the fact there is no X-Requester-URA header present.
```

### 3. GET DocumentReference?subject:identifier=... or DocumentReference?patient:identifier=... (Search by Subject or Patient Identifier)

**Request:**
```http
GET /DocumentReference?subject:identifier=http://example.com/BSNToken|token-hospital-abc123-def456
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
      "resourceType": "DocumentReference",
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

### Search Requirements


- At least one search parameter (`subject` or `patient`) must be provided
- If neither is provided, returns `IllegalArgumentException`: "You have to search by 'patient' or 'subject' (patient)."

## Architecture

### Components

1. **BsnUtil** - Handles token ↔ pseudonym conversions using XOR encoding
2. **PseudonymInterceptor** - HAPI FHIR interceptor with hooks:
   - `STORAGE_PRESTORAGE_RESOURCE_CREATED` - Converts tokens to pseudonyms before storage
   - `STORAGE_PRESHOW_RESOURCES` - Converts pseudonyms to tokens before presentation
   - `STORAGE_PRESEARCH_REGISTERED` - Converts search parameters from tokens to pseudonyms
   - `SERVER_OUTGOING_RESPONSE` - Handles POST responses without audience header

### Data Flow

#### Storage (POST):
```
Client Token (BSNToken)
    ↓
[Interceptor: STORAGE_PRESTORAGE_RESOURCE_CREATED]
    ↓
Convert to Pseudonym
    ↓
Store as Reference: {baseUrl}/Patient/{pseudonym}
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
Convert to Token (audience-specific)
    ↓
Client Token (BSNToken)
```

#### Search:
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
POST /fhir/DocumentReference
GET /fhir/DocumentReference/{id}
GET /fhir/DocumentReference?subject:identifier=http://example.com/BSNToken|token-nvi-38bf96b43cbb92b830-e60a7ad0
POST /fhir/DocumentReference/_search
```
