import { NextRequest } from 'next/server';
import { v4 as uuidv4 } from 'uuid';
import prisma from '@/lib/prisma';
import { signCredential, getSubjectDidFromProof, verifyAccessToken } from '@/lib/crypto/signing';
import { generateCNonce } from '@/lib/oid4vci/pkce';
import { getIssuerDid, getBaseUrl, getCredentialValidityDays, getCNonceExpirySeconds, jsonResponse } from '@/lib/utils';

interface AuthenticatedOrg {
  id: string;
  name: string;
  type: string;
  typeLabel: string;
  agbCode: string;
  uraNumber: string;
}

interface CredentialRequest {
  format: string;
  credential_definition?: {
    type?: string[];
    '@context'?: string[];
  };
  proof?: {
    proof_type: string;
    jwt: string;
  };
}

/**
 * Credential endpoint for OID4VCI
 * Issues VektisOrgCredential to the wallet
 */
export async function POST(req: NextRequest) {
  console.log('[Credential] POST request received');

  const issuerDid = getIssuerDid(req);
  const baseUrl = getBaseUrl(req);
  console.log('[Credential] issuerDid:', issuerDid);
  console.log('[Credential] baseUrl:', baseUrl);

  // Extract and verify access token
  const authorization = req.headers.get('Authorization');
  if (!authorization || !authorization.startsWith('Bearer ')) {
    console.log('[Credential] ERROR: Missing or invalid Authorization header');
    return jsonResponse(
      { error: 'invalid_token', error_description: 'Missing or invalid Authorization header' },
      { status: 401 }
    );
  }

  const accessToken = authorization.slice('Bearer '.length);
  console.log('[Credential] Access token received (first 20 chars):', accessToken.substring(0, 20) + '...');

  // Find token response
  const tokenResponse = await prisma.tokenResponse.findUnique({
    where: { accessToken },
  });

  if (!tokenResponse) {
    console.log('[Credential] ERROR: Access token not found in database');
    return jsonResponse(
      { error: 'invalid_token', error_description: 'Access token not found' },
      { status: 401 }
    );
  }
  console.log('[Credential] Token found, id:', tokenResponse.id);

  // Check if token is expired
  if (new Date() > tokenResponse.expiresAt) {
    console.log('[Credential] ERROR: Access token has expired. ExpiresAt:', tokenResponse.expiresAt);
    return jsonResponse(
      { error: 'invalid_token', error_description: 'Access token has expired' },
      { status: 401 }
    );
  }

  // Check if token is revoked
  if (tokenResponse.isRevoked) {
    console.log('[Credential] ERROR: Access token has been revoked');
    return jsonResponse(
      { error: 'invalid_token', error_description: 'Access token has been revoked' },
      { status: 401 }
    );
  }

  // Verify the access token signature
  try {
    await verifyAccessToken(accessToken, issuerDid);
    console.log('[Credential] Access token signature verified');
  } catch (err) {
    console.log('[Credential] ERROR: Access token signature verification failed:', err);
    return jsonResponse(
      { error: 'invalid_token', error_description: 'Access token signature verification failed' },
      { status: 401 }
    );
  }

  // Parse credential request
  let credentialRequest: CredentialRequest;
  try {
    credentialRequest = await req.json();
    console.log('[Credential] Request body:', JSON.stringify(credentialRequest, null, 2));
  } catch (err) {
    console.log('[Credential] ERROR: Failed to parse request body:', err);
    return jsonResponse(
      { error: 'invalid_request', error_description: 'Invalid JSON in request body' },
      { status: 400 }
    );
  }

  // Default format to jwt_vc_json if not provided (per credential_configurations_supported)
  const format = credentialRequest.format || 'jwt_vc_json';
  console.log('[Credential] Format:', format, credentialRequest.format ? '(provided)' : '(defaulted)');

  // Validate format
  if (format !== 'jwt_vc_json') {
    console.log('[Credential] ERROR: Unsupported format:', format);
    return jsonResponse(
      { error: 'unsupported_credential_format', error_description: 'Only jwt_vc_json format is supported' },
      { status: 400 }
    );
  }

  // Validate proof
  if (!credentialRequest.proof || !credentialRequest.proof.jwt) {
    console.log('[Credential] ERROR: Missing proof or proof.jwt. proof:', JSON.stringify(credentialRequest.proof));
    return jsonResponse(
      { error: 'invalid_proof', error_description: 'Proof JWT is required' },
      { status: 400 }
    );
  }
  console.log('[Credential] Proof type:', credentialRequest.proof.proof_type);

  // Extract subject DID from proof
  const subjectDid = getSubjectDidFromProof(credentialRequest.proof.jwt);
  if (!subjectDid) {
    console.log('[Credential] ERROR: Could not extract subject DID from proof JWT');
    return jsonResponse(
      { error: 'invalid_proof', error_description: 'Could not extract subject DID from proof' },
      { status: 400 }
    );
  }
  console.log('[Credential] Subject DID extracted:', subjectDid);

  // Get authenticated organization from token response
  if (!tokenResponse.authenticatedOrg) {
    console.log('[Credential] ERROR: No organization data found for token. authenticatedOrg:', tokenResponse.authenticatedOrg);
    return jsonResponse(
      { error: 'invalid_request', error_description: 'No organization data found for this token' },
      { status: 400 }
    );
  }
  const authenticatedOrg = JSON.parse(tokenResponse.authenticatedOrg) as AuthenticatedOrg;
  console.log('[Credential] Authenticated organization:', authenticatedOrg.name);

  // Build credential
  const credentialId = `urn:uuid:${uuidv4()}`;
  const issuanceDate = new Date();
  const validityDays = getCredentialValidityDays();
  const expirationDate = new Date(issuanceDate.getTime() + validityDays * 24 * 60 * 60 * 1000);

  const credentialSubject = {
    id: subjectDid,
    organizationName: authenticatedOrg.name,
    organizationType: authenticatedOrg.type,
    agbCode: authenticatedOrg.agbCode,
    uraNumber: authenticatedOrg.uraNumber,
  };

  const credentialPayload = {
    vc: {
      '@context': [
        'https://www.w3.org/2018/credentials/v1',
        `${baseUrl}/contexts/vektis-org.jsonld`,
      ],
      id: credentialId,
      type: ['VerifiableCredential', 'VektisOrgCredential'],
      credentialSubject,
      issuer: issuerDid,
      issuanceDate: issuanceDate.toISOString(),
      expirationDate: expirationDate.toISOString(),
    },
  };

  // Sign the credential
  console.log('[Credential] Signing credential with id:', credentialId);
  let signedCredential: string;
  try {
    signedCredential = await signCredential(
      credentialPayload,
      issuerDid,
      subjectDid,
      validityDays
    );
    console.log('[Credential] Credential signed successfully');
  } catch (err) {
    console.log('[Credential] ERROR: Failed to sign credential:', err);
    return jsonResponse(
      { error: 'server_error', error_description: 'Failed to sign credential' },
      { status: 500 }
    );
  }

  // Generate new c_nonce
  const newCNonce = generateCNonce();
  const cNonceExpiresIn = getCNonceExpirySeconds();

  // Store issued credential
  console.log('[Credential] Storing issued credential in database');
  await prisma.issuedCredential.create({
    data: {
      credentialId,
      issuerDid,
      subjectDid,
      credentialType: JSON.stringify(['VerifiableCredential', 'VektisOrgCredential']),
      format: 'jwt_vc_json',
      credentialSubject: JSON.stringify(credentialSubject),
      tokenResponseId: tokenResponse.id,
      cNonceUsed: tokenResponse.cNonce,
      issuanceDate,
      expirationDate,
    },
  });

  // Update token response
  await prisma.tokenResponse.update({
    where: { id: tokenResponse.id },
    data: {
      credentialsIssued: { increment: 1 },
      cNonce: newCNonce,
      cNonceExpiresAt: new Date(Date.now() + cNonceExpiresIn * 1000),
    },
  });

  console.log('[Credential] SUCCESS: Credential issued for', authenticatedOrg.name, 'to subject', subjectDid);

  return jsonResponse({
    format: 'jwt_vc_json',
    credential: signedCredential,
    c_nonce: newCNonce,
    c_nonce_expires_in: cNonceExpiresIn,
  });
}
