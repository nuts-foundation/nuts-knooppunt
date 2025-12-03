import { NextRequest, NextResponse } from 'next/server';
import prisma from '@/lib/prisma';
import { generateAuthorizationCode, generateState } from '@/lib/oid4vci/pkce';
import { getBaseUrl } from '@/lib/utils';

/**
 * Authorization endpoint for OID4VCI
 * Handles the initial authorization request from the wallet
 */
export async function GET(req: NextRequest) {
  const searchParams = req.nextUrl.searchParams;

  // Extract OAuth2 parameters
  const responseType = searchParams.get('response_type');
  const clientId = searchParams.get('client_id');
  const redirectUri = searchParams.get('redirect_uri');
  const codeChallenge = searchParams.get('code_challenge');
  const codeChallengeMethod = searchParams.get('code_challenge_method');
  const authorizationDetailsStr = searchParams.get('authorization_details');
  const state = searchParams.get('state') || generateState();
  const issuerState = searchParams.get('issuer_state');

  // Validate required parameters
  if (responseType !== 'code') {
    return NextResponse.json(
      { error: 'unsupported_response_type', error_description: 'Only code response type is supported' },
      { status: 400 }
    );
  }

  if (!clientId) {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'client_id is required' },
      { status: 400 }
    );
  }

  if (!redirectUri) {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'redirect_uri is required' },
      { status: 400 }
    );
  }

  if (!codeChallenge) {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'code_challenge is required (PKCE)' },
      { status: 400 }
    );
  }

  if (codeChallengeMethod !== 'S256' && codeChallengeMethod !== 'plain') {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'code_challenge_method must be S256 or plain' },
      { status: 400 }
    );
  }

  if (!authorizationDetailsStr) {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'authorization_details is required' },
      { status: 400 }
    );
  }

  // Parse authorization_details
  let authorizationDetails;
  try {
    authorizationDetails = JSON.parse(authorizationDetailsStr);
  } catch {
    return NextResponse.json(
      { error: 'invalid_request', error_description: 'Invalid authorization_details JSON' },
      { status: 400 }
    );
  }

  // Generate authorization code (will be used after e-Herkenning login)
  const generatedCode = generateAuthorizationCode();

  // Store authorization request
  const expiresAt = new Date(Date.now() + 10 * 60 * 1000); // 10 minutes expiry

  await prisma.authorizationRequest.create({
    data: {
      clientId,
      responseType,
      redirectUri,
      state,
      codeChallenge,
      codeChallengeMethod,
      authorizationDetails: JSON.stringify(authorizationDetails),
      issuerState,
      generatedCode,
      expiresAt,
    },
  });

  // Redirect to mock e-Herkenning login page
  const baseUrl = getBaseUrl(req);
  const loginUrl = new URL('/e-herkenning', baseUrl);
  loginUrl.searchParams.set('state', state);

  return NextResponse.redirect(loginUrl.toString());
}
