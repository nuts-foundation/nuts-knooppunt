/**
 * Local credential signing using Ed25519 keys
 */

import { SignJWT } from 'jose';
import { getPrivateKey, getOrGenerateKeyPair } from '../crypto/ed25519';
import { CredentialRequest } from './types';

/**
 * Issue a credential using local Ed25519 signing
 */
export async function issueCredentialLocally(
  request: CredentialRequest
): Promise<string> {
  console.log('[LocalIssuer] Signing credential locally');
  console.log('[LocalIssuer] Credential ID:', request.credentialId);
  console.log('[LocalIssuer] Issuer DID:', request.issuerDid);

  const keyPair = await getOrGenerateKeyPair(request.issuerDid, 'VC');
  const privateKey = await getPrivateKey(keyPair.privateKeyJwk);

  const expirationTime = Math.floor(request.expirationDate.getTime() / 1000);
  const issuedAt = Math.floor(request.issuanceDate.getTime() / 1000);

  const payload = {
    vc: {
      '@context': request.context,
      id: request.credentialId,
      type: request.type,
      credentialSubject: {
        id: request.subjectDid,
        ...request.credentialSubject,
      },
      issuer: request.issuerDid,
      issuanceDate: request.issuanceDate.toISOString(),
      expirationDate: request.expirationDate.toISOString(),
    },
  };

  const signedCredential = await new SignJWT(payload as unknown as Record<string, unknown>)
    .setProtectedHeader({
      alg: 'EdDSA',
      typ: 'JWT',
      kid: keyPair.privateKeyJwk.kid,
    })
    .setIssuedAt(issuedAt)
    .setIssuer(request.issuerDid)
    .setSubject(request.subjectDid)
    .setExpirationTime(expirationTime)
    .sign(privateKey);

  console.log('[LocalIssuer] Credential signed successfully');

  return signedCredential;
}

