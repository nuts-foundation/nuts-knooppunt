import { NextRequest, NextResponse } from 'next/server';
import { getAuthorizationServerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl } from '@/lib/utils';

export async function GET(req: NextRequest) {
  const baseUrl = getBaseUrl(req);

  const metadata = getAuthorizationServerMetadata(baseUrl);

  return NextResponse.json(metadata, {
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'public, max-age=3600',
    },
  });
}
