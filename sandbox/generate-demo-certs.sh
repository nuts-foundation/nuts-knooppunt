#!/bin/bash
# Generates the GF Sandbox demo PKI and the mock Dezi signing key.
#
# Nothing this produces is committed: the output directory is gitignored. Run
# this once before `docker compose --profile sandbox up`.
#
# A dedicated CA is generated rather than reusing test/e2e/pep/certs, which
# exists for PEP mTLS tests and issues UZI otherName certificates rather than
# TLS server certificates.
#
# Usage: ./sandbox/generate-demo-certs.sh

set -euo pipefail

# Belt and braces with the chmod 600 below: files land without group/world
# permissions from the moment openssl creates them, not just afterward.
umask 077

OUT="$(cd "$(dirname "$0")" && pwd)/.certs"
mkdir -p "$OUT"
chmod 700 "$OUT"

if [[ -f $OUT/ca.pem && -f $OUT/ca.key && -f $OUT/mock-dezi.pem && -f $OUT/mock-dezi.key \
      && -f $OUT/dezi-signing.key && -f $OUT/ca-only/gf-sandbox-demo-ca.pem ]]; then
  echo "Demo material already present in $OUT; nothing to do."
  echo "Delete the directory and re-run to rotate it."
  exit 0
fi

echo "Generating demo CA..."
openssl genrsa -out "$OUT/ca.key" 4096
openssl req -x509 -new -nodes -key "$OUT/ca.key" -sha256 -days 3650 \
  -subj "/CN=GF Sandbox Demo CA/O=GF Sandbox" -out "$OUT/ca.pem"

echo "Generating mock-dezi server certificate..."
openssl genrsa -out "$OUT/mock-dezi.key" 2048
openssl req -new -key "$OUT/mock-dezi.key" -out "$OUT/mock-dezi.csr" \
  -subj "/CN=mock-dezi"

# Go ignores the Common Name for hostname verification, so the DNS SAN is
# mandatory, not decorative.
cat > "$OUT/mock-dezi.ext" <<'EOF'
extendedKeyUsage = serverAuth
subjectAltName = DNS:mock-dezi, DNS:localhost
EOF

#
# -CAserial is explicit rather than relying on -CAcreateserial's default
# naming: some OpenSSL/LibreSSL builds derive that default from the first
# dot in the full path, not the last dot in the basename, which turns the
# dotdir ".certs" into a stray sibling file (sandbox/.srl) outside $OUT.
openssl x509 -req -in "$OUT/mock-dezi.csr" -CA "$OUT/ca.pem" -CAkey "$OUT/ca.key" \
  -CAcreateserial -CAserial "$OUT/ca.srl" -out "$OUT/mock-dezi.pem" -days 3650 -sha256 \
  -extfile "$OUT/mock-dezi.ext"

echo "Generating mock Dezi attestation signing key..."
# mock-components/dezi/keys.go accepts either PKCS#1 or PKCS#8, but new keys
# are still written as PKCS#1 here for continuity with keys already deployed
# in that format. OpenSSL 3 defaults genrsa to PKCS#8; -traditional restores
# PKCS#1. LibreSSL, the macOS default, predates that flag and rejects it
# outright (nonzero exit, no file written), so probe for support first
# rather than letting the script fail under `set -e` on macOS.
if openssl genrsa -traditional -out /dev/null 512 >/dev/null 2>&1; then
  openssl genrsa -traditional -out "$OUT/dezi-signing.key" 2048
else
  openssl genrsa -out "$OUT/dezi-signing.key" 2048
fi

rm -f "$OUT/mock-dezi.csr" "$OUT/mock-dezi.ext"
chmod 600 "$OUT"/*.key

# The knooppunt container mounts this directory into its certificate path, and
# Go reads every file it finds there. Keep private keys out of it.
mkdir -p "$OUT/ca-only"
cp "$OUT/ca.pem" "$OUT/ca-only/gf-sandbox-demo-ca.pem"

echo
echo "Written to $OUT (gitignored):"
echo "  ca.pem, ca.key             demo certificate authority"
echo "  mock-dezi.pem, .key        TLS server certificate, SAN mock-dezi + localhost"
echo "  dezi-signing.key           attestation signing key"
echo "  ca-only/                   CA alone, mounted into the knooppunt trust path"
