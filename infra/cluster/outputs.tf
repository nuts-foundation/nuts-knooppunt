output "kube_id" {
  description = "Managed Kubernetes Service cluster ID."
  value       = ovh_cloud_project_kube.test.id
}

output "region" {
  description = "Cluster region."
  value       = ovh_cloud_project_kube.test.region
}

# Deliberately no kubeconfig output - self-served per-person from the
# Control Panel (admins via role, ops via kubeconfig/create) is still the
# intended day-to-day path.
#
# That said, omitting this output does NOT keep the kubeconfig out of
# Terraform state. The OVH provider's ovh_cloud_project_kube resource stores
# the full kubeconfig, including the client's private key, as resource
# attributes regardless of whether any output references them (see
# setKubeconfig in the provider source:
# https://github.com/ovh/terraform-provider-ovh/blob/v2.19.0/ovh/resource_cloud_project_kube.go#L982-L1000).
# Anyone with read access to this state (the S3 backend bucket) already has
# a working cluster credential - treat access to that bucket accordingly.
