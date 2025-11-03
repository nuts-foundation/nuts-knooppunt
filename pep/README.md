# Policy Enforcement Point (PEP)

NGINX-based reference implementation that enforces authorization decisions from Knooppunt's PDP.

## Architecture

```
┌─────────────────┐
│ External Client │
└────────┬────────┘
         │ GET /fhir/Patient/123
         │ Authorization: Bearer <token>
         ▼
┌────────────────────────────────────────┐
│           PEP (NGINX)                  │
│  ┌──────────────────────────────────┐  │
│  │ 1. Extract token                 │  │
│  │ 2. Introspect (mock for testing) │  │
│  │ 3. Build OPA request             │  │
│  │ 4. Call PDP for decision         │  │
│  │ 5. Enforce (allow/deny)          │  │
│  └──────────────────────────────────┘  │
└────────┬─────────────────┬─────────────┘
         │                 │
         │ POST /v1/data/  │
         │  knooppunt/authz│
         ▼                 │
┌────────────────────┐     │
│ Knooppunt PDP      │     │
│ (port 8081)        │     │
│ - Validates input  │     │
│ - Returns allow/deny     │
└────────────────────┘     │
                           │ (if allowed)
                           ▼
                  ┌────────────────┐
                  │  FHIR Server   │
                  └────────────────┘
```

## Quick Start

```bash
# Start PEP with rest of stack
docker compose --profile pep up -d

# Test with mock token (format: bearer-<ura>-<uzi_role>-<practitioner_id>-<bsn>)
curl -H "Authorization: Bearer bearer-00000020-01.015-123456789-900186021" \
  http://localhost:9080/fhir/Patient/patient-123
```

**Endpoints:**
- PEP: `http://localhost:9080`
- PDP: `http://localhost:8081/pdp/v1/data/knooppunt/authz` (internal API)

## How It Works

1. Extract bearer token from `Authorization` header
2. Introspect token via `/_introspect` endpoint (mock NJS function for testing)
3. Extract FHIR context (resource type, ID) from request URI
4. Build OPA request and call Knooppunt PDP
5. Enforce decision: allow (200) or deny (403)

## Configuration

Environment variables in `docker-compose.yml`:

```yaml
# Backend connections
FHIR_BACKEND_HOST=hapi-fhir   # FHIR server
FHIR_BACKEND_PORT=7050        # HAPI FHIR default port, as 8080 is used by the knooppunt
KNOOPPUNT_PDP_HOST=knooppunt  # PDP endpoint
KNOOPPUNT_PDP_PORT=8081       # Internal API only

# Data holder configuration (organization where data is stored)
DATA_HOLDER_ORGANIZATION_URA=00000666
DATA_HOLDER_FACILITY_TYPE=Z3

# Request configuration
REQUESTING_FACILITY_TYPE=Z3
PURPOSE_OF_USE=treatment
```

## OPA Request Format

The PEP sends requests with clear field names matching XACML/Mitz terminology:

```json
POST /pdp/v1/data/knooppunt/authz

{
  "input": {
    "method": "GET",
    "path": ["fhir", "Patient", "patient-123"],

    // REQUESTING PARTY (who is asking for data)
    "requesting_organization_ura": "00000020",
    "requesting_uzi_role_code": "01.015",
    "requesting_practitioner_identifier": "123456789",
    "requesting_facility_type": "Z3",

    // DATA HOLDER PARTY (who has the data)
    "data_holder_organization_ura": "00000666",
    "data_holder_facility_type": "Z3",

    // PATIENT/RESOURCE CONTEXT
    "patient_bsn": "900186021",
    "resource_type": "Patient",
    "resource_id": "patient-123",

    // PURPOSE OF USE
    "purpose_of_use": "treatment"
  }
}
```

**Expected response:**
```json
{
  "result": {
    "allow": true
  }
}
```

## Production Deployment

Replace the mock token introspection with real OAuth in `nginx/conf.d/knooppunt.conf`. Something like this:

```nginx
# Change from:
location = /_introspect {
    internal;
    js_content authorize.mockIntrospect;
}

# To:
location = /_introspect {
    internal;
    proxy_pass http://nuts_node/internal/auth/v2/accesstoken/introspect;
    proxy_method POST;
    proxy_set_header Content-Type "application/x-www-form-urlencoded";
    proxy_pass_request_body on;
}
```

No changes needed to `authorize.js`.

## Implementation Details

- **nginx/nginx.conf** - Main NGINX config (rate limiting, request size limits)
- **nginx/conf.d/knooppunt.conf** - PEP routes and upstreams
- **nginx/js/authorize.js** - Authorization logic (NJS)
- **nginx/js/authorize.test.js** - Unit tests for authorization logic

See inline code comments for details.

## Testing

```bash
cd pep/nginx/js
npm install
npm test
```
