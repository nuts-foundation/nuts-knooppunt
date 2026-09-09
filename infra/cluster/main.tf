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
  name         = "default"

  # d2 not b3: from 2026-10-01, local storage and public IPv4 unbundle from
  # Gen 3 (b3/c3/r3) pricing. d2 keeps storage included and supports
  # monthly billing. If memory-bound later, move to d2-8 rather than
  # adding nodes.
  flavor_name = "d2-4"

  # 3 nodes not 2: upgrades drain one node at a time, so workloads must
  # fit on 2. Autoscaling off makes the compute bill a constant -
  # over-deploying yields pending pods, not invoices.
  desired_nodes = 3
  min_nodes     = 3
  max_nodes     = 3
  autoscale     = false
}
