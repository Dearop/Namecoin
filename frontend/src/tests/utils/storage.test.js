import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  setItem,
  getItem,
  removeItem,
  clear,
  saveWalletData,
  getWalletData,
  saveNonce,
  getNonce,
  incrementNonce,
  saveDomain,
  getDomains,
  StorageKeys,
} from '../../utils/storage.js';

// Mock localStorage
const localStorageMock = (() => {
  let store = {};

  return {
    getItem: (key) => store[key] || null,
    setItem: (key, value) => {
      store[key] = value.toString();
    },
    removeItem: (key) => {
      delete store[key];
    },
    clear: () => {
      store = {};
    },
  };
})();

global.localStorage = localStorageMock;

describe('storage.js', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('setItem and getItem', () => {
    it('should store and retrieve a value', () => {
      setItem('test', { data: 'value' });
      const result = getItem('test');
      expect(result).toEqual({ data: 'value' });
    });

    it('should return default value if key does not exist', () => {
      const result = getItem('nonexistent', 'default');
      expect(result).toBe('default');
    });

    it('should return null if no default value provided', () => {
      const result = getItem('nonexistent');
      expect(result).toBe(null);
    });

    it('should handle string values', () => {
      setItem('test', 'string value');
      const result = getItem('test');
      expect(result).toBe('string value');
    });

    it('should handle number values', () => {
      setItem('test', 42);
      const result = getItem('test');
      expect(result).toBe(42);
    });

    it('should handle array values', () => {
      setItem('test', [1, 2, 3]);
      const result = getItem('test');
      expect(result).toEqual([1, 2, 3]);
    });

    it('should handle nested objects', () => {
      const nested = { a: { b: { c: 'deep' } } };
      setItem('test', nested);
      const result = getItem('test');
      expect(result).toEqual(nested);
    });
  });

  describe('removeItem', () => {
    it('should remove an item', () => {
      setItem('test', 'value');
      removeItem('test');
      const result = getItem('test');
      expect(result).toBe(null);
    });

    it('should not throw error if item does not exist', () => {
      expect(() => removeItem('nonexistent')).not.toThrow();
    });
  });

  describe('clear', () => {
    it('should remove all peerster items', () => {
      setItem(StorageKeys.WALLET, { key: 'value' });
      setItem(StorageKeys.NONCE, 5);
      setItem(StorageKeys.DOMAINS, []);
      setItem('other_key', 'should remain'); // Non-peerster key

      clear();

      expect(getItem(StorageKeys.WALLET)).toBe(null);
      expect(getItem(StorageKeys.NONCE)).toBe(null);
      expect(getItem(StorageKeys.DOMAINS)).toBe(null);
      expect(getItem('other_key')).toBe('should remain');
    });
  });

  describe('Wallet helpers', () => {
    it('should save and retrieve wallet data', () => {
      const wallet = {
        publicKey: 'abc123',
        privateKey: 'def456',
        createdAt: '2025-01-01',
      };
      saveWalletData(wallet);
      const retrieved = getWalletData();
      expect(retrieved).toEqual(wallet);
    });

    it('should return null if no wallet saved', () => {
      const retrieved = getWalletData();
      expect(retrieved).toBe(null);
    });
  });

  describe('Nonce helpers', () => {
    it('should save and retrieve nonce', () => {
      saveNonce(10);
      const retrieved = getNonce();
      expect(retrieved).toBe(10);
    });

    it('should return 0 if no nonce saved', () => {
      const retrieved = getNonce();
      expect(retrieved).toBe(0);
    });

    it('should increment nonce', () => {
      saveNonce(5);
      const next = incrementNonce();
      expect(next).toBe(6);
      expect(getNonce()).toBe(6);
    });

    it('should increment from 0 if no nonce saved', () => {
      const next = incrementNonce();
      expect(next).toBe(1);
      expect(getNonce()).toBe(1);
    });

    it('should increment multiple times', () => {
      incrementNonce(); // 1
      incrementNonce(); // 2
      const next = incrementNonce(); // 3
      expect(next).toBe(3);
      expect(getNonce()).toBe(3);
    });
  });

  describe('Domain helpers', () => {
    it('should save and retrieve domains', () => {
      const domain1 = { name: 'example.com', hash: 'abc' };
      const domain2 = { name: 'test.com', hash: 'def' };

      saveDomain(domain1);
      saveDomain(domain2);

      const domains = getDomains();
      expect(domains).toHaveLength(2);
      expect(domains[0]).toEqual(domain1);
      expect(domains[1]).toEqual(domain2);
    });

    it('should return empty array if no domains saved', () => {
      const domains = getDomains();
      expect(domains).toEqual([]);
    });

    it('should append to existing domains', () => {
      saveDomain({ name: 'first.com', hash: 'abc' });
      saveDomain({ name: 'second.com', hash: 'def' });
      
      const domains = getDomains();
      expect(domains).toHaveLength(2);
    });
  });

  describe('StorageKeys', () => {
    it('should have correct prefixed keys', () => {
      expect(StorageKeys.WALLET).toBe('peerster_wallet');
      expect(StorageKeys.NONCE).toBe('peerster_nonce');
      expect(StorageKeys.DOMAINS).toBe('peerster_domains');
      expect(StorageKeys.TRANSACTIONS).toBe('peerster_transactions');
    });
  });

  describe('Error handling', () => {
    it('should handle JSON parse errors gracefully', () => {
      localStorage.setItem('test', 'invalid json{');
      const result = getItem('test', 'default');
      expect(result).toBe('default');
    });
  });
});
