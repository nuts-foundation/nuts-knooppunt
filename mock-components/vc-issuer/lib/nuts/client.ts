/**
 * Nuts Node API Client
 * Handles communication with a Nuts node for credential issuance
 */

export interface NutsVCRequest {
  type: string;
  issuer: string;
  credentialSubject: Record<string, unknown>;
  '@context'?: string[];
  expirationDate?: string;
}

export interface NutsVCResponse {
  credential: string;
}

/**
 * Issue a Verifiable Credential via the Nuts node
 */
export async function issueCredentialViaNuts(
  nutsNodeUrl: string,
  request: NutsVCRequest
): Promise<string> {
  const endpoint = `${nutsNodeUrl}/internal/vcr/v2/issuer/vc`;

  console.log('[NutsClient] Issuing credential via Nuts node:', endpoint);
  console.log('[NutsClient] Request:', JSON.stringify(request, null, 2));

  const response = await fetch(endpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
  });

  if (!response.ok) {
    const errorText = await response.text();
    console.log('[NutsClient] ERROR: Failed to issue credential via Nuts node');
    console.log('[NutsClient] Status:', response.status);
    console.log('[NutsClient] Response:', errorText);
    throw new Error(`Failed to issue credential via Nuts node: ${response.status} ${errorText}`);
  }

  const result: NutsVCResponse = await response.json();
  console.log('[NutsClient] Successfully issued credential via Nuts node');

  return result.credential;
}

/**
 * Check if Nuts node integration is enabled
 */
export function isNutsNodeEnabled(): boolean {
  return !!process.env.NUTS_NODE_INTERNAL_URL;
}

/**
 * Get the Nuts node internal URL
 */
export function getNutsNodeUrl(): string | undefined {
  return process.env.NUTS_NODE_INTERNAL_URL;
}

/**
 * Get the configured issuer DID for Nuts node
 */
export function getNutsIssuerDid(): string | undefined {
  return process.env.NUTS_ISSUER_DID;
}

