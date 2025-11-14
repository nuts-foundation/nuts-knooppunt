# Changelog

All notable changes to the nuts-knooppunt Helm chart will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.3] - 2025-01-12

### Added
- HTTP interface configuration section in values.yaml
  - `config.http.public.address` - TCP address for public HTTP interface (default: `:8080`)
  - `config.http.public.url` - Optional public base URL
  - `config.http.internal.address` - TCP address for internal HTTP interface (default: `:8081`)
  - `config.http.internal.url` - Optional internal base URL
- Complete Nuts Knooppunt application configuration section in values.yaml
  - mCSD (Mobile Care Services Discovery) configuration
  - mCSD Admin configuration
  - MITZ Connector configuration
  - PDP (Policy Decision Point) configuration
- `config.mcsd.adminexclude` - List of FHIR base URLs to exclude from administration directories to prevent self-syncing

### Changed
- Configuration is now rendered into knooppunt.yml ConfigMap for better configuration management

## [0.1.2] - 2024-12-XX

### Added
- HAPI FHIR configuration properties in `fhir.configExtra`:
  - `hapi.fhir.server_id_strategy: UUID` - Generate UUIDs for resources instead of integers
  - `hapi.fhir.store_meta_source_information: SOURCE_URI` - Store meta.source as provided by client
  - `server.tomcat.relaxed-query-chars: |` - Prevent encoding of pipe character in queries
  - `hapi.fhir.version: R4` - Set FHIR version to R4
- `fhir.multitenancy.enabled: false` - Multitenancy configuration option

## [0.1.1] - 2024-12-XX

### Changed
- Updated default nuts-knooppunt image tag from `0.1.2` to `0.2.0`

## [0.1.0] - 2024-11-XX

### Added
- Initial Helm chart release for nuts-knooppunt
- Kubernetes deployment templates
- Service configuration with ClusterIP type
- Ingress support with configurable hostnames and paths
- HTTPRoute support for Gateway API
- Liveness and readiness probes on `/status` endpoint
- Autoscaling configuration (disabled by default)
- ServiceAccount with configurable annotations
- HAPI FHIR subchart integration
  - PostgreSQL database support
  - Configurable storage (5Gi default)
- NUTS Node subchart integration
  - Database secret conversion from postgresql:// to postgres:// scheme
  - Filesystem storage for crypto keys
  - Persistent volume claim support
- Security contexts for pods and containers
- Resource limits and requests (commented out by default)
- Volume and volumeMount support
