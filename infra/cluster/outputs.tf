output "kube_id" {
  description = "Managed Kubernetes Service cluster ID."
  value       = ovh_cloud_project_kube.test.id
}

output "region" {
  description = "Cluster region."
  value       = ovh_cloud_project_kube.test.region
}

# Deliberately no kubeconfig output - self-served per-person from the
# Control Panel (admins via role, ops via kubeconfig/create), not
# committed to Terraform state as a single shared credential.
