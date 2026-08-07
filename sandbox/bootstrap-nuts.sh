#!/usr/bin/env bash
# Creates the Nuts subject the sandbox requests tokens for, and puts an
# X509Credential carrying De Plataan's URA in its wallet.
#
# Idempotent by checking before every mutation: the node returns an error for
# a subject that already exists, so a blind create breaks every restart. The
# equivalent helpers in test/e2e/pep/authorization_test.go are unconditional
# and deliberately not copied.
#
# The durable, resettable version of this belongs to #542; this exists so the
# demo runs and the tests have something to drive.
#
# Usage: ./sandbox/bootstrap-nuts.sh

set -euo pipefail

NUTS="${NUTS_INTERNAL_BASE_URL:-http://localhost:8081}/nuts/internal"
SUBJECT="${SANDBOX_NUTS_SUBJECT:-plataan}"
CERTS="$(cd "$(dirname "$0")" && pwd)/.certs"

did=$(curl -sf "$NUTS/vdr/v2/subject" | python3 -c '
import json, sys

subjects = json.load(sys.stdin)
print((subjects.get(sys.argv[1]) or [""])[0])
' "$SUBJECT")

if [[ -z $did ]]; then
  echo "Creating subject $SUBJECT..."
  # The name has to be in the body: without it the node generates a uuid, and
  # every wallet call below would then address a subject that does not exist.
  did=$(curl -sf -X POST "$NUTS/vdr/v2/subject" \
    -H 'Content-Type: application/json' -d "{\"subject\":\"$SUBJECT\"}" \
    | python3 -c 'import json, sys; print(json.load(sys.stdin)["documents"][0]["id"])')
else
  echo "Subject $SUBJECT already exists as $did."
fi

# A wallet returns a JWT credential as a JSON string and a JSON-LD credential
# as an object whose type is a bare string when it has only one, so all three
# shapes have to be read before concluding the credential is missing.
has_x509=$(curl -sf "$NUTS/vcr/v2/holder/$SUBJECT/vc" | python3 -c '
import base64, json, sys


def types(credential):
    if isinstance(credential, str):
        segments = credential.split(".")
        if len(segments) != 3:
            return []
        try:
            payload = segments[1]
            claims = json.loads(base64.urlsafe_b64decode(payload + "=" * (-len(payload) % 4)))
        except ValueError:
            return []
        credential = claims.get("vc", {})
    declared = credential.get("type", [])
    return [declared] if isinstance(declared, str) else declared


print(any("X509Credential" in types(vc) for vc in json.load(sys.stdin)))
')

if [[ $has_x509 == True ]]; then
  echo "Wallet already holds an X509Credential; nothing to do."
  exit 0
fi

if [[ ${SANDBOX_SKIP_DIDX509:-0} == 1 ]]; then
  # The idempotence test drives the two node calls without Docker. The payload
  # decodes to {"vc":{"type":["VerifiableCredential","X509Credential"]}}, so
  # the check above reads the placeholder back exactly as it reads a real one.
  credential="eyJhbGciOiJub25lIn0.eyJ2YyI6eyJ0eXBlIjpbIlZlcmlmaWFibGVDcmVkZW50aWFsIiwiWDUwOUNyZWRlbnRpYWwiXX19.placeholder"
else
  echo "Issuing the X509Credential..."
  credential=$(docker run --rm \
    -v "$CERTS/plataan-uzi-chain.pem:/cert-chain.pem:ro" \
    -v "$CERTS/plataan-uzi.key:/cert-key.key:ro" \
    nutsfoundation/go-didx509-toolkit:main \
    vc /cert-chain.pem /cert-key.key "CN=GF Sandbox Demo CA" "$did")
fi

curl -sf -X POST "$NUTS/vcr/v2/holder/$SUBJECT/vc" \
  -H 'Content-Type: application/json' -d "\"$credential\"" > /dev/null
echo "Stored the X509Credential for $SUBJECT."
