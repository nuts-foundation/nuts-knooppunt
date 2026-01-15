# OPA Integration Summary

## What Was Implemented

Successfully integrated Open Policy Agent (OPA) as a policy agent in the PDP component with the following features:

### 1. Policy Bundles

Created 4 Rego policies for different authorization scopes:

- **mcsd_update.rego** - Authorizes MCSD update operations (create, update, delete) for organizations, locations, healthcare services, practitioners, and practitioner roles
- **mcsd_query.rego** - Authorizes MCSD query operations (read, search) for the same resources
- **bgz_patient.rego** - Authorizes patient access to their own health data with patient ID matching
- **bgz_professional.rego** - Authorizes healthcare professionals with valid roles (nurse, physician, specialist, GP, pharmacist) and purpose of use (treatment) to access patient data

### 2. Bundle Generation

Implemented a Go-based bundle generator (`component/pdp/opa/cmd/generate_bundles/main.go`) that:
- Reads all `.rego` files from the `policies/` directory
- Creates OPA-compatible tar.gz bundles with manifests
- Can be invoked via `go:generate` directive in `bundles.go`
- Manually creates tar archives to avoid OPA SDK bundle writer issues

### 3. OPA Service

Created `component/pdp/opa/service.go` with:
- Bundle loading from embedded tar.gz files
- Policy evaluation using OPA's Rego engine
- Structured result format with allow/deny and reasons
- Bundle serving capabilities for external OPA instances

### 4. PDP Integration

Modified the PDP component to:
- Initialize OPA service on startup
- Replace hardcoded switch-case logic with OPA policy evaluation
- Keep MitZ integration for bgz scopes (layered authorization)
- Serve policy bundles via HTTP endpoints

### 5. HTTP Endpoints

Added internal HTTP endpoints:
- `GET /pdp/bundles` - Lists all available policy bundles
- `GET /pdp/bundles/{scope}` - Downloads a specific policy bundle

### 6. Testing

Comprehensive test coverage in `component/pdp/opa/service_test.go`:
- Bundle loading tests
- Policy evaluation tests for all 4 scopes
- Positive and negative test cases
- Reasons extraction for denials

## Files Created/Modified

### Created Files:
- `component/pdp/opa/policies/mcsd_update.rego`
- `component/pdp/opa/policies/mcsd_query.rego`
- `component/pdp/opa/policies/bgz_patient.rego`
- `component/pdp/opa/policies/bgz_professional.rego`
- `component/pdp/opa/cmd/generate_bundles/main.go`
- `component/pdp/opa/bundles.go`
- `component/pdp/opa/service.go`
- `component/pdp/opa/service_test.go`
- `component/pdp/opa/bundles/` (generated bundles)
- `component/pdp/opa/README.md`
- `component/pdp/opa_eval.go`

### Modified Files:
- `component/pdp/component.go` - Added OPA initialization and bundle serving endpoints
- `component/pdp/shared.go` - Added OPA service field to Component struct

## Usage

### Generating Bundles

```bash
cd component/pdp/opa
go generate
```

Or:

```bash
cd component/pdp/opa
go run cmd/generate_bundles/main.go
```

### Running Tests

```bash
go test ./component/pdp/opa/...
```

### Accessing Bundles via HTTP

```bash
# List all bundles
curl http://localhost:8080/pdp/bundles

# Download a specific bundle
curl http://localhost:8080/pdp/bundles/mcsd_update -o mcsd_update.tar.gz
```

## How It Works

1. **Startup**: PDP component initializes OPA service, which loads all embedded policy bundles
2. **Request**: PDP receives authorization request with scope/qualification
3. **Capability Check**: Request is validated against FHIR capability statement
4. **OPA Evaluation**: Policy input is evaluated using OPA with the appropriate scope's bundle
5. **MitZ Check** (bgz scopes only): If OPA allows, MitZ policy is additionally checked
6. **Response**: Combined result is returned with allow/deny and reasons

## Future Enhancements

- Support for dynamic policy loading without redeployment
- Policy versioning and rollback
- Audit logging of policy decisions
- Integration with external policy management systems
- Policy testing framework with OPA's built-in test runner

