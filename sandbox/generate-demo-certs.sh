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

OUT="$(cd "$(dirname "$0")" && pwd)/.certs"
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
# gives for its own copy of this guard: a directory is perfectly readable, and
# a directory is the state this exists to keep out.
#
# It refuses rather than repairs. Nothing here can tell a directory Docker
# created from one the operator cares about, and rm -rf of a path this script
# did not create is not its call to make.
#
# ca-only is the asymmetry: it is legitimately a directory while everything
# else this script writes is a file, so the two kinds are checked separately. A
# blanket -d test over the output would reject a healthy tree.
#
# Every path this script installs is listed, not just the four compose mounts
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
    dezi-signing.key ca-only/gf-sandbox-demo-ca.pem \
    plataan-uzi.pem plataan-uzi-chain.pem plataan-uzi.key; do
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
# The other two keys stay owner-only, because no fixed-UID container reads
# them. ca.key is mounted nowhere at all. plataan-uzi.key is mounted only into
# the one-shot toolkit container in bootstrap-nuts.sh, whose pinned image runs
# as root and so reads a host-owned 0600 bind mount unaided. It signs the
# ten-year did:x509 identity this demo authorizes against: at 0644 every
# account on the host could copy it and mint credentials as De Plataan.
#
# This runs on both paths, including the early exit below, and narrows as well
# as widens. An earlier version of this script left everything at 0600 and a
# later one widened plataan-uzi.key to 0644; whoever generated with either is
# exactly the person this has to converge, and re-running would otherwise tell
# them there is nothing to do while changing nothing.
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
  for name in ca.pem mock-dezi.pem mock-dezi.key dezi-signing.key \
    plataan-uzi.pem plataan-uzi-chain.pem; do
    if [[ -f $OUT/$name ]]; then
      chmod 644 "$OUT/$name"
    fi
  done
  if [[ -f $OUT/ca-only/gf-sandbox-demo-ca.pem ]]; then
    chmod 644 "$OUT/ca-only/gf-sandbox-demo-ca.pem"
  fi
  for name in ca.key plataan-uzi.key; do
    if [[ -f $OUT/$name ]]; then
      chmod 600 "$OUT/$name"
    fi
  done
}

# Repair modes before probing for the material: a 0700 directory left by an
# older run would otherwise fail every -f test below and silently take the
# generation path, which then fails on the files that are already there.
apply_modes

# The URA travels in the leaf's SAN otherName, which is where the bgz
# presentation definition's descriptor looks for it. Named up here because
# generation and validation have to agree on it: a leaf carrying anything else
# is a leaf this script must not accept.
PLATAAN_URA=00000010
PLATAAN_OTHERNAME="2.16.528.1.1007.99.2110-1-0-S-${PLATAAN_URA}-00.000-0"

# Set by validate_material to the first relationship that does not hold, so
# whichever caller acts on the failure can say what it was.
MATERIAL_PROBLEM=""

# A root generated before this material became did:x509 anchored is not a CA,
# and the resolver rejects every chain it anchors
# (nuts-node vdr/didx509/resolver.go). No amount of re-running fixes that by
# chmod alone.
root_is_ca() {
  openssl x509 -in "$1" -noout -text 2>/dev/null | grep -q "CA:TRUE"
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

is_private_key() {
  openssl pkey -in "$1" -noout >/dev/null 2>&1
}

# The chain is a concatenation, so comparing it against one is both the
# simplest check available and the strictest: it says this file is exactly
# this leaf followed by this root, with nothing else in it and nothing in the
# wrong order.
chain_is_leaf_then_root() {
  cmp -s <(cat "$1/plataan-uzi.pem" "$1/ca.pem") "$1/plataan-uzi-chain.pem"
}

# openssl's own rendering cannot be grepped portably: LibreSSL, which is what
# /usr/bin/openssl is on macOS, prints "othername:<unsupported>" where
# OpenSSL 3 prints the value. The DER can be, because the URA travels as a
# UTF8String and is therefore its own bytes on the wire.
leaf_carries_the_ura() {
  openssl x509 -in "$1" -outform DER 2>/dev/null | grep -qa "$PLATAAN_OTHERNAME"
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
    ca-only/gf-sandbox-demo-ca.pem plataan-uzi.pem plataan-uzi.key \
    plataan-uzi-chain.pem; do
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
# certificate_issued_by rejects an expired chain. The URA is the opposite
# case, and is checked: nothing between here and the presentation definition
# ever looks at it, so a leaf without it fails as a policy non-match in a
# service that has never heard of this directory.
validate_material() {
  local dir=$1

  check "ca.pem is not a CA certificate" \
    root_is_ca "$dir/ca.pem" || return 1
  check "ca.key is not the key ca.pem was issued for" \
    certificate_matches_key "$dir/ca.pem" "$dir/ca.key" || return 1
  check "mock-dezi.key is not the key mock-dezi.pem was issued for" \
    certificate_matches_key "$dir/mock-dezi.pem" "$dir/mock-dezi.key" || return 1
  check "plataan-uzi.key is not the key plataan-uzi.pem was issued for" \
    certificate_matches_key "$dir/plataan-uzi.pem" "$dir/plataan-uzi.key" || return 1
  check "mock-dezi.pem was not issued by ca.pem" \
    certificate_issued_by "$dir/mock-dezi.pem" "$dir/ca.pem" || return 1
  check "plataan-uzi.pem was not issued by ca.pem" \
    certificate_issued_by "$dir/plataan-uzi.pem" "$dir/ca.pem" || return 1
  check "ca-only/gf-sandbox-demo-ca.pem is not a copy of ca.pem" \
    cmp -s "$dir/ca-only/gf-sandbox-demo-ca.pem" "$dir/ca.pem" || return 1
  check "plataan-uzi-chain.pem is not plataan-uzi.pem followed by ca.pem" \
    chain_is_leaf_then_root "$dir" || return 1
  check "plataan-uzi.pem does not carry $PLATAAN_OTHERNAME in its SAN otherName" \
    leaf_carries_the_ura "$dir/plataan-uzi.pem" || return 1
  check "dezi-signing.key is not a private key" \
    is_private_key "$dir/dezi-signing.key" || return 1
}

# Moves the validated set into place. One rename at a time, so an interruption
# here still leaves a mixture of old and new files behind. What it cannot
# leave behind is a mixture that the run after it calls healthy: every file in
# the set is tied to the root by one of the checks above, so any prefix of
# these renames fails at least one of them and regenerates. Detecting the
# broken state is the property that was missing, not atomicity of the swap,
# and bash has no way to rename ten paths as one operation regardless.
install_material() {
  local name
  for name in ca.key ca.pem ca.srl mock-dezi.pem mock-dezi.key dezi-signing.key \
    plataan-uzi.pem plataan-uzi.key plataan-uzi-chain.pem; do
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

echo "Generating De Plataan UZI-style certificate..."
# The shape follows test/e2e/pep/certs/issue-cert.sh, whose format the existing
# descriptor pattern already matches; the single capture group yields the bare
# URA.
openssl genrsa -out "$STAGE/plataan-uzi.key" 2048
openssl req -new -key "$STAGE/plataan-uzi.key" -out "$STAGE/plataan-uzi.csr" \
  -subj "/CN=plataan/O=Ziekenhuis De Plataan/L=Utrecht/serialNumber=0"
printf '%s\n' \
  'extendedKeyUsage = clientAuth' \
  "subjectAltName = otherName:2.5.5.5;UTF8:$PLATAAN_OTHERNAME" \
  > "$STAGE/plataan-uzi.ext"
openssl x509 -req -in "$STAGE/plataan-uzi.csr" -CA "$STAGE/ca.pem" -CAkey "$STAGE/ca.key" \
  -CAcreateserial -CAserial "$STAGE/ca.srl" -out "$STAGE/plataan-uzi.pem" -days 3650 -sha256 \
  -extfile "$STAGE/plataan-uzi.ext"
# End-entity first, then the CA, per RFC 5246.
cat "$STAGE/plataan-uzi.pem" "$STAGE/ca.pem" > "$STAGE/plataan-uzi-chain.pem"

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
echo "  plataan-uzi.pem, .key      UZI-style leaf, SAN otherName carries the URA"
echo "  plataan-uzi-chain.pem      leaf + ca.pem, sorted leaf to root"
