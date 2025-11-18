# Helm Charts

This directory contains Helm charts for nuts-knooppunt and its components.

## Available Charts

- `helm-nuts-knooppunt` - Main chart with fhir, pep, and nuts-node dependencies
- `helm-fhir` - HAPI FHIR server (standalone)
- `helm-pep` - Policy Enforcement Point (standalone)

## Version Management

Different charts have different versioning strategies:

- **helm-nuts-knooppunt**: Coupled to git tags (auto-versioned on release)
- **helm-fhir**: Independent versioning (follows HAPI FHIR version, published on changes)
- **helm-pep**: Independent versioning (manual versioning, published on changes)

### Releasing nuts-knooppunt

The nuts-knooppunt chart version is coupled to the application version.

1. Go to GitHub → [Releases](https://github.com/nuts-foundation/nuts-knooppunt/releases) → "Draft a new release"
2. Choose a tag (e.g., `v0.2.0`) or create a new one
3. Add release title and notes describing changes
4. Click "Publish release"

GitHub Actions automatically:
- Extracts version from tag (`v0.2.0` → `0.2.0`)
- Updates `helm/nuts-knooppunt/Chart.yaml` with `version` and `appVersion`
- Pulls dependencies (helm-fhir, helm-pep, nuts-node-chart) from their registries
- Packages and publishes helm-nuts-knooppunt to GHCR

**Note:** The workflow runs on any git tag matching `v*`.

**Result:** `helm install --version 0.2.0` deploys exactly application v0.2.0.

### Updating helm-fhir or helm-pep

These charts are published automatically when their directories change on the `main` branch.

**Example: Upgrading HAPI FHIR**

1. Update `helm/fhir/Chart.yaml`:
   ```yaml
   version: 7.4.0        # Match HAPI version
   appVersion: "7.4.0"
   ```

2. Update `helm/fhir/values.yaml`:
   ```yaml
   image:
     tag: v7.4.0         # New HAPI FHIR image version
   ```

3. Update `helm/nuts-knooppunt/Chart.yaml` dependency:
   ```yaml
   dependencies:
     - name: helm-fhir
       version: "7.4.0"  # Reference new chart version
   ```

4. Create PR, merge to `main` → workflow automatically publishes helm-fhir

5. Create new nuts-knooppunt release (e.g., `v0.2.1`) to include the new FHIR version

**Same process for helm-pep**: Update `Chart.yaml` version and `appVersion`, merge to main.

### Why These Versioning Strategies?

**nuts-knooppunt (coupled):**
- ✅ Git tag version = chart version = app version
- ✅ Every release is immediately installable via Helm
- ✅ Users see `v0.3.0` on GitHub → can install with `--version 0.3.0`
- ✅ No manual version synchronization needed

**fhir/pep (independent):**
- ✅ FHIR chart version follows HAPI FHIR version (semantic clarity)
- ✅ PEP has its own release cycle independent of nuts-knooppunt
- ✅ Only published when actually changed (efficiency)
- ✅ nuts-knooppunt dependencies explicitly declare which versions they need

## Published Charts

Charts are available at GitHub Container Registry:
```
oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt
oci://ghcr.io/nuts-foundation/helm-fhir
oci://ghcr.io/nuts-foundation/helm-pep
```

## Installation

```bash
# Install specific release version
helm install my-knooppunt oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt --version 0.2.0

# Override specific values
helm install my-knooppunt oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt \
  --version 0.2.0 \
  --set replicaCount=3

# Use custom image tag (not recommended - breaks version coupling)
helm install my-knooppunt oci://ghcr.io/nuts-foundation/helm-nuts-knooppunt \
  --version 0.2.0 \
  --set image.tag=custom-build
```

## Development

For local development and testing:

```bash
# Package charts locally
helm package helm/fhir -d .helm-packages
helm package helm/pep -d .helm-packages
helm package helm/nuts-knooppunt -d .helm-packages

# Install from local package
helm install my-knooppunt .helm-packages/helm-nuts-knooppunt-*.tgz
```