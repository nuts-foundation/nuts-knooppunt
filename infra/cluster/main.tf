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
  # The flavor can't change in place, so a new flavor means a new pool. The
  # name changes with it, so create_before_destroy can bring the new pool up
  # before the old one is removed.
  name = "d2-8"

  # d2 not b3: from 2026-10-01, local storage and public IPv4 unbundle from
  # Gen 3 (b3/c3/r3) pricing. d2 keeps storage included and supports
  # monthly billing. d2-8, not d2-4: HAPI needs ~1.3Gi and a d2-4 node has
  # only ~2Gi for all pods, so HAPI was evicted (and took Postgres with it).
  flavor_name = "d2-8"

  # 2 nodes: accepted tradeoff for a test environment - an upgrade drain
  # can leave everything on one node, so brief downtime is possible during
  # upgrades. Autoscaling off makes the compute bill a constant -
  # over-deploying yields pending pods, not invoices.
  desired_nodes = 2
  min_nodes     = 2
  max_nodes     = 2
  autoscale     = false

  # Monthly is cheaper than hourly for nodes that run all month. The pool was
  # created as monthly in the Control Panel and imported; without this,
  # Terraform wants hourly.
  monthly_billed = true

  lifecycle {
    create_before_destroy = true
  }
}
