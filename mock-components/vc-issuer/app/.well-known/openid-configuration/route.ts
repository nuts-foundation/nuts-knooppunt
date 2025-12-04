import { NextRequest } from 'next/server';
import { getAuthorizationServerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl, jsonResponse } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const baseUrl = getBaseUrl(req);
  const metadata = getAuthorizationServerMetadata(baseUrl);

  return jsonResponse(metadata, {
    headers: { 'Cache-Control': 'public, max-age=3600' },
  });
}
