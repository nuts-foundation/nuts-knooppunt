# infra/cluster

Terraform for the OVHcloud Managed Kubernetes Service (MKS) cluster and
node pool backing the GF test environment. See
[`infra-identity`](https://github.com/nuts-foundation/infra-identity)'s
`DESIGN.md` for the access model (admins vs. ops) this cluster plugs into.

## What's here

- `ovh_cloud_project_kube.test` — MKS cluster, region `DE1`, free control
  plane plan, `MINIMAL_DOWNTIME` update policy.
- `ovh_cloud_project_kube_nodepool.test` — 2x `d2-4` nodes, fixed
  (autoscaling off). Accepted tradeoff: upgrades may cause brief downtime
  since a drain can leave everything on one node.

Not yet scaffolded: Kubernetes-level manifests (namespaces, RBAC,
ResourceQuota, ingress) and the CI/CD deploy workflow — pending a
cluster/CI design discussion within the team (see
`infra-identity/DESIGN.md`, Open items).

## Setup

Provider credentials (never committed):

```
export OVH_APPLICATION_KEY=...
export OVH_APPLICATION_SECRET=...
export OVH_CONSUMER_KEY=...
```

**Current status**: cluster and node pool are applied and running
(`gf-test`, region DE1, 2x `d2-4`). A credential with write access to
`/cloud/project/*` exists; get one via `api.ovh.com/createToken` scoped to
that path if you need your own.

State backend (S3-compatible, separate bucket/credentials from
`infra-identity`'s state — don't reuse):

```
terraform init \
  -backend-config="bucket=..." \
  -backend-config="key=cluster.tfstate" \
  -backend-config="region=de" \
  -backend-config="endpoints={s3=\"https://s3.de.io.cloud.ovh.net\"}" \
  -backend-config="skip_credentials_validation=true" \
  -backend-config="skip_region_validation=true" \
  -backend-config="skip_requesting_account_id=true" \
  -backend-config="skip_s3_checksum=true" \
  -backend-config="use_path_style=true"
```

Bucket doesn't exist yet — create it the same way as
`infra-identity-tfstate` (OVHcloud Object Storage, private, region
matching the cluster).

No kubeconfig output here on purpose — it's self-served per person from
the Control Panel (admins via their role, ops via the
`kube/kubeconfig/create` IAM action), and that stays the intended
day-to-day path.

That said, omitting the output does **not** keep the kubeconfig out of
Terraform state. The OVH provider stores the full kubeconfig, including
the client's private key, as attributes on the `ovh_cloud_project_kube`
resource regardless of whether any output references them ([provider
source](https://github.com/ovh/terraform-provider-ovh/blob/v2.19.0/ovh/resource_cloud_project_kube.go#L982-L1000)).
Anyone with read access to this state (the S3 backend bucket) already has
a working cluster credential — treat access to that bucket accordingly.
