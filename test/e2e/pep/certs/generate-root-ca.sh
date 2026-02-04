#!/bin/bash
# Generates a fake UZI root CA for testing
# Based on: https://github.com/nuts-foundation/uzi-did-x509-issuer/tree/main/test_ca

set -e

if [[ $OSTYPE == msys ]]; then
  echo "Script does not work on GitBash/Cygwin!"
  exit 1
fi

CONFIG="
[req]
distinguished_name=dn
[ dn ]
[ ext ]
basicConstraints=CA:TRUE,pathlen:0
"

echo "Generating Fake UZI Root CA..."
openssl genrsa -out ca.key 2048
openssl req -config <(echo "$CONFIG") -extensions ext -x509 -new -nodes -key ca.key -sha256 -days 3650 -out ca.pem -subj "/CN=Fake UZI Root CA"
echo "Root CA generated: ca.pem, ca.key"
