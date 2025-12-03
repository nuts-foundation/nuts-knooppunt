import { generateDidWeb, didWebToUrl, generateDidDocument } from './did-web';
import { JWK } from 'jose';

describe('did-web', () => {
  describe('generateDidWeb', () => {
    it('should generate did:web for simple hostname', () => {
      expect(generateDidWeb('example.com')).toBe('did:web:example.com');
    });

    it('should encode port numbers', () => {
      expect(generateDidWeb('localhost:3000')).toBe('did:web:localhost%3A3000');
    });

    it('should handle subdomains', () => {
      expect(generateDidWeb('issuer.example.com')).toBe('did:web:issuer.example.com');
    });

    it('should convert paths to colons', () => {
      expect(generateDidWeb('example.com/issuers/1')).toBe('did:web:example.com:issuers:1');
    });
  });

  describe('didWebToUrl', () => {
    it('should convert simple did:web to well-known URL', () => {
      expect(didWebToUrl('did:web:example.com')).toBe('https://example.com/.well-known/did.json');
    });

    it('should decode port numbers', () => {
      expect(didWebToUrl('did:web:localhost%3A3000')).toBe(
        'https://localhost:3000/.well-known/did.json'
      );
    });

    it('should handle paths in did:web', () => {
      expect(didWebToUrl('did:web:example.com:issuers:1')).toBe(
        'https://example.com/issuers/1/did.json'
      );
    });

    it('should throw for invalid did:web format', () => {
      expect(() => didWebToUrl('did:key:z123')).toThrow('Invalid did:web format');
      expect(() => didWebToUrl('not-a-did')).toThrow('Invalid did:web format');
    });
  });

  describe('generateDidDocument', () => {
    const mockPublicKeyJwk: JWK = {
      kty: 'OKP',
      crv: 'Ed25519',
      x: 'test-public-key-x',
      alg: 'EdDSA',
      use: 'sig',
      kid: 'did:web:example.com#key-1',
    };

    it('should generate a valid DID document structure', () => {
      const did = 'did:web:example.com';
      const doc = generateDidDocument(did, mockPublicKeyJwk);

      expect(doc).toHaveProperty('@context');
      expect(doc).toHaveProperty('id', did);
      expect(doc).toHaveProperty('verificationMethod');
      expect(doc).toHaveProperty('authentication');
      expect(doc).toHaveProperty('assertionMethod');
    });

    it('should include the public key in verificationMethod', () => {
      const did = 'did:web:example.com';
      const doc = generateDidDocument(did, mockPublicKeyJwk) as {
        verificationMethod: Array<{
          id: string;
          type: string;
          controller: string;
          publicKeyJwk: JWK;
        }>;
      };

      expect(doc.verificationMethod).toHaveLength(1);
      expect(doc.verificationMethod[0].type).toBe('JsonWebKey2020');
      expect(doc.verificationMethod[0].controller).toBe(did);
      expect(doc.verificationMethod[0].publicKeyJwk.kty).toBe('OKP');
      expect(doc.verificationMethod[0].publicKeyJwk.crv).toBe('Ed25519');
    });

    it('should use kid from JWK if provided', () => {
      const did = 'did:web:example.com';
      const doc = generateDidDocument(did, mockPublicKeyJwk) as {
        verificationMethod: Array<{ id: string }>;
        authentication: string[];
      };

      expect(doc.verificationMethod[0].id).toBe('did:web:example.com#key-1');
      expect(doc.authentication).toContain('did:web:example.com#key-1');
    });

    it('should generate default key id if kid not in JWK', () => {
      const did = 'did:web:example.com';
      const jwkWithoutKid: JWK = { ...mockPublicKeyJwk };
      delete jwkWithoutKid.kid;

      const doc = generateDidDocument(did, jwkWithoutKid) as {
        verificationMethod: Array<{ id: string }>;
      };

      expect(doc.verificationMethod[0].id).toBe(`${did}#key-1`);
    });
  });
});
