import { SignJWT, jwtVerify, decodeProtectedHeader, JWK } from 'jose';
import { getPrivateKey, getOrGenerateKeyPair } from './ed25519';

export interface CredentialPayload {
  vc: {
    '@context': string[];
    id: string;
    type: string[];
    credentialSubject: Record<string, unknown>;
    issuer: string;
    issuanceDate: string;
    expirationDate?: string;
  };
}

/**
 * Sign a Verifiable Credential as JWT
 */
export async function signCredential(
  payload: CredentialPayload,
  issuerDid: string,
  subjectDid: string,
  expirationDays: number = 365
): Promise<string> {
  const keyPair = await getOrGenerateKeyPair(issuerDid, 'VC');
  const privateKey = await getPrivateKey(keyPair.privateKeyJwk);

  const expirationTime = Math.floor(Date.now() / 1000) + expirationDays * 24 * 60 * 60;

  return new SignJWT(payload as unknown as Record<string, unknown>)
    .setProtectedHeader({
      alg: 'EdDSA',
      typ: 'JWT',
      kid: keyPair.privateKeyJwk.kid,
    })
    .setIssuedAt()
    .setIssuer(issuerDid)
    .setSubject(subjectDid)
    .setExpirationTime(expirationTime)
    .sign(privateKey);
}

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
