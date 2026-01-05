import { NextRequest } from 'next/server';
import { getCredentialIssuerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl, getIssuerDid, jsonResponse } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const baseUrl = getBaseUrl(req);
  const issuerDid = getIssuerDid(req);
  const metadata = getCredentialIssuerMetadata(baseUrl, issuerDid);

  return jsonResponse(metadata, {
    headers: { 'Cache-Control': 'public, max-age=3600' },
  });
}
