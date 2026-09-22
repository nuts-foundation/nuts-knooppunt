#!/usr/bin/env bash
# Deploys the sandbox environment to the OVHcloud MKS cluster (infra/cluster).
#
# discovery/*.json and policy/*.json are loaded as --set-file values rather
# than inlined into values.yaml - the chart supports any number of discovery
# service definitions and policy documents (including zero), so they can't
# be named in a fixed set of flags. Add or remove a file here and it's
# picked up automatically, no script or values.yaml change needed.
#
# Usage: infra/ovhcloud-sandbox/deploy.sh [extra helm upgrade args...]
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

set_file_args=()
for f in discovery/*.json; do
  [ -e "$f" ] || continue
  id="$(basename "$f" .json)"
  set_file_args+=(--set-file "discoveryDefinitions.${id//./\\.}=${f}")
done
for f in policy/*.json; do
  [ -e "$f" ] || continue
  name="$(basename "$f")"
  set_file_args+=(--set-file "policyDocuments.${name//./\\.}=${f}")
done

# --wait: the seed Job is a post-upgrade hook that calls knooppunt and HAPI.
# Without --wait, Helm runs the hook as soon as the manifests are applied,
# before the Deployments' readiness probes pass, and the seed fails.
helm upgrade sandbox ../../helm/nuts-knooppunt \
  --wait \
  -f values.yaml \
  ${set_file_args[@]+"${set_file_args[@]}"} \
  "$@"
