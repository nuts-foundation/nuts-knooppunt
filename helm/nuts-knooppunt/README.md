# NUTS Knooppunt Helm Chart

Helm chart for deploying NUTS Knooppunt, a healthcare data exchange platform.

## Overview

This Helm chart deploys the NUTS Knooppunt application along with its dependencies:
- NUTS Node: The core NUTS node for healthcare data exchange
- HAPI FHIR Server (optional): FHIR server implementation
- PostgreSQL databases: Managed by CloudNativePG operator

## Prerequisites

- Kubernetes cluster (v1.20+)
- Helm 3.x
- CloudNativePG operator installed in your cluster

### Installing CloudNativePG Operator

Version 1.25 or later: the shared database (`database.enabled`) declares a
`Database` resource per component, which older operators don't have.

```bash
helm repo add cnpg https://cloudnative-pg.github.io/charts
helm upgrade --install cnpg cnpg/cloudnative-pg --namespace cnpg-system --create-namespace
```

## Installation

### Add Required Helm Repositories

```bash
helm repo add nuts-foundation https://nuts-foundation.github.io/nuts-node/
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo update
```

### Update Chart Dependencies

```bash
helm dependency update
```

### Install the Chart

```bash
helm install my-nuts-knooppunt .
```

With custom values:

```bash
helm install my-nuts-knooppunt . -f my-values.yaml
```

## Configuration

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nuts.enabled` | Enable NUTS Node deployment | `true` |
| `nuts.database.storage.size` | Storage size for NUTS database | `5Gi` |
| `fhir.enabled` | Enable HAPI FHIR server | `false` |
| `fhir.database.storage.size` | Storage size for FHIR database | `5Gi` |
| `ingress.enabled` | Enable ingress | `true` |
| `ingress.hosts` | Ingress hostnames | `[{host: chart-example.local}]` |
| `replicaCount` | Number of replicas | `1` |

### Database Storage Configuration

The PostgreSQL database storage sizes are now configurable:

```yaml
# NUTS database storage
nuts:
  database:
    storage:
      size: 10Gi  # Adjust as needed

# FHIR database storage
fhir:
  enabled: true
  database:
    storage:
      size: 20Gi  # Adjust as needed
```

### Shared Database

`database.enabled: true` provisions one CloudNativePG `Cluster` and, per entry
in `database.databases` (default: `fhir`, `nuts`), a logical database, an
owning role and a Secret `<release>-db-<name>` with `username`, `password` and
a ready-made `uri`. The embedded nuts-node is pointed at the `nuts` database
automatically (`NUTS_STORAGE_SQL_CONNECTION`). Pair it with
`persistence.enabled: true` for the nuts-node's crypto keys, which live on the
filesystem. HAPI is wired by the environment's values (see
`infra/ovhcloud-sandbox/values.yaml`): `fhir.postgres.enabled: false` plus
`fhir.extraEnv` pointing at the `fhir` database.

### Database Connection Configuration

The NUTS node requires a PostgreSQL database connection. The database is automatically created by CloudNativePG, but you need to configure the connection string.

#### Get the Database Connection String

```bash
# Get the auto-generated connection string from the CloudNativePG secret
RELEASE_NAME=my-nuts-knooppunt
kubectl get secret ${RELEASE_NAME}-nuts-db-app -o jsonpath='{.data.uri}' | base64 -d
```

#### Configure the Connection

Create a `values.yaml` file with the connection string:

```yaml
# Enable FHIR server
fhir:
  enabled: true
  database:
    storage:
      size: 10Gi

# Configure NUTS node
nuts:
  enabled: true
  database:
    storage:
      size: 10Gi
  nuts:
    config:
      storage:
        sql:
          # Replace with the URI from the secret above
          # Format: postgres://app:{password}@{release-name}-nuts-db-rw:5432/app
          connection: "postgres://app:YOUR_PASSWORD_HERE@my-nuts-knooppunt-nuts-db-rw:5432/app"
      strictmode: true
      verbosity: info

# Configure ingress
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: nuts.example.com
      paths:
        - path: /
          pathType: Prefix
```

Then install or upgrade:

```bash
helm install my-nuts-knooppunt . -f values.yaml
# or
helm upgrade my-nuts-knooppunt . -f values.yaml
```

**Note**: The database password is auto-generated during installation. You must retrieve it from the secret after the first install and then upgrade the release with the connection string configured.

## Uninstallation

```bash
helm uninstall my-nuts-knooppunt
```

Note: Persistent Volume Claims (PVCs) are retained by default. To delete them:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=my-nuts-knooppunt
```

## Dependencies

This chart depends on:

- **fhir** (v0.1.0): HAPI FHIR server chart (file://../fhir)
- **nuts-node-chart** (v0.0.6): NUTS Node official chart (https://nuts-foundation.github.io/nuts-node/)

## Upgrading

To upgrade an existing installation:

```bash
helm dependency update
helm upgrade my-nuts-knooppunt .
```

## Testing

Test the chart with a dry-run:

```bash
helm install my-nuts-knooppunt . --dry-run --debug
```

## Support

For issues and questions:
- NUTS Foundation: https://nuts.nl
- GitHub: https://github.com/nuts-foundation

## Version

- Chart Version: 0.1.0
- App Version: 1.16.0
