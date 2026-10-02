#!/bin/bash
# Generates the GF Sandbox demo PKI and the mock Dezi signing key.
#
# Nothing this produces is committed: the output directory is gitignored. Run
# this once before
# `docker compose -f docker-compose.yml -f docker-compose.sandbox.yml --profile sandbox up`.
#
# A dedicated CA is generated rather than reusing test/e2e/pep/certs, which
# exists for PEP mTLS tests and issues UZI otherName certificates rather than
# TLS server certificates.
#
# Usage: ./sandbox/generate-demo-certs.sh

set -euo pipefail

# Keys land owner-only from the moment openssl creates them rather than after
# a later chmod. The final modes are set at the bottom, once it is clear which
# files a container has to read.
umask 077

HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$HERE/.certs"
mkdir -p "$OUT"

# Generation happens here and the live material is touched only once the whole
# set exists and has been validated. Inside $OUT so that installing it is a
# rename rather than a copy, which needs the same filesystem, and because $OUT
# is the gitignored directory this script owns: a sibling would litter the
# checkout every time a run died. Removing it is the one exception to the rule
# further down about not deleting what this script did not create, and it is
# not much of one, since nothing else writes a dotted staging directory here.
STAGE="$OUT/.staging"
trap 'rm -rf "$STAGE"' EXIT

# Docker creates a directory wherever a bind-mount source is missing, and
# compose mounts four paths out of this directory: mock-dezi.pem,
# mock-dezi.key and dezi-signing.key from docker-compose.yml, and
# ca-only/gf-sandbox-demo-ca.pem from docker-compose.sandbox.yml. A stack
# brought up before this script has ever run therefore leaves a directory at
# each of them.
#
# The presence probe below is -f, which a directory fails, so without this the
# script reads the material as absent and generates a replacement set. Staging
# keeps that off the live files, but installing it does not: the renames go one
# at a time, so ca.key and ca.pem land and then the first directory stops the
# rest, leaving half a rotated PKI and a directory the operator has to know to
# delete. The ca-only case is worse still, because mv writes *into* a directory
# rather than failing, so that one exits 0 with the knooppunt's trust path
# mounted from a directory. -f rather than -r for the reason bootstrap-nuts.sh
# used to give for its own copy of this guard: a directory is perfectly
# readable, and a directory is the state this exists to keep out.
#
# It refuses rather than repairs. Nothing here can tell a directory Docker
# created from one the operator cares about, and rm -rf of a path this script
# did not create is not its call to make.
#
# ca-only is the asymmetry: it is legitimately a directory while everything
# else this script writes is a file, so the two kinds are checked separately.
# A blanket -d test over the output would reject a healthy tree.
#
# Every path this script installs is listed, not just the three compose mounts
# and not just the material apply_modes carries modes for: ca.srl is here too,
# because a directory at any of them stops the renames halfway. The csr and ext
# intermediates used to be listed for the same reason and no longer are, since
# they are written in the staging directory now and never appear here; refusing
# a whole run over a leftover one would cost the operator a rotation for a path
# nothing reads. Unlisted paths are left alone: the directory also holds
# whatever the operator put there, which this script does not touch.
refuse_wrong_kind() {
  echo "$1 exists but is not $2." >&2
  echo "Bringing the compose stack up before this script runs leaves a Docker-created directory at every path it mounts." >&2
  echo "Delete $OUT and re-run this script." >&2
  exit 1
}

require_expected_kinds() {
  if [[ -e $OUT/ca-only && ! -d $OUT/ca-only ]]; then
    refuse_wrong_kind "$OUT/ca-only" "a directory"
  fi

  local name
  for name in ca.pem ca.key ca.srl mock-dezi.pem mock-dezi.key \
    dezi-signing.key ca-only/gf-sandbox-demo-ca.pem; do
    if [[ -e $OUT/$name && ! -f $OUT/$name ]]; then
      refuse_wrong_kind "$OUT/$name" "a regular file"
    fi
  done
}

# Ahead of apply_modes and of the presence probe, so a wedged tree costs
# neither a chmod nor a key.
require_expected_kinds

# The containers read these files as fixed non-root UIDs (18092 for mock-dezi,
# 18081 for the knooppunt), while a bind mount on Linux passes the host file's
# owner and mode straight through. Anything left at 0600 owned by the host user
# is therefore unreadable inside the container, and mock-dezi exits fatally on
# its signing key. Docker Desktop on macOS hides this by remapping the owner,
# which is why it only shows up on Linux and rootless Docker.
#
# So: certificates are public and get 0644, and exactly two private keys follow
# them, mock-dezi.key and dezi-signing.key. Those two are widened because
# otherwise the stack does not start, not because the material does not matter.
# On a shared host 0644 means every local account can read them, and whoever
# reads mock-dezi.key can impersonate the mock Dezi service to anything that
# trusts this demo CA. That is an acceptable trade only because these two are
# throwaway keys for a local demo, regenerated by deleting this directory and
# gitignored; it is not a trade to copy into anything real.
#
# ca.key stays owner-only: no fixed-UID container reads it, and it is mounted
# nowhere at all.
#
# This runs on both paths, including the early exit below, and narrows as well
# as widens. An earlier version of this script left everything at 0600;
# whoever generated with it is exactly the person this has to converge, and
# re-running would otherwise tell them there is nothing to do while changing
# nothing.
#
# Every file is named rather than globbed. A glob over "$OUT" would sweep in
# whatever else happens to sit there: an unrelated private key called
# something.pem would be widened to 0644, and a dangling symlink would make
# chmod fail mid-command, which under `set -e` would abandon the rest of this
# function and leave the very outage it exists to repair.
#
# Directories are made searchable first, before anything tests for or changes a
# file inside them. An older run could leave $OUT or ca-only at 0700, and a
# directory that cannot be traversed makes every probe below it fail.
apply_modes() {
  chmod 755 "$OUT"
  if [[ -d $OUT/ca-only ]]; then
    chmod 755 "$OUT/ca-only"
  fi

  local name
  for name in ca.pem mock-dezi.pem mock-dezi.key dezi-signing.key; do
    if [[ -f $OUT/$name ]]; then
      chmod 644 "$OUT/$name"
    fi
  done
  if [[ -f $OUT/ca-only/gf-sandbox-demo-ca.pem ]]; then
    chmod 644 "$OUT/ca-only/gf-sandbox-demo-ca.pem"
  fi
  if [[ -f $OUT/ca.key ]]; then
    chmod 600 "$OUT/ca.key"
  fi
}

# Repair modes before probing for the material: a 0700 directory left by an
# older run would otherwise fail every -f test below and silently take the
# generation path, which then fails on the files that are already there.
apply_modes

# Set by validate_material to the first relationship that does not hold, so
# whichever caller acts on the failure can say what it was.
MATERIAL_PROBLEM=""

# Prints the offset of the OCTET STRING holding the named extension's value, so
# the rest of a check can reparse that extension alone.
#
# The name is matched as the *value of an OBJECT record*, not as text anywhere
# in the output. openssl renders a known extension OID by its friendly name, and
# a certificate is free to carry that same string elsewhere: put it in the
# subject and it appears as a UTF8STRING well before the extensions, so an
# unanchored match selects an earlier extension's OCTET STRING and answers about
# the wrong extension entirely.
#
# Scanning forward to the OCTET STRING rather than reading the next line: an
# Extension is "OID, optional critical BOOLEAN, extnValue OCTET STRING", so a
# critical extension puts a BOOLEAN in between, and passing a BOOLEAN's offset
# to -strparse segfaults LibreSSL 3.3.6.
extension_value_offset() {
  openssl asn1parse -in "$1" 2>/dev/null | awk -v name="$2" '
    /prim: *OBJECT/ { seen = ($0 ~ (":" name "$")); next }
    seen && /prim: *OCTET STRING/ {
      split($0, field, ":")
      gsub(/ /, "", field[1])
      print field[1]
      exit
    }
  '
}

# A root generated before this material became did:x509 anchored is not a CA,
# and the resolver rejects every chain it anchors: it checks IsCA on the
# certificate the fingerprint names (nuts-node vdr/didx509/resolver.go). No
# amount of re-running fixes that by chmod alone.
#
# Basic Constraints is decoded rather than grepped for. Searching the rendered
# certificate for "CA:TRUE" finds the text wherever it sits, and a self-signed
# certificate with no extensions at all and the subject "CN=CA:TRUE" satisfied
# it. Nothing else in the set contradicted that, because LibreSSL 3.3.6 accepts
# a non-CA certificate as an explicit -CAfile anchor, so certificate_issued_by
# passed too and the whole set validated while the node rejected it.
root_is_ca() {
  local offset
  offset=$(extension_value_offset "$1" 'X509v3 Basic Constraints')
  [[ -n $offset ]] || return 1
  # cA defaults to FALSE and is omitted when false, so a present BOOLEAN TRUE is
  # the whole of the assertion.
  openssl asn1parse -in "$1" -strparse "$offset" 2>/dev/null | grep -qE 'BOOLEAN +:(255|TRUE)'
}

# Public keys rather than moduli, because that covers every key type openssl
# writes here without caring which one it is.
#
# The non-empty test is not belt and braces. Two failed openssl calls both
# yield the empty string, so an unguarded comparison answers "these agree"
# when what happened is "neither of these parsed", which is precisely the case
# this function exists to catch.
certificate_matches_key() {
  local certificate_public key_public
  certificate_public=$(openssl x509 -in "$1" -noout -pubkey 2>/dev/null) || return 1
  key_public=$(openssl pkey -in "$2" -pubout 2>/dev/null) || return 1
  [[ -n $certificate_public && $certificate_public == "$key_public" ]]
}

certificate_issued_by() {
  openssl verify -CAfile "$2" "$1" >/dev/null 2>&1
}

# The type, not merely that it parses. mock-components/dezi/keys.go
# type-asserts *rsa.PrivateKey and exits with "is a %T, not an RSA private key"
# on anything else, so a perfectly valid EC key here is a stack that does not
# start. openssl pkey, which this used to use, accepts every key type openssl
# knows and so accepted exactly that.
#
# openssl rsa accepts both encodings this script can produce, PKCS#1 from
# genrsa -traditional and PKCS#8 from genrsa without it, which is the same pair
# mock-dezi accepts, and rejects everything else.
is_rsa_private_key() {
  openssl rsa -in "$1" -noout >/dev/null 2>&1
}

check() {
  local problem=$1
  shift
  if "$@"; then
    return 0
  fi
  MATERIAL_PROBLEM=$problem
  return 1
}

material_present() {
  local dir=$1 name
  for name in ca.pem ca.key mock-dezi.pem mock-dezi.key dezi-signing.key \
    ca-only/gf-sandbox-demo-ca.pem; do
    [[ -f $dir/$name ]] || return 1
  done
}

# What has to hold before this script may call a directory usable. Existence
# was the whole of the old test, and existence is the one thing an interrupted
# run guarantees: it overwrote the files it got to and left the rest, so every
# path was still there and every later run said there was nothing to do. The
# mismatch surfaced two services away, at credential storage or at the token
# request, naming the node rather than this.
#
# So the relationships are what get checked, and they are checked by whoever
# asks: the early exit before it claims the material works, and generation
# before it lets a new set replace the old one. A relationship the early exit
# skipped would be exactly the one that survives every future run.
#
# Not checked, deliberately: mock-dezi.pem's DNS SAN, and how long anything
# has left to live. A wrong hostname fails loudly at the TLS handshake and
# says so ("certificate is valid for ..."), and expiry is both a decade away
# and already covered for the certificates that anchor anything, since
# certificate_issued_by rejects an expired chain.
validate_material() {
  local dir=$1

  check "ca.pem is not a CA certificate" \
    root_is_ca "$dir/ca.pem" || return 1
  check "ca.key is not the key ca.pem was issued for" \
    certificate_matches_key "$dir/ca.pem" "$dir/ca.key" || return 1
  check "mock-dezi.key is not the key mock-dezi.pem was issued for" \
    certificate_matches_key "$dir/mock-dezi.pem" "$dir/mock-dezi.key" || return 1
  check "mock-dezi.pem was not issued by ca.pem" \
    certificate_issued_by "$dir/mock-dezi.pem" "$dir/ca.pem" || return 1
  check "ca-only/gf-sandbox-demo-ca.pem is not a copy of ca.pem" \
    cmp -s "$dir/ca-only/gf-sandbox-demo-ca.pem" "$dir/ca.pem" || return 1
  check "dezi-signing.key is not an RSA private key" \
    is_rsa_private_key "$dir/dezi-signing.key" || return 1
}

# Moves the validated set into place. One rename at a time, so an interruption
# here still leaves a mixture of old and new files behind. What it cannot
# leave behind is a mixture that the run after it calls healthy: every file in
# the set is tied to the root by one of the checks above, so any prefix of
# these renames fails at least one of them and regenerates. Detecting the
# broken state is the property that was missing, not atomicity of the swap,
# and bash has no way to rename several paths as one operation regardless.
install_material() {
  local name
  for name in ca.key ca.pem ca.srl mock-dezi.pem mock-dezi.key dezi-signing.key; do
    mv -f "$STAGE/$name" "$OUT/$name"
  done
  mkdir -p "$OUT/ca-only"
  mv -f "$STAGE/ca-only/gf-sandbox-demo-ca.pem" "$OUT/ca-only/gf-sandbox-demo-ca.pem"
}

if material_present "$OUT" && validate_material "$OUT"; then
  echo "Demo material already present in $OUT and still hangs together;"
  echo "permissions normalized, nothing else to do."
  echo "Delete the directory and re-run to rotate it."
  exit 0
fi

# Nothing to say on a first run, where the directory is simply empty. A
# directory that has material in it and is about to lose it is a different
# matter: this is the only place the operator can be told which relationship
# failed, and the failure it reports is invisible from `ls`.
if [[ -n $MATERIAL_PROBLEM ]]; then
  echo "Regenerating the whole set: $MATERIAL_PROBLEM." >&2
fi

rm -rf "$STAGE"
mkdir -p "$STAGE/ca-only"

echo "Generating demo CA..."
openssl genrsa -out "$STAGE/ca.key" 4096
# CA:TRUE is not decorative: the did:x509 resolver fingerprints this
# certificate and rejects the chain unless it is a CA
# (nuts-node vdr/didx509/resolver.go). A root without Basic Constraints
# produces a chain the node will not resolve.
openssl req -x509 -new -nodes -key "$STAGE/ca.key" -sha256 -days 3650 \
  -extensions ext -config <(printf '%s\n' \
    '[req]' 'distinguished_name=dn' '[ dn ]' \
    '[ ext ]' 'basicConstraints=critical,CA:TRUE,pathlen:1' 'keyUsage=critical,keyCertSign,cRLSign') \
  -out "$STAGE/ca.pem" -subj "/CN=GF Sandbox Demo CA"

echo "Generating mock-dezi server certificate..."
openssl genrsa -out "$STAGE/mock-dezi.key" 2048
openssl req -new -key "$STAGE/mock-dezi.key" -out "$STAGE/mock-dezi.csr" \
  -subj "/CN=mock-dezi"

# Go ignores the Common Name for hostname verification, so the DNS SAN is
# mandatory, not decorative.
cat > "$STAGE/mock-dezi.ext" <<'EOF'
extendedKeyUsage = serverAuth
subjectAltName = DNS:mock-dezi, DNS:localhost
EOF

#
# -CAserial is explicit rather than relying on -CAcreateserial's default
# naming: some OpenSSL/LibreSSL builds derive that default from the first
# dot in the full path, not the last dot in the basename, which turns the
# dotdir ".certs" into a stray sibling file (sandbox/.srl) outside $OUT.
openssl x509 -req -in "$STAGE/mock-dezi.csr" -CA "$STAGE/ca.pem" -CAkey "$STAGE/ca.key" \
  -CAcreateserial -CAserial "$STAGE/ca.srl" -out "$STAGE/mock-dezi.pem" -days 3650 -sha256 \
  -extfile "$STAGE/mock-dezi.ext"

echo "Generating mock Dezi attestation signing key..."
# mock-components/dezi/keys.go accepts either PKCS#1 or PKCS#8, but new keys
# are still written as PKCS#1 here for continuity with keys already deployed
# in that format. OpenSSL 3 defaults genrsa to PKCS#8; -traditional restores
# PKCS#1. LibreSSL, the macOS default, predates that flag and rejects it
# outright (nonzero exit, no file written), so probe for support first
# rather than letting the script fail under `set -e` on macOS.
if openssl genrsa -traditional -out /dev/null 512 >/dev/null 2>&1; then
  openssl genrsa -traditional -out "$STAGE/dezi-signing.key" 2048
else
  openssl genrsa -out "$STAGE/dezi-signing.key" 2048
fi

# The knooppunt container mounts the file below into its certificate path.
# Keep private keys out of this directory: Go reads every file it finds in
# /etc/ssl/certs, and a future mount of the whole directory should not be able
# to pull one in.
cp "$STAGE/ca.pem" "$STAGE/ca-only/gf-sandbox-demo-ca.pem"

# This script is the only thing that can produce a set it will accept, so it
# holds itself to the same check. Failing here means openssl did something
# other than what the lines above say, and the right answer to that is to
# leave whatever is already in $OUT alone rather than replace it with this.
if ! validate_material "$STAGE"; then
  echo "Generated material does not hang together: $MATERIAL_PROBLEM." >&2
  echo "$OUT was left as it was." >&2
  exit 1
fi

install_material
apply_modes

echo
echo "Written to $OUT (gitignored):"
echo "  ca.pem, ca.key             demo certificate authority"
echo "  mock-dezi.pem, .key        TLS server certificate, SAN mock-dezi + localhost"
echo "  dezi-signing.key           attestation signing key"
echo "  ca-only/                   CA alone, mounted into the knooppunt trust path"
