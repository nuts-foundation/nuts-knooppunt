import { NextRequest, NextResponse } from 'next/server';
import { getCredentialIssuerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl, getIssuerDid } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const baseUrl = getBaseUrl(req);
  const issuerDid = getIssuerDid(req);

  const metadata = getCredentialIssuerMetadata(baseUrl, issuerDid);

  return NextResponse.json(metadata, {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}
