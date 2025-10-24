# Configuration

The Nuts Knooppunt application supports configuration through YAML files and environment variables.

## Configuration Priority

Configuration is loaded in the following order (later sources override earlier ones):

1. Default values (hardcoded)
2. YAML configuration file
3. Environment variables with `KNPT_` prefix

## YAML Configuration

The application loads the following configuration files:

- `config/knooppunt.yml`: Knooppunt-specific configuration
- `config/nuts.yml`: Nuts-specific configuration,
  see [Nuts documentation](https://nuts-node.readthedocs.io/en/stable/pages/deployment/configuration.html)

### Example YAML Configuration

```yaml
# mCSD (Mobile Care Services Discovery) configuration
mcsd:
  # Root Admin Directories to synchronize from
  admin:
    "example-org":
      fhirbaseurl: "https://fhir.example.org/fhir"
    "another-org":
      fhirbaseurl: "https://fhir.another-org.com/fhir"

  # Local Query Directory to sync to
  query:
    fhirbaseurl: "http://localhost:8080/fhir"

# mCSD Admin configuration
mcsdadmin:
  # Base URL for FHIR server used by admin interface
  fhirbaseurl: "http://localhost:8080/fhir"

# Nuts node configuration  
nuts:
  # Whether to enable the Nuts node component
  enabled: true

# NVI (Nederlandse VerwijsIndex) configuration
nvi:
  # Base URL for FHIR server used by NVI component
  baseurl: "https://example.com/nvi"
```

## Environment Variables

Environment variables use the prefix `KNPT_` followed by the configuration path in uppercase with underscores:

| Environment Variable                | YAML Path                      | Description                                                                                                                                                |
|-------------------------------------|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `KNPT_NUTS_ENABLED`                 | `nuts.enabled`                 | Enable embedded Nuts node                                                                                                                                  |
| `KNPT_MCSDADMIN_FHIRBASEURL`        | `mcsdadmin.fhirbaseurl`        | FHIR base URL for admin interface                                                                                                                          |
| `KNPT_MCSD_QUERY_FHIRBASEURL`       | `mcsd.query.fhirbaseurl`       | Local Query Directory FHIR base URL                                                                                                                        |
| `KNPT_MCSD_ADMIN_<KEY>_FHIRBASEURL` | `mcsd.admin.<key>.fhirbaseurl` | Remote mCSD Admin Directory FHIR base URL                                                                                                                  |
| `KNPT_NVI_BASEURL`                  | `nvi.baseurl`                  | Base URL of the NVI service.                                                                                                                               |
| `KNPT_NVI_AUDIENCE`                 | `nvi.audience`                 | Name of the NVI service, used for creating BSN transport tokens. Defaults to "nvi".                                                                        |
| `KNPT_MITZ_MITZBASE`                | `mitz.mitzbase`                | Base URL of the MITZ endpoint                                                                                                                              |
| `KNPT_MITZ_NOTIFYENDPOINT`          | `mitz.notifyendpoint`          | Endpoint that will be used in Subscription.channel.endpoint when subscribing to Mitz (unless one is provided in the Subscription request to the knooppunt) |
| `KNPT_MITZ_GATEWAYSYSTEM`           | `mitz.gatewaysystem`           | URL where MITZ will send consent notifications (your callback endpoint)                                                                                    |
| `KNPT_MITZ_SOURCESYSTEM`            | `mitz.sourcesystem`            | Gateway system OID (added as FHIR extension)                                                                                                               |
| `KNPT_MITZ_TLSCERTFILE`             | `mitz.tlscertfile`             | Path to client certificate (.p12/.pfx or .pem)                                                                                                             |
| `KNPT_MITZ_TLSKEYFILE`              | `mitz.tlskeyfile`              | Path to private key (only for .pem certs)                                                                                                                  |
| `KNPT_MITZ_TLSKEYPASSWORD`          | `mitz.tlskeypassword`          | Password for .p12/.pfx                                                                                                                               |
| `KNPT_MITZ_TLSCAFILE`               | `mitz.tlscafile`               | Path to server certificate                                                                                                                                 |

### Example Environment Variable Usage

```bash
# Disable Nuts node component
export KNPT_NUTS_ENABLED=false

# Set FHIR base URL for admin interface  
export KNPT_MCSDADMIN_FHIRBASEURL=http://fhir.example.com:8080/fhir

# Set FHIR base URL for NVI component
export KNPT_NVI_BASEURL=http://nvi.example.com:8080/fhir

# Start the application
./nuts-knooppunt
```

## Configuration Options

### mCSD Configuration

The mCSD (Mobile Care Services Discovery) component synchronizes healthcare service information from external
directories.

- `mcsd.admin`: Map of root directories to synchronize from
    - Each entry has a `fhirbaseurl` pointing to the external FHIR server
- `mcsd.query.fhirbaseurl`: URL of the local FHIR directory to store synchronized data

### mCSD Admin Configuration

The mCSD Admin component provides a web interface for managing healthcare service information.

- `mcsdadmin.fhirbaseurl`: URL of the FHIR server for the admin interface

### Nuts Node Configuration

The Nuts node component integrates with the Nuts network for decentralized healthcare data exchange.

- `nuts.enabled`: Whether to enable the Nuts node component (default: true)

### NVI Configuration

The NVI (Nederlandse VerwijsIndex) component provides an API for registering DocumentReference resources.

- `nvi.baseurl`: URL of the FHIR server for NVI DocumentReference registration
- `nvi.audience`: Name of the NVI service, used for creating BSN transport tokens (default: "nvi")
