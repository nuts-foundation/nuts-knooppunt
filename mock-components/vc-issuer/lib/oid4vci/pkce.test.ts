import crypto from 'crypto';
import {
  verifyCodeChallenge,
  generateAuthorizationCode,
  generateCNonce,
  generateState,
} from './pkce';

describe('PKCE', () => {
  describe('verifyCodeChallenge', () => {
    it('should verify plain method correctly', () => {
      const verifier = 'test-code-verifier';
      expect(verifyCodeChallenge(verifier, verifier, 'plain')).toBe(true);
      expect(verifyCodeChallenge(verifier, 'wrong', 'plain')).toBe(false);
    });

    it('should verify S256 method correctly', () => {
      const verifier = 'dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk';

      // Compute expected challenge
      const hash = crypto.createHash('sha256').update(verifier).digest();
      const expectedChallenge = hash
        .toString('base64')
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=/g, '');

      expect(verifyCodeChallenge(verifier, expectedChallenge, 'S256')).toBe(true);
      expect(verifyCodeChallenge(verifier, 'wrong-challenge', 'S256')).toBe(false);
    });

    it('should reject unknown methods', () => {
      expect(verifyCodeChallenge('verifier', 'challenge', 'unknown')).toBe(false);
    });
  });

  describe('generateAuthorizationCode', () => {
    it('should generate a base64url encoded string', () => {
      const code = generateAuthorizationCode();
      expect(code).toMatch(/^[A-Za-z0-9_-]+$/);
    });

    it('should generate unique codes', () => {
      const code1 = generateAuthorizationCode();
      const code2 = generateAuthorizationCode();
      expect(code1).not.toBe(code2);
    });

    it('should generate codes of consistent length (32 bytes = ~43 chars base64url)', () => {
      const code = generateAuthorizationCode();
      expect(code.length).toBeGreaterThanOrEqual(40);
      expect(code.length).toBeLessThanOrEqual(45);
    });
  });

  describe('generateCNonce', () => {
    it('should generate a base64url encoded string', () => {
      const nonce = generateCNonce();
      expect(nonce).toMatch(/^[A-Za-z0-9_-]+$/);
    });

    it('should generate unique nonces', () => {
      const nonce1 = generateCNonce();
      const nonce2 = generateCNonce();
      expect(nonce1).not.toBe(nonce2);
    });
  });

  describe('generateState', () => {
    it('should generate a base64url encoded string', () => {
      const state = generateState();
      expect(state).toMatch(/^[A-Za-z0-9_-]+$/);
    });

    it('should generate unique states', () => {
      const state1 = generateState();
      const state2 = generateState();
      expect(state1).not.toBe(state2);
    });
  });
});
