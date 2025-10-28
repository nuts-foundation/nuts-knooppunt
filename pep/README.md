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

# Test with mock token (format: bearer-<ura>-<role>-<uzi>)
curl -H "Authorization: Bearer bearer-00000020-practitioner-123456789" \
  http://localhost:9080/fhir/Patient/patient-123
```

**Endpoints:**
- PEP: `http://localhost:9080`
- PDP: `http://localhost:8081/v1/data/knooppunt/authz` (internal API)

## How It Works

1. Extract bearer token from `Authorization` header
2. Introspect token via `/_introspect` endpoint (mock NJS function for testing)
3. Extract FHIR context (resource type, ID) from request URI
4. Build OPA request and call Knooppunt PDP
5. Enforce decision: allow (200) or deny (403)

## Configuration

Environment variables in `docker-compose.yml`:

```yaml
FHIR_BACKEND_HOST=hapi-fhir   # FHIR server
FHIR_BACKEND_PORT=7050
KNOOPPUNT_PDP_HOST=knooppunt  # PDP endpoint
KNOOPPUNT_PDP_PORT=8081       # Internal API only
```

## OPA Request Format

The PEP sends snake_case formatted requests to the PDP:

```json
POST /v1/data/knooppunt/authz

{
  "input": {
    "method": "GET",
    "path": ["fhir", "Patient", "patient-123"],
    "subject_type": "practitioner",
    "subject_id": "mock-user",
    "subject_role": "practitioner",
    "subject_uzi": "123456789",
    "organization_ura": "00000020",
    "resource_type": "Patient",
    "resource_id": "patient-123",
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
