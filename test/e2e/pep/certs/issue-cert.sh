#!/bin/bash
# Issues a test UZI certificate with SAN containing URA
# Based on: https://github.com/nuts-foundation/uzi-did-x509-issuer/tree/main/test_ca
#
# Usage: ./issue-cert.sh <hostname> <organization_name> <locality> <uzi> <ura> <agb>
# Example: ./issue-cert.sh nodeA "Test Hospital" "Amsterdam" 0 87654321 0
#
# The URA is encoded in the SAN otherName field as:
# 2.16.528.1.1007.99.2110-1-<uzi>-S-<ura>-00.000-<agb>

set -e

if [[ $OSTYPE == msys ]]; then
  echo "Detected GitBash/Cygwin on Windows"
  DN_PREFIX="//"
else
  DN_PREFIX="/"
fi

HOST=$1
X509_O=$2
X509_L=$3
UZI=$4
URA=$5
AGB=$6

if [[ -z $HOST || -z $X509_O || -z $X509_L || -z $UZI || -z $URA || -z $AGB ]]; then
  echo "Usage: $0 HOST ORGANIZATION LOCALITY UZI URA AGB"
  echo "Example: $0 nodeA 'Test Hospital' Amsterdam 0 87654321 0"
  exit 1
fi

# Check if CA exists
if [[ ! -f ca.pem || ! -f ca.key ]]; then
  echo "Error: CA files not found. Run generate-root-ca.sh first."
  exit 1
fi

echo "Generating key and certificate for $HOST (URA: $URA)..."

# Generate private key
openssl genrsa -out $HOST.key 2048

# Create CSR
openssl req -new -key $HOST.key -out $HOST.csr \
  -subj "${DN_PREFIX}CN=${HOST}/O=${X509_O}/L=${X509_L}/serialNumber=${UZI}"

# Create extension file with URA in SAN
# The format matches the bgz.json pattern: ^[0-9.]+-\d+-\d+-S-(\d+)-00\.000-\d+$
local_openssl_config="
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = otherName:2.5.5.5;UTF8:2.16.528.1.1007.99.2110-1-${UZI}-S-${URA}-00.000-${AGB}
"
echo "$local_openssl_config" > $HOST.ext

# Sign the certificate
openssl x509 -req -in $HOST.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
  -out $HOST.pem -days 365 -sha256 -extfile $HOST.ext

# Create certificate chain (end-entity cert first, then CA per RFC 5246)
cat $HOST.pem > $HOST-chain.pem
cat ca.pem >> $HOST-chain.pem

# Cleanup
rm -f $HOST.csr $HOST.ext

echo "Certificate generated:"
echo "  - $HOST.key (private key)"
echo "  - $HOST.pem (certificate)"
echo "  - $HOST-chain.pem (certificate chain)"
