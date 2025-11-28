import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  buildTransaction,
  computeTransactionID,
  encodePayload,
  validateTransaction,
} from '../../services/transaction.service.js';

// Mock the dependencies
vi.mock('../../utils/hash.js', () => ({
  generateTxID: vi.fn(async (params) => {
    // Mock implementation that creates a predictable hash
    return `txid_${params.type}`;
  }),
}));

describe('transaction.service.js', () => {
  describe('buildTransaction', () => {
    it('should build a basic transaction', async () => {
      const params = {
        type: 'name_new',
        walletID: 'abc123',
        fee: 1,
        payload: 'commitment_hash',
      };

      const tx = await buildTransaction(params);

      expect(tx).toEqual({
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment_hash',
        transactionID: null,
      });
    });

    it('should encode payload correctly', async () => {
      const params = {
        type: 'name_new',
        walletID: 'abc123',
        fee: 1,
        payload: { domain: 'example.com', hash: 'abc' },
      };

      const tx = await buildTransaction(params);
      expect(tx.payload).toBe(JSON.stringify({ domain: 'example.com', hash: 'abc' }));
    });

    it('should handle string payload', async () => {
      const params = {
        type: 'name_new',
        walletID: 'abc123',
        fee: 1,
        payload: 'simple_string',
      };

      const tx = await buildTransaction(params);
      expect(tx.payload).toBe('simple_string');
    });
  });

  describe('computeTransactionID', () => {
    it('should add transaction ID to transaction', async () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        transactionID: null,
      };

      const result = await computeTransactionID(tx);

      expect(result.transactionID).toBe('txid_name_new');
      expect(result.type).toBe('name_new');
      expect(result.source).toBe('abc123');
    });

    it('should not mutate original transaction', async () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
        transactionID: null,
      };

      const result = await computeTransactionID(tx);

      expect(tx.transactionID).toBe(null);
      expect(result.transactionID).toBe('txid_name_new');
    });
  });

  describe('encodePayload', () => {
    it('should return string payload as-is', () => {
      const payload = 'commitment_hash_123';
      const encoded = encodePayload(payload);
      expect(encoded).toBe('commitment_hash_123');
    });

    it('should stringify object payload', () => {
      const payload = { domain: 'example.com', hash: 'abc123' };
      const encoded = encodePayload(payload);
      expect(encoded).toBe(JSON.stringify(payload));
    });

    it('should handle nested objects', () => {
      const payload = { 
        data: { 
          nested: { 
            value: 'deep' 
          } 
        } 
      };
      const encoded = encodePayload(payload);
      expect(encoded).toBe(JSON.stringify(payload));
    });

    it('should handle arrays', () => {
      const payload = [1, 2, 3];
      const encoded = encodePayload(payload);
      expect(encoded).toBe(JSON.stringify(payload));
    });

    it('should handle null', () => {
      const payload = null;
      const encoded = encodePayload(payload);
      expect(encoded).toBe('null');
    });

    it('should handle numbers', () => {
      const payload = 42;
      const encoded = encodePayload(payload);
      expect(encoded).toBe('42');
    });
  });

  describe('validateTransaction', () => {
    it('should validate a correct transaction', () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(true);
      expect(result.errors).toHaveLength(0);
    });

    it('should detect missing type', () => {
      const tx = {
        source: 'abc123',
        fee: 1,
        payload: 'commitment',
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Transaction type is required');
    });

    it('should detect missing source', () => {
      const tx = {
        type: 'name_new',
        fee: 1,
        payload: 'commitment',
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Source wallet is required');
    });

    it('should detect invalid fee', () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 0,
        payload: 'commitment',
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Fee must be at least 1');
    });

    it('should detect missing payload', () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 1,
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Payload is required');
    });

    it('should collect multiple errors', () => {
      const tx = {};

      const result = validateTransaction(tx);
      expect(result.valid).toBe(false);
      expect(result.errors.length).toBeGreaterThan(1);
    });

    it('should accept fee greater than 1', () => {
      const tx = {
        type: 'name_new',
        source: 'abc123',
        fee: 10,
        payload: 'commitment',
      };

      const result = validateTransaction(tx);
      expect(result.valid).toBe(true);
    });

  });
});
