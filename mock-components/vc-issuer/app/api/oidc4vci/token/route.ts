import { NextRequest } from 'next/server';
import prisma from '@/lib/prisma';
import { verifyCodeChallenge, generateCNonce } from '@/lib/oid4vci/pkce';
import { signAccessToken } from '@/lib/crypto/signing';
import { getIssuerDid, getAccessTokenExpirySeconds, getCNonceExpirySeconds, jsonResponse } from '@/lib/utils';

/**
 * Token endpoint for OID4VCI
 * Exchanges authorization code for access token
 */
export async function POST(req: NextRequest) {
  const formData = await req.formData();

  const grantType = formData.get('grant_type') as string;
  const code = formData.get('code') as string;
  const codeVerifier = formData.get('code_verifier') as string;
  const clientId = formData.get('client_id') as string;
  const redirectUri = formData.get('redirect_uri') as string;

  // Validate grant type
  if (grantType !== 'authorization_code') {
    return jsonResponse(
      { error: 'unsupported_grant_type', error_description: 'Only authorization_code grant type is supported' },
      { status: 400 }
    );
  }

  // Validate required parameters
  if (!code) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'code is required' },
      { status: 400 }
    );
  }

  if (!codeVerifier) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'code_verifier is required' },
      { status: 400 }
    );
  }

  if (!clientId) {
    return jsonResponse(
      { error: 'invalid_request', error_description: 'client_id is required' },
      { status: 400 }
    );
  }

  // Find authorization request by code
  const authRequest = await prisma.authorizationRequest.findUnique({
    where: { generatedCode: code },
  });

  if (!authRequest) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'Authorization code not found' },
      { status: 400 }
    );
  }

  // Check if expired
  if (new Date() > authRequest.expiresAt) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'Authorization code has expired' },
      { status: 400 }
    );
  }

  // Check if already used
  if (authRequest.isUsed) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'Authorization code has already been used' },
      { status: 400 }
    );
  }

  // Validate client_id matches
  if (authRequest.clientId !== clientId) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'client_id does not match' },
      { status: 400 }
    );
  }

  // Validate redirect_uri matches (if provided)
  if (redirectUri && authRequest.redirectUri !== redirectUri) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'redirect_uri does not match' },
      { status: 400 }
    );
  }

  // Verify PKCE code challenge
  const validChallenge = verifyCodeChallenge(
    codeVerifier,
    authRequest.codeChallenge,
    authRequest.codeChallengeMethod
  );

  if (!validChallenge) {
    return jsonResponse(
      { error: 'invalid_grant', error_description: 'code_verifier does not match code_challenge' },
      { status: 400 }
    );
  }

  // Mark authorization request as used
  await prisma.authorizationRequest.update({
    where: { generatedCode: code },
    data: { isUsed: true },
  });

  // Generate access token
  const issuerDid = getIssuerDid(req);
  const expiresIn = getAccessTokenExpirySeconds();
  const cNonceExpiresIn = getCNonceExpirySeconds();

  const authorizationDetails = JSON.parse(authRequest.authorizationDetails);
  const accessTokenPayload = {
    authorization_details: authorizationDetails,
  };

  const accessToken = await signAccessToken(accessTokenPayload, issuerDid, clientId, expiresIn);
  const cNonce = generateCNonce();

  // Store token response
  await prisma.tokenResponse.create({
    data: {
      clientId,
      accessToken,
      cNonce,
      cNonceExpiresAt: new Date(Date.now() + cNonceExpiresIn * 1000),
      expiresAt: new Date(Date.now() + expiresIn * 1000),
      authRequestId: authRequest.id,
      authorizationDetails: authRequest.authorizationDetails,
      authenticatedOrg: authRequest.authenticatedOrg,
    },
  });

  return jsonResponse({
    access_token: accessToken,
    token_type: 'bearer',
    expires_in: expiresIn,
    c_nonce: cNonce,
    c_nonce_expires_in: cNonceExpiresIn,
    authorization_details: authorizationDetails,
  });
}
