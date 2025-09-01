# Configuration

The Nuts Knooppunt application supports configuration through YAML files and environment variables.

## Configuration Priority

Configuration is loaded in the following order (later sources override earlier ones):

1. Default values (hardcoded)
2. YAML configuration file 
3. Environment variables with `KNPT_` prefix

## YAML Configuration

The application automatically looks for configuration files in the following locations:

- `config/knooppunt.yaml`
- `config/knooppunt.yml`

### Example YAML Configuration

```yaml
# mCSD (Mobile Care Services Discovery) configuration
mcsd:
  # Root directories to synchronize from
  rootdirectories:
    "example-org":
      fhirbaseurl: "https://fhir.example.org/fhir"
    "another-org":
      fhirbaseurl: "https://fhir.another-org.com/fhir"
  
  # Local FHIR directory configuration
  localdirectory:
    fhirbaseurl: "http://localhost:8080/fhir"

# mCSD Admin configuration
mcsdadmin:
  # Base URL for FHIR server used by admin interface
  fhirbaseurl: "http://localhost:8080/fhir"

# Nuts node configuration  
nuts:
  # Whether to enable the Nuts node component
  enabled: true
  
  # Path to Nuts node configuration file (optional)
  configfile: "config/nuts.yaml"
```

## Environment Variables

Environment variables use the prefix `KNPT_` followed by the configuration path in uppercase with underscores:

| Environment Variable                  | YAML Path                         | Description                         |
|---------------------------------------|-----------------------------------|-------------------------------------|
| `KNPT_NUTS_ENABLED`                   | `nuts.enabled`                    | Enable embedded Nuts node           |
| `KNPT_NUTS_CONFIGFILE`                | `nuts.configfile`                 | Path to Nuts node configuration file |
| `KNPT_MCSDADMIN_FHIRBASEURL`          | `mcsdadmin.fhirbaseurl`           | FHIR base URL for admin interface   |
| `KNPT_MCSD_LOCALDIRECTORY_FHIRBASEURL`| `mcsd.localdirectory.fhirbaseurl` | Local FHIR directory URL            |

### Example Environment Variable Usage

```bash
# Disable Nuts node component
export KNPT_NUTS_ENABLED=false

# Set FHIR base URL for admin interface  
export KNPT_MCSDADMIN_FHIRBASEURL=http://fhir.example.com:8080/fhir

# Start the application
./nuts-knooppunt
```

## Configuration Options

### mCSD Configuration

The mCSD (Mobile Care Services Discovery) component synchronizes healthcare service information from external directories.

- `mcsd.rootdirectories`: Map of root directories to synchronize from
  - Each entry has a `fhirbaseurl` pointing to the external FHIR server
- `mcsd.localdirectory.fhirbaseurl`: URL of the local FHIR directory to store synchronized data

### mCSD Admin Configuration

The mCSD Admin component provides a web interface for managing healthcare service information.

- `mcsdadmin.fhirbaseurl`: URL of the FHIR server for the admin interface

### Nuts Node Configuration

The Nuts node component integrates with the Nuts network for decentralized healthcare data exchange.

- `nuts.enabled`: Whether to enable the Nuts node component (default: true)
- `nuts.configfile`: Path to the Nuts node configuration file (optional)