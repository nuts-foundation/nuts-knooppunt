import { NextRequest } from 'next/server';
import { generateDidWeb } from './crypto/did-web';

/**
 * Get the issuer hostname from request headers, falling back to environment variable.
 * This is the single source of truth for the issuer's identity.
 */
export function getIssuerHostname(req?: NextRequest): string {
  if (req) {
    const host = req.headers.get('host') || req.headers.get('x-forwarded-host');
    if (host) {
      return host;
    }
  }

  return process.env.ISSUER_HOSTNAME || 'localhost:3000';
}

/**
 * Get the base URL from request headers, falling back to ISSUER_HOSTNAME env var.
 * Protocol is determined from x-forwarded-proto header or defaults to https.
 */
export function getBaseUrl(req?: NextRequest): string {
  if (req) {
    const host = req.headers.get('host') || req.headers.get('x-forwarded-host');
    if (host) {
      const protocol = req.headers.get('x-forwarded-proto') || 'https';
      return `${protocol}://${host}`;
    }
  }

  // Fall back to ISSUER_HOSTNAME with https protocol
  const hostname = process.env.ISSUER_HOSTNAME || 'localhost:3000';
  const isLocalhost = hostname.startsWith('localhost');
  const protocol = isLocalhost ? 'http' : 'https';
  return `${protocol}://${hostname}`;
}

/**
 * Get the issuer DID from request hostname
 */
export function getIssuerDid(req?: NextRequest): string {
  const hostname = getIssuerHostname(req);
  return generateDidWeb(hostname);
}

/**
 * Get credential validity in days from environment
 */
export function getCredentialValidityDays(): number {
  const days = process.env.CREDENTIAL_VALIDITY_DAYS;
  return days ? parseInt(days, 10) : 365;
}

/**
 * Get access token expiry in seconds from environment
 */
export function getAccessTokenExpirySeconds(): number {
  const seconds = process.env.ACCESS_TOKEN_EXPIRY_SECONDS;
  return seconds ? parseInt(seconds, 10) : 86400;
}

/**
 * Get c_nonce expiry in seconds from environment
 */
export function getCNonceExpirySeconds(): number {
  const seconds = process.env.C_NONCE_EXPIRY_SECONDS;
  return seconds ? parseInt(seconds, 10) : 86400;
}
