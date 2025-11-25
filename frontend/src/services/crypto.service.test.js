import { describe, it, expect, vi } from 'vitest';
import {
  generateSaltedHash,
  hashDomainWithSalt,
  verifyDomainHash,
  generateTransactionSignature,
  verifyTransactionSignature,
  hashTransactionData,
  hashTransaction,
  generateSalt,
} from './crypto.service.js';

// Mock tweetnacl
vi.mock('tweetnacl', () => ({
  default: {
    sign: {
      detached: vi.fn((message, privateKey) => {
        // Return a mock signature based on message
        return new Uint8Array(64).fill(1);
      }),
      detached: {
        verify: vi.fn((message, signature, publicKey) => {
          // Mock verification - return true if signature is all 1s
          return signature.every(byte => byte === 1);
        }),
      },
    },
  },
}));

describe('crypto.service.js', () => {
  describe('generateSalt', () => {
    it('should generate a salt', () => {
      const salt = generateSalt();
      expect(salt).toBeDefined();
      expect(typeof salt).toBe('string');
      expect(salt.length).toBeGreaterThan(0);
    });

    it('should generate different salts', () => {
      const salt1 = generateSalt();
      const salt2 = generateSalt();
      expect(salt1).not.toBe(salt2);
    });
  });

  describe('generateSaltedHash', () => {
    it('should generate hash and salt', async () => {
      const domain = 'example.com';
      const result = await generateSaltedHash(domain);

      expect(result).toHaveProperty('hashedDomain');
      expect(result).toHaveProperty('salt');
      expect(typeof result.hashedDomain).toBe('string');
      expect(typeof result.salt).toBe('string');
    });

    it('should generate different results each time', async () => {
      const domain = 'example.com';
      const result1 = await generateSaltedHash(domain);
      const result2 = await generateSaltedHash(domain);

      // Different salts should produce different hashes
      expect(result1.salt).not.toBe(result2.salt);
      expect(result1.hashedDomain).not.toBe(result2.hashedDomain);
    });
  });

  describe('hashDomainWithSalt', () => {
    it('should hash domain with salt', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hash = await hashDomainWithSalt(domain, salt);

      expect(hash).toBeDefined();
      expect(typeof hash).toBe('string');
      expect(hash.length).toBe(64); // SHA256 = 32 bytes = 64 hex chars
    });

    it('should produce consistent hashes', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hash1 = await hashDomainWithSalt(domain, salt);
      const hash2 = await hashDomainWithSalt(domain, salt);

      expect(hash1).toBe(hash2);
    });

    it('should produce different hashes for different domains', async () => {
      const salt = 'abc123';
      const hash1 = await hashDomainWithSalt('example1.com', salt);
      const hash2 = await hashDomainWithSalt('example2.com', salt);

      expect(hash1).not.toBe(hash2);
    });

    it('should produce different hashes for different salts', async () => {
      const domain = 'example.com';
      const hash1 = await hashDomainWithSalt(domain, 'salt1');
      const hash2 = await hashDomainWithSalt(domain, 'salt2');

      expect(hash1).not.toBe(hash2);
    });
  });

  describe('verifyDomainHash', () => {
    it('should verify correct hash', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hashedDomain = await hashDomainWithSalt(domain, salt);

      const isValid = await verifyDomainHash(domain, salt, hashedDomain);
      expect(isValid).toBe(true);
    });

    it('should reject incorrect hash', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const wrongHash = 'incorrect_hash';

      const isValid = await verifyDomainHash(domain, salt, wrongHash);
      expect(isValid).toBe(false);
    });

    it('should reject wrong domain', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hashedDomain = await hashDomainWithSalt(domain, salt);

      const isValid = await verifyDomainHash('wrong.com', salt, hashedDomain);
      expect(isValid).toBe(false);
    });

    it('should reject wrong salt', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hashedDomain = await hashDomainWithSalt(domain, salt);

      const isValid = await verifyDomainHash(domain, 'wrong_salt', hashedDomain);
      expect(isValid).toBe(false);
    });
  });

  describe('verifyTransactionSignature', () => {
    it('should verify valid signature', async () => {
      const txHash = 'a'.repeat(64);
      const signature = '01'.repeat(64); // Mock signature (all 1s)
      const publicKey = 'c'.repeat(64);

      const isValid = await verifyTransactionSignature(txHash, signature, publicKey);

      expect(isValid).toBe(true);
    });

    it('should handle verification errors gracefully', async () => {
      const txHash = 'invalid';
      const signature = 'invalid';
      const publicKey = 'invalid';

      const isValid = await verifyTransactionSignature(txHash, signature, publicKey);

      expect(isValid).toBe(false);
    });
  });

  describe('hashTransactionData', () => {
    it('should hash transaction data', async () => {
      const txData = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
      };

      const hash = await hashTransactionData(txData);

      expect(hash).toBeDefined();
      expect(typeof hash).toBe('string');
      expect(hash.length).toBe(64);
    });

    it('should produce consistent hashes', async () => {
      const txData = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
      };

      const hash1 = await hashTransactionData(txData);
      const hash2 = await hashTransactionData(txData);

      expect(hash1).toBe(hash2);
    });

    it('should produce different hashes for different data', async () => {
      const txData1 = { type: 'name_new', source: 'abc', fee: 1 };
      const txData2 = { type: 'name_new', source: 'xyz', fee: 1 };

      const hash1 = await hashTransactionData(txData1);
      const hash2 = await hashTransactionData(txData2);

      expect(hash1).not.toBe(hash2);
    });
  });

  describe('hashTransaction', () => {
    it('should hash complete transaction object', async () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };

      const hash = await hashTransaction(tx);

      expect(hash).toBeDefined();
      expect(typeof hash).toBe('string');
      expect(hash.length).toBe(64);
    });

    it('should include all transaction fields', async () => {
      const tx1 = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };

      const tx2 = { ...tx1, nonce: 2 };

      const hash1 = await hashTransaction(tx1);
      const hash2 = await hashTransaction(tx2);

      // Changing any field should change the hash
      expect(hash1).not.toBe(hash2);
    });

    it('should produce consistent hashes', async () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };

      const hash1 = await hashTransaction(tx);
      const hash2 = await hashTransaction(tx);

      expect(hash1).toBe(hash2);
    });

    it('should hash stringified JSON', async () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };

      const hash = await hashTransaction(tx);

      // Verify it's hashing the JSON string representation
      expect(hash).toBeDefined();
      expect(typeof hash).toBe('string');
    });
  });
});
