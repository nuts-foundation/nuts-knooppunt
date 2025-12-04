import { NextRequest, NextResponse } from 'next/server';
import prisma from '@/lib/prisma';
import { getOrganizationById } from '@/lib/mock-data/organizations';
import { jsonResponse } from '@/lib/utils';

/**
 * Callback endpoint after e-Herkenning authentication
 * Updates the authorization request with selected organization and redirects to wallet
 */
export async function GET(req: NextRequest) {
  const searchParams = req.nextUrl.searchParams;

  const state = searchParams.get('state');
  const orgId = searchParams.get('org');

  if (!state) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'state is required' },
      { status: 400 }
    );
  }

  if (!orgId) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'org is required' },
      { status: 400 }
    );
  }

  // Find the authorization request
  const authRequest = await prisma.authorizationRequest.findUnique({
    where: { state },
  });

  if (!authRequest) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request not found' },
      { status: 400 }
    );
  }

  // Check if expired
  if (new Date() > authRequest.expiresAt) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request has expired' },
      { status: 400 }
    );
  }

  // Check if already used
  if (authRequest.isUsed) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Authorization request has already been used' },
      { status: 400 }
    );
  }

  // Get the selected organization
  const organization = getOrganizationById(orgId);
  if (!organization) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Organization not found' },
      { status: 400 }
    );
  }

  // Update authorization request with authenticated organization
  await prisma.authorizationRequest.update({
    where: { state },
    data: {
      authenticatedOrg: JSON.stringify({
        id: organization.id,
        name: organization.name,
        type: organization.type,
        typeLabel: organization.typeLabel,
        agbCode: organization.agbCode,
        uraNumber: organization.uraNumber,
      }),
    },
  });

  // Build redirect URL with authorization code
  const redirectUrl = new URL(authRequest.redirectUri);
  redirectUrl.searchParams.set('code', authRequest.generatedCode);
  if (authRequest.state) {
    redirectUrl.searchParams.set('state', authRequest.state);
  }

  return NextResponse.redirect(redirectUrl.toString());
}
