# Helm Charts

This directory contains Helm charts for nuts-knooppunt and its components.

## Available Charts

- `helm-nuts-knooppunt` - Main chart with fhir, pep, and nuts-node dependencies
- `helm-fhir` - HAPI FHIR server (standalone)
- `helm-pep` - Policy Enforcement Point (standalone)

## Version Management

Different charts have different versioning strategies:

- **helm-nuts-knooppunt**: Automatic versioning (set by git tag)
- **helm-fhir**: Manual versioning (set in Chart.yaml)
- **helm-pep**: Manual versioning (set in Chart.yaml)

**Why automatic for nuts-knooppunt?**

This chart is only published on git tags, which by definition indicate app changes. Automatically coupling chart version to git tag simplifies versioning: `helm install --version 0.2.0` always deploys app v0.2.0.

**Why manual for fhir/pep?**

These components have independent release cycles from nuts-knooppunt:
- **PEP**: Reference implementation, changes infrequently
- **FHIR**: Third-party HAPI server, follows HAPI release schedule

Following Helm best practices (Bitnami, Prometheus, etc.), chart version is managed manually and tracks chart development separately from app version. This allows:
- Chart bug fixes without app version changes (0.1.0 → 0.1.1)
- App upgrades with chart version bump (0.2.0 → 0.3.0 when upgrading HAPI 7.2.0 → 7.4.0)
- Flexibility to release chart improvements independent of app releases

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

**⚠️ CRITICAL: Always bump chart version when changing image tags or config. OCI registries are immutable - publishing the same version twice overwrites the existing chart.**

**Semantic versioning guide:**
- **Patch (0.1.0 → 0.1.1)**: Chart bug fixes, config tweaks, same app version
- **Minor (0.1.0 → 0.2.0)**: New app version, backwards compatible
- **Major (0.1.0 → 1.0.0)**: Breaking chart changes (renamed values, removed features)

**Example: Upgrading HAPI FHIR**

1. Update `helm/fhir/Chart.yaml`:
   ```yaml
   version: 0.2.0        # Bump minor (new HAPI version)
   appVersion: "7.4.0"   # New HAPI version
   ```

2. Update `helm/fhir/values.yaml`:
   ```yaml
   image:
     tag: v7.4.0         # New HAPI FHIR image
   ```

3. Update `helm/nuts-knooppunt/Chart.yaml` dependency:
   ```yaml
   dependencies:
     - name: helm-fhir
       version: "0.2.0"  # Reference new chart version
   ```

4. Create PR, merge to `main` → workflow automatically publishes helm-fhir:0.2.0

5. Create new nuts-knooppunt release (e.g., `v0.2.1`) to include the new FHIR version

**Example: Chart config fix (no HAPI upgrade)**

1. Update `helm/fhir/Chart.yaml`:
   ```yaml
   version: 0.1.1        # Bump patch (config fix only)
   appVersion: "7.2.0"   # Same HAPI version
   ```

2. Fix config in templates or values.yaml

3. Merge to `main` → workflow publishes helm-fhir:0.1.1

**Same process for helm-pep**: Always bump chart version when changing anything.

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