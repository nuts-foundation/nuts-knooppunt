#!/usr/bin/env bash
# Checks that the test environment's public hosts serve real content, not
# just a 200. Run by ci.yaml after each deploy and by smoke-test.yaml every
# hour. The Zonnebloem EHR check reads clients from HAPI, so it also fails
# when HAPI or its database is down.
#
# Each check is retried, so a pod that is briefly restarting doesn't fail it.
# Prints every failing check and exits 1 if any failed.
#
# Usage: infra/ovhcloud-test/smoke-test.sh
set -uo pipefail

attempts=3
delay=20

# URL, then a string the response body must contain.
checks=(
  "https://knooppunt.test.gf.nuts.nl/nuts/status" "OK"
  "https://knooppunt.test.gf.nuts.nl/.well-known/oauth-authorization-server/nuts/oauth2/00000020" '"issuer":"https://knooppunt.test.gf.nuts.nl/nuts/oauth2/00000020"'
  "https://nutsadmin.test.gf.nuts.nl/" "<title>Nuts Demo registry admin</title>"
  "https://mock-dezi.test.gf.nuts.nl/dezi/jwks.json" '"kid":"sandbox-dezi-1"'
  "https://sandbox.test.gf.nuts.nl/" "<title>GF Sandbox</title>"
  "https://zonnebloem-ehr.test.gf.nuts.nl/" "Anna Jansen"
)

failed=0
for ((i = 0; i < ${#checks[@]}; i += 2)); do
  url="${checks[i]}"
  want="${checks[i+1]}"
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if body="$(curl -sS --fail --max-time 15 "$url" 2>&1)"; then
      [[ "$body" == *"$want"* ]] && break
      reason="response does not contain '$want'"
    else
      reason="$body"
    fi
    if ((attempt < attempts)); then
      sleep "$delay"
    fi
  done
  if ((attempt > attempts)); then
    echo "FAIL $url: $reason"
    failed=1
  else
    echo "OK   $url"
  fi
done
exit "$failed"
