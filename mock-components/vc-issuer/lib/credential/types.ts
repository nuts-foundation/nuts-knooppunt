/**
 * Credential Issuance Package
 * Provides unified interface for credential issuance via local signing or Nuts node
 */

/**
 * Credential request parameters
 */
export interface CredentialRequest {
  credentialId: string;
  issuerDid: string;
  subjectDid: string;
  credentialSubject: Record<string, unknown>;
  context: string[];
  type: string[];
  issuanceDate: Date;
  expirationDate: Date;
}

/**
 * Configuration for credential issuance
 */
export interface IssuanceConfig {
  mode: 'local' | 'nuts';
  nutsNodeUrl?: string;
}

/**
 * Get the issuance configuration based on environment
 */
export function getIssuanceConfig(): IssuanceConfig {
  const nutsNodeUrl = process.env.NUTS_NODE_INTERNAL_URL;

  if (nutsNodeUrl) {
    return {
      mode: 'nuts',
      nutsNodeUrl,
    };
  }

  return {
    mode: 'local',
  };
}

