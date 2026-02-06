import { SignJWT, jwtVerify, decodeProtectedHeader } from 'jose';
import { getPrivateKey, getOrGenerateKeyPair } from './ed25519';

/**
 * Sign an access token JWT
 */
export async function signAccessToken(
  payload: Record<string, unknown>,
  issuerDid: string,
  subject: string,
  expirationSeconds: number = 86400
): Promise<string> {
  const keyPair = await getOrGenerateKeyPair(issuerDid, 'VC');
  const privateKey = await getPrivateKey(keyPair.privateKeyJwk);

  const expirationTime = Math.floor(Date.now() / 1000) + expirationSeconds;

  return new SignJWT(payload)
    .setProtectedHeader({
      alg: 'EdDSA',
      kid: keyPair.privateKeyJwk.kid,
    })
    .setIssuedAt()
    .setIssuer(issuerDid)
    .setSubject(subject)
    .setAudience(issuerDid)
    .setExpirationTime(expirationTime)
    .sign(privateKey);
}

/**
 * Verify an access token JWT
 */
export async function verifyAccessToken(
  token: string,
  issuerDid: string
): Promise<Record<string, unknown>> {
  const keyPair = await getOrGenerateKeyPair(issuerDid, 'VC');
  const privateKey = await getPrivateKey(keyPair.privateKeyJwk);

  const { payload } = await jwtVerify(token, privateKey, {
    audience: issuerDid,
    issuer: issuerDid,
  });

  return payload as Record<string, unknown>;
}

/**
 * Extract subject DID from a proof JWT's kid header
 */
export function getSubjectDidFromProof(proofJwt: string): string {
  const header = decodeProtectedHeader(proofJwt);
  let did = header.kid || '';

  // kid is usually in format "did:web:example.com#key-1"
  if (did.includes('#')) {
    did = did.split('#')[0];
  }

  return did;
}
