import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  sendTransaction,
  getTransactionStatus,
  getBlockchainState,
} from '../../services/api.service.js';

// Mock fetch globally
global.fetch = vi.fn();

describe('api.service.js', () => {
  beforeEach(() => {
    fetch.mockClear();
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
        source: 'wallet123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      const result = await sendTransaction(tx, signature);

      expect(result).toEqual(mockResponse);
      expect(fetch).toHaveBeenCalledWith(
        '/namecoin/transaction',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            transaction: tx,
            signature: signature,
          }),
        })
      );
    });

    it('should throw error on non-ok response', async () => {
      fetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      const tx = {
        type: 'name_new',
        source: 'wallet123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      await expect(sendTransaction(tx, signature)).rejects.toThrow('HTTP error! status: 500');
    });

    it('should handle network errors', async () => {
      fetch.mockRejectedValueOnce(new Error('Network error'));

      const tx = {
        type: 'name_new',
        source: 'wallet123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
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
        source: 'wallet123',
        fee: 1,
        payload: 'commitment',
        nonce: 1,
        transactionID: 'txid123',
      };
      const signature = 'sig123';

      await sendTransaction(tx, signature);

      const callArgs = fetch.mock.calls[0];
      const bodyString = callArgs[1].body;
      const bodyObj = JSON.parse(bodyString);

      expect(bodyObj).toEqual({
        transaction: tx,
        signature: signature,
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
      expect(fetch).toHaveBeenCalledWith('/namecoin/transaction/abc123');
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

      expect(fetch).toHaveBeenCalledWith('/namecoin/transaction/txid789');
    });
  });

  describe('getBlockchainState', () => {
    it('should get blockchain state successfully', async () => {
      const mockResponse = { blocks: [], height: 0 };
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await getBlockchainState();

      expect(result).toEqual(mockResponse);
      expect(fetch).toHaveBeenCalledWith('/blockchain');
    });

    it('should throw error on non-ok response', async () => {
      fetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      await expect(getBlockchainState()).rejects.toThrow('HTTP error! status: 500');
    });

    it('should handle network errors', async () => {
      fetch.mockRejectedValueOnce(new Error('Network error'));

      await expect(getBlockchainState()).rejects.toThrow('Network error');
    });

    it('should use correct endpoint', async () => {
      fetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ blocks: [] }),
      });

      await getBlockchainState();

      expect(fetch).toHaveBeenCalledWith('/blockchain');
    });
  });
});
