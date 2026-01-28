import { NextRequest } from 'next/server';
import { getOrGenerateKeyPair } from '@/lib/crypto/ed25519';
import { generateDidDocument } from '@/lib/crypto/did-web';
import { getIssuerDid, jsonResponse } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const issuerDid = getIssuerDid(req);
  const keyPair = await getOrGenerateKeyPair(issuerDid, 'VC');
  const didDocument = generateDidDocument(issuerDid, keyPair.publicKeyJwk);

  return jsonResponse(didDocument, {
    headers: { 'Cache-Control': 'public, max-age=3600' },
  });
}
