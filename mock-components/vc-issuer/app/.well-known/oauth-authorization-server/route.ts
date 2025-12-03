import { NextRequest, NextResponse } from 'next/server';
import { getAuthorizationServerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl } from '@/lib/utils';

/**
 * OAuth 2.0 Authorization Server Metadata (RFC 8414)
 * https://www.rfc-editor.org/rfc/rfc8414.html
 *
 * This endpoint returns the authorization server metadata at:
 * /.well-known/oauth-authorization-server
 */
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
