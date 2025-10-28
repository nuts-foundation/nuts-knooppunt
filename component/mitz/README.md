# MITZ Connector Component

The MITZ Connector component enables integration with the Dutch national consent management system for healthcare. It
handles consent subscription management and authorization queries.

## Overview

MITZ is a central consent management system in the Netherlands that allows patients to manage their healthcare data
sharing preferences. This component provides:

1. **Consent Subscription Management** - Subscribe to consent notifications for specific patient-provider-category
   combinations
2. **Consent Authorization Queries** - Check if a healthcare provider has consent to access patient data
3. **Notification Handling** - Receive and process consent change notifications from MITZ

## Architecture

The component acts as a proxy between your application and the MITZ FHIR endpoints, handling:

- mTLS authentication with client certificates
- FHIR Subscription resource creation and management
- XACML authorization decision queries
- Configurable subscription notification endpoint

## Configuration

Configure the MITZ component in `knooppunt.yml`:

```yaml
mitz:
  # MITZ base URL (paths are hardcoded in code)
  mitzbase: "https://tst-api.mijn-mitz.nl"

  # Endpoint URL where MITZ will send consent change notifications
  notifyendpoint: "https://your-app.example.com/mitz/notify"

  # Optional: Gateway and source system OIDs (added as extensions to subscriptions)
  gatewaysystem: "urn:oid:2.16.840.1.113883.2.4.6.6.1"
  sourcesystem: "urn:oid:2.16.840.1.113883.2.4.6.6.90000017"

  # mTLS client certificate configuration
  tlscertfile: "/path/to/client-cert.p12"
  tlskeypassword: "your-certificate-password"
  tlscafile: "/path/to/ca-cert.pem"
```


### Endpoint Paths

The component uses these hardcoded paths relative to `mitzbase`:

- Subscription endpoint: `/abonnementen/fhir`
- Consent check endpoint: `/geslotenautorisatievraag/xacml3`
-

## Prerequisites

- vendor needs to have their certificate whitelisted by the test mitz team (
  see [mTLS Authentication](#mTLS-Authentication))
    - to circumvent this bureaucratic issue, a proxy will be setup through which all connections from local knoppunts
      will be routed. mTLS will
      be handled on that proxy with certificate of Rein.
- notify endpoint needs to be whitelisted by the test mitz team
    - to circumvent this bureaucratic issue, a proxy will be setup that'll be able to accept these notifications

## Subscription Endpoint Configuration

The component requires a configured notification endpoint where MITZ will send consent change notifications:

1. Set the `notify_endpoint` in your configuration (`knooppunt.yml`)
2. When a subscription is created without an explicit `channel.endpoint`, the configured value is used
3. If the subscription request includes an explicit `channel.endpoint`, that value takes precedence
4. Ensure the endpoint is whitelisted with your MITZ provider so they can reach it

This ensures your application receives all consent notifications from MITZ without additional discovery overhead.

## mTLS Authentication

MITZ requires mutual TLS authentication. Configure your client certificate:

**Option 1: PKCS#12 format (.p12/.pfx)**

```yaml
mitz:
  tlscertfile: "/path/to/client-cert.p12"
  tlskeypassword: "certificate-password"
```

**Option 2: PEM format (separate files)**

```yaml
mitz:
  tlscertfile: "/path/to/client-cert.pem"
  tlskeyfile: "/path/to/client-key.pem"
  tlskeypassword: "key-password"  # if key is encrypted
```

**Server Certificate Verification**:

```yaml
mitz:
  tlscafile: "/path/to/ca-cert.pem"
```

## Subscription Validation

The component validates subscriptions against MITZ requirements:

✅ **Valid**:

- Status is `"requested"`
- Reason is `"OTV"`
- Criteria starts with `Consent?_query=otv&`
- Criteria contains `patientid=`, `providerid=`, and `providertype=` parameters
- Channel type is `"rest-hook"`
- Extensions are limited to: Patient.birthDate, GatewaySystem, SourceSystem

❌ **Invalid**: Subscription will be rejected with a FHIR OperationOutcome error

## Error Handling

The component handles MITZ errors and translates them to FHIR OperationOutcome responses:

| MITZ Status | Error Type   | Description                               |
|-------------|--------------|-------------------------------------------|
| 400         | Invalid      | Resource doesn't meet FHIR specifications |
| 401         | Security     | Not authorized to create subscription     |
| 404         | NotFound     | MITZ endpoint not found                   |
| 422         | BusinessRule | MITZ business rules not met               |
| 429         | Throttled    | Too many requests                         |

## Integration with Other Components

The MITZ component is independently initialized and does not require other components:

```go
// In cmd/start.go
mitzComponent, err := mitz.New(config.MITZ)
```

The MITZ configuration is independent - you only need to provide the MITZ endpoint details and the notification callback
URL via configuration.

## Dependencies

- `github.com/SanteonNL/go-fhir-client` - FHIR client library
- `github.com/zorgbijjou/golang-fhir-models` - FHIR data models

## See Also

- [mCSD Component](../mcsd/README.md)
- [XACML Library](./xacml/README.md)
