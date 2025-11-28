import { describe, it, expect, beforeEach } from 'vitest';
import {
  sha256,
  bytesToHex,
  hexToBytes,
  generateRandomSalt,
  generateHash,
  generateSalt,
  hashTxData,
  generateTxID,
} from '../../utils/hash.js';

describe('hash.js', () => {
  describe('bytesToHex', () => {
    it('should convert bytes to hex string', () => {
      const bytes = new Uint8Array([0, 15, 255, 128]);
      const hex = bytesToHex(bytes);
      expect(hex).toBe('000fff80');
    });

    it('should handle empty byte array', () => {
      const bytes = new Uint8Array([]);
      const hex = bytesToHex(bytes);
      expect(hex).toBe('');
    });

    it('should pad single digit hex values', () => {
      const bytes = new Uint8Array([1, 2, 3]);
      const hex = bytesToHex(bytes);
      expect(hex).toBe('010203');
    });
  });

  describe('hexToBytes', () => {
    it('should convert hex string to bytes', () => {
      const hex = '000fff80';
      const bytes = hexToBytes(hex);
      expect(bytes).toEqual(new Uint8Array([0, 15, 255, 128]));
    });

    it('should handle empty hex string', () => {
      const hex = '';
      const bytes = hexToBytes(hex);
      expect(bytes).toEqual(new Uint8Array([]));
    });

    it('should be inverse of bytesToHex', () => {
      const original = new Uint8Array([10, 20, 30, 40, 50]);
      const hex = bytesToHex(original);
      const result = hexToBytes(hex);
      expect(result).toEqual(original);
    });
  });

  describe('sha256', () => {
    it('should hash a string', async () => {
      const input = 'test';
      const hash = await sha256(input);
      expect(hash).toBe('9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08');
    });

    it('should hash bytes', async () => {
      const input = new Uint8Array([116, 101, 115, 116]); // 'test'
      const hash = await sha256(input);
      expect(hash).toBe('9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08');
    });

    it('should produce different hashes for different inputs', async () => {
      const hash1 = await sha256('test1');
      const hash2 = await sha256('test2');
      expect(hash1).not.toBe(hash2);
    });

    it('should produce consistent hashes', async () => {
      const hash1 = await sha256('consistent');
      const hash2 = await sha256('consistent');
      expect(hash1).toBe(hash2);
    });

    it('should hash empty string', async () => {
      const hash = await sha256('');
      expect(hash).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855');
    });
  });

  describe('generateRandomSalt', () => {
    it('should generate salt of specified length', () => {
      const salt = generateRandomSalt(32);
      expect(salt).toHaveLength(64); // 32 bytes = 64 hex chars
    });

    it('should generate different salts each time', () => {
      const salt1 = generateRandomSalt(32);
      const salt2 = generateRandomSalt(32);
      expect(salt1).not.toBe(salt2);
    });

    it('should generate hex string', () => {
      const salt = generateRandomSalt(16);
      expect(/^[0-9a-f]+$/.test(salt)).toBe(true);
    });
  });

  describe('generateSalt', () => {
    it('should generate 16-byte salt', () => {
      const salt = generateSalt();
      expect(salt).toHaveLength(32); // 16 bytes = 32 hex chars
    });

    it('should generate different salts', () => {
      const salt1 = generateSalt();
      const salt2 = generateSalt();
      expect(salt1).not.toBe(salt2);
    });
  });

  describe('generateHash', () => {
    it('should hash domain with salt', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hash = await generateHash(domain, salt);
      expect(hash).toHaveLength(64); // SHA256 produces 32 bytes = 64 hex chars
    });

    it('should produce different hashes for different domains', async () => {
      const salt = 'abc123';
      const hash1 = await generateHash('example1.com', salt);
      const hash2 = await generateHash('example2.com', salt);
      expect(hash1).not.toBe(hash2);
    });

    it('should produce different hashes for different salts', async () => {
      const domain = 'example.com';
      const hash1 = await generateHash(domain, 'salt1');
      const hash2 = await generateHash(domain, 'salt2');
      expect(hash1).not.toBe(hash2);
    });

    it('should produce consistent hashes', async () => {
      const domain = 'example.com';
      const salt = 'abc123';
      const hash1 = await generateHash(domain, salt);
      const hash2 = await generateHash(domain, salt);
      expect(hash1).toBe(hash2);
    });
  });

  describe('hashTxData', () => {
    it('should hash transaction data object', async () => {
      const txData = { type: 'name_new', source: 'abc', fee: 1 };
      const hash = await hashTxData(txData);
      expect(hash).toHaveLength(64);
    });

    it('should produce consistent hashes for same data', async () => {
      const txData = { type: 'name_new', source: 'abc', fee: 1 };
      const hash1 = await hashTxData(txData);
      const hash2 = await hashTxData(txData);
      expect(hash1).toBe(hash2);
    });

    it('should produce different hashes for different data', async () => {
      const txData1 = { type: 'name_new', source: 'abc', fee: 1 };
      const txData2 = { type: 'name_new', source: 'xyz', fee: 1 };
      const hash1 = await hashTxData(txData1);
      const hash2 = await hashTxData(txData2);
      expect(hash1).not.toBe(hash2);
    });
  });

  describe('generateTxID', () => {
    it('should generate transaction ID from parameters', async () => {
      const params = {
        type: 'name_new',
        sourceID: 'abc123',
        fee: 1,
        payload: 'hash123',
      };
      const txID = await generateTxID(params);
      expect(txID).toHaveLength(64);
    });

    it('should produce consistent IDs for same parameters', async () => {
      const params = {
        type: 'name_new',
        sourceID: 'abc123',
        fee: 1,
        payload: 'hash123',
      };
      const txID1 = await generateTxID(params);
      const txID2 = await generateTxID(params);
      expect(txID1).toBe(txID2);
    });

    it('should include all parameters in hash', async () => {
      const params = {
        type: 'name_new',
        sourceID: 'abc123',
        fee: 1,
        payload: 'hash123',
      };
      const baseID = await generateTxID(params);

      // Changing any parameter should change the ID
      const params2 = { ...params, type: 'name_update' };
      const id2 = await generateTxID(params2);
      expect(id2).not.toBe(baseID);

      const params3 = { ...params, fee: 2 };
      const id3 = await generateTxID(params3);
      expect(id3).not.toBe(baseID);
    });
  });
});
