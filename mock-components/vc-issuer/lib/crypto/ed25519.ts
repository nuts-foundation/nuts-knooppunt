import { generateKeyPair, exportJWK, importJWK, calculateJwkThumbprint, KeyLike, JWK } from 'jose';
import prisma from '@/lib/prisma';

export interface KeyPairResult {
  publicKeyJwk: JWK;
  privateKeyJwk: JWK;
  thumbprint: string;
}

/**
 * Generate a new Ed25519 key pair
 */
export async function generateEd25519KeyPair(did: string): Promise<KeyPairResult> {
  const { publicKey, privateKey } = await generateKeyPair('EdDSA', { crv: 'Ed25519' });

  const publicKeyJwk = await exportJWK(publicKey);
  const privateKeyJwk = await exportJWK(privateKey);

  // Calculate thumbprint for key identification
  const thumbprint = await calculateJwkThumbprint(publicKeyJwk, 'sha256');

  // Add algorithm and key use
  publicKeyJwk.alg = 'EdDSA';
  publicKeyJwk.use = 'sig';
  privateKeyJwk.alg = 'EdDSA';
  privateKeyJwk.use = 'sig';

  // Set kid to DID fragment
  const kid = `${did}#${thumbprint}`;
  publicKeyJwk.kid = kid;
  privateKeyJwk.kid = kid;

  return { publicKeyJwk, privateKeyJwk, thumbprint };
}

/**
 * Get or generate a key pair for a DID
 */
export async function getOrGenerateKeyPair(
  did: string,
  keyType: string = 'VC'
): Promise<KeyPairResult> {
  // Check if key pair already exists
  const existing = await prisma.didKeyPair.findUnique({
    where: {
      uniqueDidKeyType: {
        did,
        keyType,
      },
    },
  });

  if (existing) {
    return {
      publicKeyJwk: JSON.parse(existing.publicKeyJwk) as JWK,
      privateKeyJwk: JSON.parse(existing.privateKeyJwk) as JWK,
      thumbprint: existing.thumbprint,
    };
  }

  // Generate new key pair
  const keyPair = await generateEd25519KeyPair(did);

  // Store in database
  await prisma.didKeyPair.create({
    data: {
      did,
      keyType,
      algorithm: 'EdDSA',
      curve: 'Ed25519',
      publicKeyJwk: JSON.stringify(keyPair.publicKeyJwk),
      privateKeyJwk: JSON.stringify(keyPair.privateKeyJwk),
      thumbprint: keyPair.thumbprint,
    },
  });

  return keyPair;
}

/**
 * Import a private key from JWK for signing
 */
export async function getPrivateKey(privateKeyJwk: JWK): Promise<KeyLike> {
  const key = await importJWK(privateKeyJwk, 'EdDSA');
  return key as KeyLike;
}

/**
 * Import a public key from JWK for verification
 */
export async function getPublicKey(publicKeyJwk: JWK): Promise<KeyLike> {
  const key = await importJWK(publicKeyJwk, 'EdDSA');
  return key as KeyLike;
}
