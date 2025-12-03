import { SignJWT, generateKeyPair, exportJWK, decodeJwt } from 'jose';
import { getSubjectDidFromProof } from './signing';

describe('signing', () => {
  describe('getSubjectDidFromProof', () => {
    it('should extract DID from kid header with fragment', async () => {
      const { privateKey } = await generateKeyPair('EdDSA');
      const kid = 'did:web:wallet.example.com#key-1';

      const jwt = await new SignJWT({ test: 'payload' })
        .setProtectedHeader({ alg: 'EdDSA', kid })
        .sign(privateKey);

      const did = getSubjectDidFromProof(jwt);
      expect(did).toBe('did:web:wallet.example.com');
    });

    it('should handle kid without fragment', async () => {
      const { privateKey } = await generateKeyPair('EdDSA');
      const kid = 'did:web:wallet.example.com';

      const jwt = await new SignJWT({ test: 'payload' })
        .setProtectedHeader({ alg: 'EdDSA', kid })
        .sign(privateKey);

      const did = getSubjectDidFromProof(jwt);
      expect(did).toBe('did:web:wallet.example.com');
    });

    it('should return empty string if no kid in header', async () => {
      const { privateKey } = await generateKeyPair('EdDSA');

      const jwt = await new SignJWT({ test: 'payload' })
        .setProtectedHeader({ alg: 'EdDSA' })
        .sign(privateKey);

      const did = getSubjectDidFromProof(jwt);
      expect(did).toBe('');
    });

    it('should handle did:jwk format', async () => {
      const { privateKey } = await generateKeyPair('EdDSA');
      const kid = 'did:jwk:eyJrdHkiOiJPS1AifQ#0';

      const jwt = await new SignJWT({ test: 'payload' })
        .setProtectedHeader({ alg: 'EdDSA', kid })
        .sign(privateKey);

      const did = getSubjectDidFromProof(jwt);
      expect(did).toBe('did:jwk:eyJrdHkiOiJPS1AifQ');
    });
  });
});
