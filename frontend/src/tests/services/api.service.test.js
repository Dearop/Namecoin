import { describe, it, expect, beforeEach, vi } from 'vitest';
import canonicalize from 'canonicalize';
import {
  sendTransaction,
  getTransactionStatus,
  getMinerID,
} from '../../services/api.service.js';

// Mock fetch globally
global.fetch = vi.fn();

// Mock localStorage (still needed for minerID)
const localStorageMock = {
  getItem: vi.fn((key) => {
    if (key === 'minerID') return 'test-miner-id';
    return null;
  }),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn()
};
global.localStorage = localStorageMock;

// Mock import.meta.env (vite sets VITE_BACKEND_URL to http://localhost:8080 by default)
vi.stubGlobal('import', {
  meta: {
    env: {
      VITE_BACKEND_URL: 'http://localhost:8080'
    }
  }
});

describe('api.service.js', () => {
  beforeEach(() => {
    fetch.mockClear();
    localStorageMock.getItem.mockClear();
  });

  describe('sendTransaction', () => {
    it('should send transaction successfully', async () => {
      const mockResponse = { success: true, txID: 'abc123' };
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const tx = {
        type: 'name_new',
        from: 'wallet123',
        amount: 1,
        payload: 'commitment',
        pk: 'pubkey123',
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      const result = await sendTransaction(tx, signature);

      expect(result).toEqual(mockResponse);
      expect(fetch).toHaveBeenCalledWith(
        'http://localhost:8080/namecoin/handle',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: canonicalize({
            type: tx.type,
            from: tx.from,
            amount: tx.amount,
            payload: tx.payload,
            pk: tx.pk,
            txId: tx.transactionID,
            signature: signature
          }),
        })
      );
    });

    it('should throw error on non-ok response', async () => {
      fetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({}),
      });

      const tx = {
        type: 'name_new',
        from: 'wallet123',
        amount: 1,
        payload: 'commitment',
        pk: 'pubkey123',
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      await expect(sendTransaction(tx, signature)).rejects.toThrow('HTTP error! status: 500');
    });

    it('should handle network errors', async () => {
      fetch.mockRejectedValueOnce(new Error('Network error'));

      const tx = {
        type: 'name_new',
        from: 'wallet123',
        amount: 1,
        payload: 'commitment',
        pk: 'pubkey123',
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      await expect(sendTransaction(tx, signature)).rejects.toThrow('Network error');
    });

    it('should send correct JSON body', async () => {
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ success: true }),
      });

      const tx = {
        type: 'name_new',
        from: 'wallet123',
        amount: 1,
        payload: 'commitment',
        pk: 'pubkey123',
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      await sendTransaction(tx, signature);

      const callArgs = fetch.mock.calls[0];
      const bodyString = callArgs[1].body;
      const bodyObj = JSON.parse(bodyString);

      expect(bodyObj).toEqual({
        type: tx.type,
        from: tx.from,
        amount: tx.amount,
        payload: tx.payload,
        pk: tx.pk,
        txId: tx.transactionID,
        signature: signature
      });
    });
  });

  describe('getTransactionStatus', () => {
    it('should get transaction status successfully', async () => {
      const mockResponse = { status: 'confirmed', txID: 'abc123' };
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await getTransactionStatus('abc123');

      expect(result).toEqual(mockResponse);
      expect(fetch).toHaveBeenCalledWith('http://localhost:8080/namecoin/transaction/abc123');
    });

    it('should throw error on non-ok response', async () => {
      fetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
      });

      await expect(getTransactionStatus('abc123')).rejects.toThrow('HTTP error! status: 404');
    });

    it('should handle network errors', async () => {
      fetch.mockRejectedValueOnce(new Error('Network error'));

      await expect(getTransactionStatus('abc123')).rejects.toThrow('Network error');
    });

    it('should use correct endpoint', async () => {
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ status: 'confirmed' }),
      });

      await getTransactionStatus('txid789');

      expect(fetch).toHaveBeenCalledWith('http://localhost:8080/namecoin/transaction/txid789');
    });
  });

  describe('getMinerID', () => {
    it('should get miner ID successfully', async () => {
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ minerID: 'miner-abc123' }),
      });

      const result = await getMinerID();

      expect(result).toBe('miner-abc123');
      expect(fetch).toHaveBeenCalledWith('http://localhost:8080/namecoin/minerid');
    });

    it('should throw error on non-ok response', async () => {
      fetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      await expect(getMinerID()).rejects.toThrow('HTTP error! status: 500');
    });

    it('should handle network errors', async () => {
      fetch.mockRejectedValueOnce(new Error('Network error'));

      await expect(getMinerID()).rejects.toThrow('Network error');
    });
  });
});
