terraform {
  required_version = ">= 1.7.0"

  required_providers {
    ovh = {
      source  = "ovh/ovh"
      version = "~> 2.19"
    }
  }

  # Partial config: bucket/key/region/endpoint passed via `terraform init -backend-config=...`
  # or a gitignored backend.hcl. Separate bucket and credentials from
  # infra-identity's state - see README.
  backend "s3" {}
}

# Credentials come from OVH_APPLICATION_KEY / OVH_APPLICATION_SECRET / OVH_CONSUMER_KEY
# env vars, never from this file.
provider "ovh" {
  endpoint = "ovh-eu"
}
