import crypto from 'crypto';

/**
 * Verify PKCE code challenge
 *
 * @param codeVerifier - The code verifier from the token request
 * @param codeChallenge - The code challenge stored from the authorization request
 * @param method - The challenge method (S256 or plain)
 * @returns true if the challenge is valid
 */
export function verifyCodeChallenge(
  codeVerifier: string,
  codeChallenge: string,
  method: string
): boolean {
  if (method === 'plain') {
    return codeVerifier === codeChallenge;
  }

  if (method === 'S256') {
    // SHA256 hash of code_verifier, base64url encoded
    const computedHash = crypto.createHash('sha256').update(codeVerifier).digest();

    const computedChallenge = computedHash
      .toString('base64')
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=/g, '');

    return computedChallenge === codeChallenge;
  }

  // Unknown method
  return false;
}

/**
 * Generate a random code for authorization
 */
export function generateAuthorizationCode(): string {
  return crypto.randomBytes(32).toString('base64url');
}

/**
 * Generate a c_nonce for credential requests
 */
export function generateCNonce(): string {
  return crypto.randomBytes(16).toString('base64url');
}

/**
 * Generate a random state parameter
 */
export function generateState(): string {
  return crypto.randomBytes(16).toString('base64url');
}
