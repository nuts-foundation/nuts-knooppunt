# Configuration

The Knooppunt application supports configuration through YAML files and environment variables.

## Sources

Configuration is loaded in the following order (later sources override earlier ones):

1. Default values
2. YAML configuration files, loaded from:
   - `config/knooppunt.yml`: Knooppunt-specific configuration ([example](../config/knooppunt.yml))
   - `config/nuts.yml`: Nuts-specific configuration, see [Nuts documentation](https://nuts-node.readthedocs.io/en/stable/pages/deployment/configuration.html) ([example](../config/nuts.yml))
3. Environment variables with `KNPT_` prefix

## Configuration Options

Environment variables use the prefix `KNPT_` followed by the configuration path in uppercase with underscores:

| Environment Variable                | YAML Path                      | Description                                                                                                                                                  |
|-------------------------------------|--------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Authentication / Nuts**           |                                |                                                                                                                                                              |
| `KNPT_NUTS_ENABLED`                 | `nuts.enabled`                 | Enable embedded Nuts node.<br/>Defaults to `true`.                                                                                                           |
| **Addressing / mCSD**               |                                |                                                                                                                                                              |
| `KNPT_MCSDADMIN_FHIRBASEURL`        | `mcsdadmin.fhirbaseurl`        | (Optional) FHIR base URL of the local mCSD Administration Directory, if managed through the mCSD Web Application.                                            |
| `KNPT_MCSD_QUERY_FHIRBASEURL`       | `mcsd.query.fhirbaseurl`       | FHIR base URL of the local mCSD Query Directory to synchronize to.                                                                                           |
| `KNPT_MCSD_ADMIN_<KEY>_FHIRBASEURL` | `mcsd.admin.<key>.fhirbaseurl` | Map of root directories (mCSD Admin Directory FHIR base URLs) to synchronize from.                                                                           |
| `KNPT_MCSD_ADMINEXCLUDE`            | `mcsd.adminexclude`            | (Optional) List of FHIR base URLs to exclude from being registered as administration directories. Useful to prevent self-referencing loops when the query directory is discovered as an Endpoint. Multiple values can be specified as a comma-separated list. |
| **Localization / NVI**              |                                |                                                                                                                                                              |
| `KNPT_NVI_BASEURL`                  | `nvi.baseurl`                  | Base URL of the NVI service.                                                                                                                                 |
| `KNPT_NVI_AUDIENCE`                 | `nvi.audience`                 | Name of the NVI service, used for creating BSN transport tokens.<br/>Defaults to `nvi`.                                                                      |
| **Consent / Mitz**                  |                                |                                                                                                                                                              |
| `KNPT_MITZ_MITZBASE`                | `mitz.mitzbase`                | Base URL of the MITZ endpoint                                                                                                                                |
| `KNPT_MITZ_NOTIFYENDPOINT`          | `mitz.notifyendpoint`          | Endpoint that will be used in `Subscription.channel.endpoint` when subscribing to Mitz (unless one is provided in the Subscription request to the knooppunt) |
| `KNPT_MITZ_GATEWAYSYSTEM`           | `mitz.gatewaysystem`           | (Optional) URL where MITZ will send consent notifications (your callback endpoint)                                                                           |
| `KNPT_MITZ_SOURCESYSTEM`            | `mitz.sourcesystem`            | (Optional) gateway system OID (added as FHIR extension)                                                                                                      |
| `KNPT_MITZ_TLSCERTFILE`             | `mitz.tlscertfile`             | Path to client certificate (.p12/.pfx or .pem)                                                                                                               |
| `KNPT_MITZ_TLSKEYFILE`              | `mitz.tlskeyfile`              | Path to private key (only for .pem certs)                                                                                                                    |
| `KNPT_MITZ_TLSKEYPASSWORD`          | `mitz.tlskeypassword`          | Password for .p12/.pfx                                                                                                                                       |
| `KNPT_MITZ_TLSCAFILE`               | `mitz.tlscafile`               | Path to server certificate                                                                                                                                   |
