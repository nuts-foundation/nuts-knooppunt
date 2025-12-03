import { NextRequest, NextResponse } from 'next/server';
import { getOrGenerateKeyPair } from '@/lib/crypto/ed25519';
import { generateDidDocument } from '@/lib/crypto/did-web';
import { getIssuerDid } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const issuerDid = getIssuerDid(req);

  // Get or generate the key pair
  const keyPair = await getOrGenerateKeyPair(issuerDid, 'VC');

  // Generate the DID document
  const didDocument = generateDidDocument(issuerDid, keyPair.publicKeyJwk);

  return NextResponse.json(didDocument, {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}
