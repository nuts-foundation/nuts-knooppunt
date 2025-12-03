import { JWK } from 'jose';

/**
 * Generate a did:web identifier from hostname
 */
export function generateDidWeb(hostname: string): string {
  // did:web uses : instead of / for path separators
  // and URL-encodes the hostname
  const encoded = hostname.replace(/:/g, '%3A').replace(/\//g, ':');
  return `did:web:${encoded}`;
}

/**
 * Parse a did:web to get the URL for DID document resolution
 */
export function didWebToUrl(did: string): string {
  if (!did.startsWith('did:web:')) {
    throw new Error('Invalid did:web format');
  }

  const identifier = did.slice('did:web:'.length);
  // Decode the identifier
  const decoded = identifier.replace(/%3A/g, ':').replace(/:/g, '/');

  // Check if it has a path
  if (decoded.includes('/')) {
    return `https://${decoded}/did.json`;
  }

  return `https://${decoded}/.well-known/did.json`;
}

/**
 * Generate a DID document for a did:web
 */
export function generateDidDocument(did: string, publicKeyJwk: JWK): object {
  const keyId = publicKeyJwk.kid || `${did}#key-1`;

  return {
    '@context': [
      'https://www.w3.org/ns/did/v1',
      'https://w3id.org/security/suites/jws-2020/v1',
    ],
    id: did,
    verificationMethod: [
      {
        id: keyId,
        type: 'JsonWebKey2020',
        controller: did,
        publicKeyJwk: {
          kty: publicKeyJwk.kty,
          crv: publicKeyJwk.crv,
          x: publicKeyJwk.x,
          alg: publicKeyJwk.alg,
          use: publicKeyJwk.use,
        },
      },
    ],
    authentication: [keyId],
    assertionMethod: [keyId],
  };
}
