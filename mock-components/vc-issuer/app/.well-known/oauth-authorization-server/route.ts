import { NextRequest } from 'next/server';
import { getAuthorizationServerMetadata } from '@/lib/oid4vci/metadata';
import { getBaseUrl, jsonResponse } from '@/lib/utils';

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

  return jsonResponse(metadata, {
    headers: { 'Cache-Control': 'public, max-age=3600' },
  });
}
