import { generateEd25519KeyPair, getPrivateKey, getPublicKey } from './ed25519';
import { SignJWT, jwtVerify } from 'jose';

describe('ed25519', () => {
  describe('generateEd25519KeyPair', () => {
    it('should generate a valid Ed25519 key pair', async () => {
      const did = 'did:web:example.com';
      const keyPair = await generateEd25519KeyPair(did);

      expect(keyPair.publicKeyJwk).toBeDefined();
      expect(keyPair.privateKeyJwk).toBeDefined();
      expect(keyPair.thumbprint).toBeDefined();
    });

    it('should set correct key properties', async () => {
      const did = 'did:web:example.com';
      const keyPair = await generateEd25519KeyPair(did);

      expect(keyPair.publicKeyJwk.kty).toBe('OKP');
      expect(keyPair.publicKeyJwk.crv).toBe('Ed25519');
      expect(keyPair.publicKeyJwk.alg).toBe('EdDSA');
      expect(keyPair.publicKeyJwk.use).toBe('sig');
    });

    it('should set kid with DID and thumbprint', async () => {
      const did = 'did:web:example.com';
      const keyPair = await generateEd25519KeyPair(did);

      expect(keyPair.publicKeyJwk.kid).toBe(`${did}#${keyPair.thumbprint}`);
      expect(keyPair.privateKeyJwk.kid).toBe(`${did}#${keyPair.thumbprint}`);
    });

    it('should generate unique key pairs', async () => {
      const did = 'did:web:example.com';
      const keyPair1 = await generateEd25519KeyPair(did);
      const keyPair2 = await generateEd25519KeyPair(did);

      expect(keyPair1.thumbprint).not.toBe(keyPair2.thumbprint);
    });
  });

  describe('getPrivateKey / getPublicKey', () => {
    it('should import keys that can sign and verify', async () => {
      const did = 'did:web:example.com';
      const keyPair = await generateEd25519KeyPair(did);

      const privateKey = await getPrivateKey(keyPair.privateKeyJwk);
      const publicKey = await getPublicKey(keyPair.publicKeyJwk);

      // Sign with private key
      const jwt = await new SignJWT({ test: 'payload' })
        .setProtectedHeader({ alg: 'EdDSA' })
        .sign(privateKey);

      // Verify with public key
      const { payload } = await jwtVerify(jwt, publicKey);
      expect(payload).toHaveProperty('test', 'payload');
    });
  });
});
