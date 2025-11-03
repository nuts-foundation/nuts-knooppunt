# Helm Charts

This directory contains Helm charts for nuts-knooppunt and its components.

## Available Charts

- `helm-nuts-knooppunt` - Main chart with fhir and nuts-node dependencies
- `helm-fhir` - HAPI FHIR server (standalone)
- `helm-pep` - Policy Enforcement Point (standalone)

## Publishing

Charts are automatically published to GitHub Container Registry on git tag push:

```bash
git tag v0.2.0
git push origin v0.2.0
```

Published charts are available at:
```
oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt
oci://ghcr.io/nuts-foundation/helm-fhir
oci://ghcr.io/nuts-foundation/helm-pep
```

## Version Management

**IMPORTANT**: Chart versions in `Chart.yaml` files are source of truth.

Before creating a release tag:
1. Update `version` in all relevant `Chart.yaml` files
2. Update dependency versions in `helm-nuts-knooppunt/Chart.yaml` if needed
3. Update `appVersion` to match the application version
4. Create PR, merge to main
5. Tag the merge commit

Example PR:
```yaml
# helm/nuts-knooppunt/Chart.yaml
version: 0.2.0
appVersion: "0.2.0"

dependencies:
  - name: helm-fhir
    version: "0.2.0"  # Must match published helm-fhir version
```

## Installation

```bash
helm install my-knooppunt oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt --version 0.1.0
```