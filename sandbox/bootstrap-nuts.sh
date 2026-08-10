#!/usr/bin/env bash
# Creates the Nuts subject the sandbox requests tokens for, and puts an
# X509Credential carrying De Plataan's URA in its wallet.
#
# Idempotent by checking before every mutation: the node returns an error for
# a subject that already exists, so a blind create breaks every restart. The
# equivalent helpers in test/e2e/pep/authorization_test.go are unconditional
# and deliberately not copied.
#
# That buys no repeated writes, not convergence on the intended state, and the
# two are the same thing only while the inputs do not change. The wallet check
# matches on credential type alone, so after a certificate rotation the wallet
# still holds one issued from the old chain, this reports nothing to do, and
# the demo fails later at token request time with an error that names the node
# rather than the bootstrap. Rotating the certificates therefore means
# clearing the Nuts volume along with them.
#
# The durable, resettable version of this belongs to #542; this exists so the
# demo runs and the tests have something to drive.
#
# Runs on the host, not in a container: it needs bash, python3 and a docker
# CLI with a reachable daemon, and the certificate paths it hands the toolkit
# resolve only where the repository is checked out.
#
# Usage: ./sandbox/bootstrap-nuts.sh, after ./sandbox/generate-demo-certs.sh

set -euo pipefail

NUTS="${NUTS_INTERNAL_BASE_URL:-http://localhost:8081}/nuts/internal"
SUBJECT="${SANDBOX_NUTS_SUBJECT:-plataan}"
CERTS="$(cd "$(dirname "$0")" && pwd)/.certs"

# Every call to the node goes through this, so that a refusal arrives as
# something the operator can act on.
#
# curl -f, which these calls used to use, supplies the non-zero status and
# throws the body away, and that body is the node's RFC 7807 problem document:
# the only account of why it refused (nuts-node
# docs/_static/common/error_response.yaml). What reached the operator instead
# was whatever python3 made of an empty stdin, which is a JSONDecodeError
# traceback naming a line of this script rather than anything the node said.
# The half-created subject a failure here leaves behind is recoverable on a
# rerun; not knowing which step failed or why is not.
#
# So -f is deliberately absent and the status comes from -w instead: curl exits
# 0 on a 4xx, this compares the code itself, and the body survives to be
# printed. -S keeps curl's own message for transport failures, which have no
# body at all and are a different problem with the same exit status. -s only
# suppresses the progress meter.
#
# The URL goes last, after the caller's options, so that no call can displace
# it; bootstrap_test.go pins the default base URL by reading the last argument
# curl was given.
node_call() {
  local step=$1 url=$2
  shift 2

  local response status body
  if ! response=$(curl -sS -w $'\n%{http_code}' "$@" "$url"); then
    echo "$step failed: could not reach the node at $url." >&2
    exit 1
  fi

  status=${response##*$'\n'}
  body=${response%$'\n'*}
  if [[ $status != 2* ]]; then
    echo "$step failed: the node answered HTTP $status." >&2
    if [[ -n $body ]]; then
      echo "$body" >&2
    fi
    exit 1
  fi

  printf '%s' "$body"
}

# Captured before it is parsed rather than piped straight into python3: a
# pipeline runs both sides at once, so a failed call would print the message
# above and then a traceback from the parser that received nothing, and the
# traceback is the half the operator would read first.
subjects=$(node_call "Listing subjects" "$NUTS/vdr/v2/subject")
did=$(printf '%s' "$subjects" | python3 -c '
import json, sys

subjects = json.load(sys.stdin)
print((subjects.get(sys.argv[1]) or [""])[0])
' "$SUBJECT")

if [[ -z $did ]]; then
  echo "Creating subject $SUBJECT..."
  # The name has to be in the body: without it the node generates a uuid, and
  # every wallet call below would then address a subject that does not exist.
  created=$(node_call "Creating the subject" "$NUTS/vdr/v2/subject" \
    -X POST -H 'Content-Type: application/json' -d "{\"subject\":\"$SUBJECT\"}")
  did=$(printf '%s' "$created" | python3 -c 'import json, sys; print(json.load(sys.stdin)["documents"][0]["id"])')
else
  echo "Subject $SUBJECT already exists as $did."
fi

# A wallet returns a JWT credential as a JSON string and a JSON-LD credential
# as an object whose type is a bare string when it has only one, so all three
# shapes have to be read before concluding the credential is missing.
wallet=$(node_call "Reading the wallet" "$NUTS/vcr/v2/holder/$SUBJECT/vc")
has_x509=$(printf '%s' "$wallet" | python3 -c '
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
        credential = claims.get("vc") if isinstance(claims, dict) else None
    # Decoding to valid JSON that is not an object is not a decoding failure,
    # so the guard above does not see it; without this, such an entry aborts
    # the run instead of being skipped like every other non-credential.
    if not isinstance(credential, dict):
        return []
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
  # Docker creates a directory wherever a bind-mount source is missing, and
  # these two paths sit inside generate-demo-certs.sh's own output directory.
  # That script's own presence guard is -f, which a directory fails, so it
  # would read the material as absent, rewrite the CA, and then die on the
  # directory it cannot overwrite, leaving half a rotated PKI behind. -f here
  # rather than -r for the same reason: a directory is perfectly readable, and
  # a directory is the state this exists to keep out.
  for certificate in plataan-uzi-chain.pem plataan-uzi.key; do
    [[ -f $CERTS/$certificate ]] ||
      { echo "missing $CERTS/$certificate; run ./sandbox/generate-demo-certs.sh first" >&2; exit 1; }
  done

  echo "Issuing the X509Credential..."
  # The tag is pinned because a demo whose point is to work on a fresh stack
  # cannot depend on a tag that moves. It deliberately does not track the
  # newest release: later tags exist upstream, and this is the one the demo has
  # been run against. Bumping it is a change to make on purpose and verify,
  # not a version to keep current, and bootstrap_test.go asserts this exact
  # string, so a bump has to happen in both places.
  credential=$(docker run --rm \
    -v "$CERTS/plataan-uzi-chain.pem:/cert-chain.pem:ro" \
    -v "$CERTS/plataan-uzi.key:/cert-key.key:ro" \
    nutsfoundation/go-didx509-toolkit:1.2.0 \
    vc /cert-chain.pem /cert-key.key "CN=GF Sandbox Demo CA" "$did")
fi

node_call "Storing the X509Credential" "$NUTS/vcr/v2/holder/$SUBJECT/vc" \
  -X POST -H 'Content-Type: application/json' -d "\"$credential\"" > /dev/null
echo "Stored the X509Credential for $SUBJECT."
