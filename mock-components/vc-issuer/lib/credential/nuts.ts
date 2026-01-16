/**
 * Nuts node credential issuance
 */

import { CredentialRequest } from './types';

interface NutsIssueRequest {
  type: string;
  issuer: string;
  credentialSubject: Record<string, unknown>;
  '@context'?: string[];
  expirationDate?: string;
  withStatusList2021Revocation: boolean;
  format: string;
}

/**
 * Issue a credential via the Nuts node
 */
export async function issueCredentialViaNuts(
  request: CredentialRequest,
  nutsNodeUrl: string
): Promise<string> {
  const endpoint = `${nutsNodeUrl}/internal/vcr/v2/issuer/vc`;

  console.log('[NutsIssuer] Issuing credential via Nuts node');
  console.log('[NutsIssuer] Endpoint:', endpoint);
  console.log('[NutsIssuer] Issuer DID:', request.issuerDid);

  const nutsRequest: NutsIssueRequest = {
    type: request.type[request.type.length - 1], // Use the most specific type
    issuer: request.issuerDid,
    credentialSubject: {
      id: request.subjectDid,
      ...request.credentialSubject,
    },
    '@context': request.context,
    expirationDate: request.expirationDate.toISOString(),
    format: "jwt_vc",
    withStatusList2021Revocation: true
  };

  console.log('[NutsIssuer] Request:', JSON.stringify(nutsRequest, null, 2));

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(nutsRequest),
  });

  if (!response.ok) {
    const errorText = await response.text();
    console.log('[NutsIssuer] ERROR: Failed to issue credential via Nuts node');
    console.log('[NutsIssuer] Status:', response.status);
    console.log('[NutsIssuer] Response:', errorText);
    throw new Error(`Failed to issue credential via Nuts node: ${response.status} ${errorText}`);
  }

  const result: string = await response.json();
  console.log('[NutsIssuer] Successfully issued credential via Nuts node');

  return result;
}

