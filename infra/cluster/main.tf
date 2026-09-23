# OVHcloud Managed Kubernetes Service cluster for the test environment.
# See ../../infra-identity DESIGN.md for the access model this cluster
# plugs into (admins vs. ops IAM policies).

resource "ovh_cloud_project_kube" "test" {
  service_name = "883f6929b35949fe86b401a4ccd0a7fd"
  name         = "gf-test"

  # DE1: MKS-available, closest region to the team (Utrecht). Matches the
  # region already used for infra-identity's state bucket.
  region = "DE1"

  # Control plane is free on this plan; we only pay for nodes/storage/networking.
  plan = "free"

  # Minor version upgrades fail outright under NEVER_UPDATE.
  update_policy = "MINIMAL_DOWNTIME"
}

resource "ovh_cloud_project_kube_nodepool" "test" {
  service_name = ovh_cloud_project_kube.test.service_name
  kube_id      = ovh_cloud_project_kube.test.id
  # Named after the flavor: flavor_name can't change in place, so a resize
  # is a new pool, and two pools can't share a name while the old one is
  # still draining (see create_before_destroy below).
  name = "d2-8"

  # d2 not b3: from 2026-10-01, local storage and public IPv4 unbundle from
  # Gen 3 (b3/c3/r3) pricing. d2 keeps storage included and supports
  # monthly billing.
  #
  # d2-8 (2 vCPU, 8 GB) rather than d2-4: OVH's managed kubelet reserves
  # about half of a node for itself, leaving a d2-4 with ~1.9 GiB for pods.
  # HAPI on Postgres alone takes ~900 MiB, so a rolling update of it
  # already tipped a node into MemoryPressure (nuts-knooppunt#602). A d2-8
  # leaves ~5-6 GiB per node. Catalog price (EUR excl. VAT, 2026-09):
  # d2-4 EUR 11.44/month, d2-8 EUR 20.60/month per node.
  flavor_name = "d2-8"

  # d2-8 monthly: EUR 20.60 vs EUR 27.16 (0.0372/h x 730) hourly, per node.
  # One-way for the running month: OVH doesn't switch a node back to
  # hourly. Fine for a pool that is meant to stay up.
  monthly_billed = true

  # The new pool has to be up before the old one goes, or the cluster is
  # empty in between. Pods still get evicted when the old nodes leave; the
  # Cinder volumes (Postgres, nuts-node data, vc-issuer) reattach on the
  # new nodes.
  lifecycle {
    create_before_destroy = true
  }

  # 2 nodes: accepted tradeoff for a test environment - an upgrade drain
  # can leave everything on one node, so brief downtime is possible during
  # upgrades. Autoscaling off makes the compute bill a constant -
  # over-deploying yields pending pods, not invoices.
  desired_nodes = 2
  min_nodes     = 2
  max_nodes     = 2
  autoscale     = false
}
