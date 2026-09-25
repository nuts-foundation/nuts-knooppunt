# infra/cluster

Terraform for the OVHcloud Managed Kubernetes Service (MKS) cluster and
node pool backing the GF test environment. See
[`infra-identity`](https://github.com/nuts-foundation/infra-identity)'s
`DESIGN.md` for the access model (admins vs. ops) this cluster plugs into.

## What's here

- `ovh_cloud_project_kube.test` — MKS cluster, region `DE1`, free control
  plane plan, `MINIMAL_DOWNTIME` update policy.
- `ovh_cloud_project_kube_nodepool.test` — 2x `d2-8` nodes, monthly
  billed, fixed (autoscaling off). Accepted tradeoff: upgrades may cause
  brief downtime since a drain can leave everything on one node.

Not yet scaffolded: Kubernetes-level manifests (namespaces, RBAC,
ResourceQuota, ingress) and the CI/CD deploy workflow — pending a
cluster/CI design discussion within the team (see
`infra-identity/DESIGN.md`, Open items).

## Setup

**Current status**: cluster and node pool are applied and running
(`gf-test`, region DE1, 2x `d2-8`, monthly billed).

Two sets of credentials, never committed. Keep them in
`.scratch/` (gitignored) as `ovh.env` and `s3.env`, or wherever your
password manager puts them:

| File | Variables | Where to get them |
|---|---|---|
| `ovh.env` | `OVH_APPLICATION_KEY`, `OVH_APPLICATION_SECRET`, `OVH_CONSUMER_KEY` | https://api.ovh.com/createToken, rights `GET/POST/PUT/DELETE` on `/cloud/project/*` |
| `s3.env` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Control Panel → Public Cloud → Object Storage → *S3 users*: the user with access to the `nuts-knooppunt-cluster-tfstate` bucket (add one if needed). The S3 backend reads them from the AWS variable names. |

Then, from this directory:

```
source ../../.scratch/ovh.env && source ../../.scratch/s3.env
terraform init -backend-config=backend.hcl
terraform plan
```

`backend.hcl` holds the state bucket settings (S3-compatible, separate
bucket and credentials from `infra-identity`'s state - don't reuse). The
first error you'll see without `s3.env` is "No valid credential sources
found": that's the state backend, not the OVH provider.

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

## Operating the sandbox

Things that aren't visible from the code and have bitten before.

### Changing the deployment

`infra/ovhcloud-sandbox/deploy.sh` is the only way to change the release.
No `kubectl edit`/`kubectl set`: the next `deploy.sh` reverts it. Two
exceptions, needed once each because Helm can't express them:

- Switching a Deployment from `RollingUpdate` to `Recreate` (the knooppunt
  and `fhir` Deployments already are). The live object carries a defaulted
  `rollingUpdate` block that the API server rejects next to `Recreate`, and
  Helm's three-way merge never removes a field the previous release didn't
  set. Done on the sandbox; for a new Deployment:
  `kubectl patch deployment <name> --type=json -p='[{"op":"remove","path":"/spec/strategy/rollingUpdate"},{"op":"replace","path":"/spec/strategy/type","value":"Recreate"}]'`
- Installing the CloudNativePG operator (cluster-wide, outside the
  release): see `helm/nuts-knooppunt/README.md`.

### Nodes

- A `d2-8` has 8 GB, but OVH's kubelet reserves about 2 GB, so ~5-6 GiB
  is available to pods. On the previous `d2-4` that was ~1.9 GiB, which
  HAPI alone (~900 MiB) plus a rolling update exceeded.
- `flavor_name` can't change in place: a resize is a new pool
  (`create_before_destroy` in `main.tf`), and the old nodes' pods get
  evicted when those nodes leave. Volumes (Postgres, nuts-node data,
  vc-issuer) are Cinder and reattach on the new nodes.
- Monthly billing is one-way for the running month.

### Public IPs

Three kinds, and only one matters for DNS:

| Where (Control Panel → Public IPs) | What | Do |
|---|---|---|
| Floating IP, attached to a load balancer: `57.129.23.152` | The ingress-nginx LoadBalancer. Every hostname points here; nip.io today, `*.test.gf.nuts.nl` as a wildcard `A` record once DNS is set up (#563). | Keep. Deleting the `ingress-nginx-controller` Service releases it and a new one gets a different address, which breaks every hostname. |
| Basic IP, one IPv4 + one IPv6 per node, plus one IPv4 for `k8s-cluster-…` (the control plane) | Nodes' own addresses, used for image pulls, the kubelet's connection to the control plane and pods' outbound calls. Replaced together with the nodes. | Leave alone. Nothing should point at them. |
| Floating or Additional IP with an empty "Associated resource" | Leftover, e.g. from a deleted load balancer. | Delete; billed per address from 2026-10-01. |

### Data

Persistent data lives in Cinder volumes (`kubectl get pvc`): the shared
Postgres (`nuts-knooppunt-db-1`), the nuts-node's keys
(`nuts-knooppunt-data`), the vc-issuer's SQLite. They are kept on
`helm uninstall`; a `kubectl delete pvc` is the only way to lose them.
The seed Job re-creates demo data on every `helm upgrade`; it does not
recreate keys or subjects that already exist.
