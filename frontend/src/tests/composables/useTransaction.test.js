import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useTransaction } from '../../composables/useTransaction.js';

// Mock dependencies
vi.mock('../../services/transaction.service.js', () => ({
  buildTransaction: vi.fn(async (params) => ({
    type: params.type,
    source: params.walletID,
    fee: params.fee,
    payload: params.payload,
    nonce: params.nonce,
    transactionID: null,
  })),
  computeTransactionID: vi.fn(async (tx) => ({
    ...tx,
    transactionID: 'computed_tx_id',
  })),
  validateTransaction: vi.fn((tx) => ({
    valid: true,
    errors: [],
  })),
}));

vi.mock('../../services/crypto.service.js', () => ({
  hashTransaction: vi.fn(async (tx) => 'mocked_tx_hash'),
  generateTransactionSignature: vi.fn(async (txHash, privateKey) => 'mocked_signature'),
  verifyTransactionSignature: vi.fn(async (txHash, signature, publicKey) => true),
}));

vi.mock('../../services/api.service.js', () => ({
  sendTransaction: vi.fn(async (tx, signature) => ({
    success: true,
    txID: tx.transactionID,
  })),
}));

vi.mock('../../utils/storage.js', () => ({
  incrementNonce: vi.fn(() => 1),
  saveDomain: vi.fn(),
}));

describe('useTransaction.js', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('initial state', () => {
    it('should have correct initial state', () => {
      const { currentTransaction, isProcessing, status, error, txHistory } = useTransaction();

      expect(currentTransaction.value).toBe(null);
      expect(isProcessing.value).toBe(false);
      expect(status.value).toBe('idle');
      expect(error.value).toBe(null);
      expect(txHistory.value).toEqual([]);
    });
  });

  describe('createTransaction', () => {
    it('should create a transaction successfully', async () => {
      const { createTransaction, currentTransaction } = useTransaction();

      const params = {
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment_hash',
      };

      const result = await createTransaction(params);

      expect(result).toEqual({
        type: 'name_new',
        source: 'wallet123',
        fee: 1,
        payload: 'commitment_hash',
        nonce: 1,
        transactionID: 'computed_tx_id',
      });

      expect(currentTransaction.value).toEqual(result);
    });

    it('should increment nonce', async () => {
      const storage = await import('../../utils/storage.js');
      const { createTransaction } = useTransaction();

      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      expect(storage.incrementNonce).toHaveBeenCalled();
    });

    it('should validate transaction', async () => {
      const txService = await import('../../services/transaction.service.js');
      const { createTransaction } = useTransaction();

      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      expect(txService.validateTransaction).toHaveBeenCalled();
    });

    it('should handle validation errors', async () => {
      const txService = await import('../../services/transaction.service.js');
      txService.validateTransaction.mockReturnValueOnce({
        valid: false,
        errors: ['Invalid fee', 'Missing payload'],
      });

      const { createTransaction, error } = useTransaction();

      await expect(
        createTransaction({
          type: 'name_new',
          walletID: 'wallet123',
          fee: 0,
          payload: '',
        })
      ).rejects.toThrow();

      expect(error.value).toContain('Invalid fee');
    });

    it('should set processing state', async () => {
      const { createTransaction, isProcessing } = useTransaction();

      const promise = createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      // Should be processing during call
      expect(isProcessing.value).toBe(true);

      await promise;

      // Should be done after call
      expect(isProcessing.value).toBe(false);
    });
  });

  describe('signAndSend', () => {
    it('should sign and send transaction', async () => {
      const cryptoService = await import('../../services/crypto.service.js');
      const apiService = await import('../../services/api.service.js');
      const { createTransaction, signAndSend, status } = useTransaction();

      // First create a transaction
      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      // Then sign and send
      const result = await signAndSend('private_key_123');

      expect(cryptoService.hashTransaction).toHaveBeenCalled();
      expect(cryptoService.generateTransactionSignature).toHaveBeenCalledWith(
        'mocked_tx_hash',
        'private_key_123'
      );
      expect(cryptoService.verifyTransactionSignature).toHaveBeenCalled();
      expect(apiService.sendTransaction).toHaveBeenCalled();
      expect(status.value).toBe('success');
      expect(result).toEqual({
        success: true,
        txID: 'computed_tx_id',
      });
    });

    it('should handle signature verification failure', async () => {
      const cryptoService = await import('../../services/crypto.service.js');
      cryptoService.verifyTransactionSignature.mockResolvedValueOnce(false);

      const { createTransaction, signAndSend, status, error } = useTransaction();

      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      await expect(signAndSend('private_key_123')).rejects.toThrow(
        'Signature verification failed after signing'
      );

      expect(status.value).toBe('error');
      expect(error.value).toBeTruthy();
    });

    it('should handle API errors', async () => {
      const apiService = await import('../../services/api.service.js');
      apiService.sendTransaction.mockRejectedValueOnce(new Error('Network error'));

      const { createTransaction, signAndSend, status, error } = useTransaction();

      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      await expect(signAndSend('private_key_123')).rejects.toThrow('Network error');

      expect(status.value).toBe('error');
      expect(error.value).toBe('Network error');
    });
  });

  describe('clearTransaction', () => {
    it('should clear current transaction', async () => {
      const { createTransaction, clearTransaction, currentTransaction } = useTransaction();

      await createTransaction({
        type: 'name_new',
        walletID: 'wallet123',
        fee: 1,
        payload: 'commitment',
      });

      expect(currentTransaction.value).not.toBe(null);

      clearTransaction();

      expect(currentTransaction.value).toBe(null);
    });
  });
});
