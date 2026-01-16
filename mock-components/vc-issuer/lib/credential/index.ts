/**
 * Unified credential issuance interface
 * Automatically selects between local signing and Nuts node based on configuration
 */

import { CredentialRequest, getIssuanceConfig } from './types';
import { issueCredentialLocally } from './local';
import { issueCredentialViaNuts } from './nuts';

/**
 * Issue a credential using the configured method (local or Nuts node)
 *
 * @param request - Credential request parameters
 * @returns Signed credential as JWT string
 * @throws Error if Nuts node is configured but NUTS_ISSUER_DID is not set
 */
export async function issueCredential(
  request: CredentialRequest
): Promise<string> {
  const config = getIssuanceConfig();

  console.log('[CredentialIssuer] Mode:', config.mode);

  if (config.mode === 'nuts') {
    if (!config.nutsNodeUrl) {
      throw new Error('NUTS_NODE_INTERNAL_URL is not configured');
    }
    if (!process.env.NUTS_ISSUER_DID) {
      throw new Error('NUTS_ISSUER_DID is required when using Nuts node for credential issuance');
    }

    return issueCredentialViaNuts(request, config.nutsNodeUrl);
  }

  return issueCredentialLocally(request);
}

// Re-export types and utilities
export type { CredentialRequest } from './types';
export { getIssuanceConfig } from './types';

