import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useWallet } from '../../composables/useWallet.js';

// Mock dependencies
vi.mock('../../services/wallet.service.js', () => ({
  generateKeyPair: vi.fn(async () => ({
    publicKey: 'mock_public_key_123',
    privateKey: 'mock_private_key_456',
  })),
  deriveWalletID: vi.fn(async (publicKey) => `hash_${publicKey}`),
  saveWallet: vi.fn(() => true),
  loadWallet: vi.fn(() => null),
  exportWalletToFile: vi.fn(),
  importWalletFromFile: vi.fn(async (file) => ({
    publicKey: 'imported_public',
    privateKey: 'imported_private',
  })),
}));

vi.mock('../../utils/storage.js', () => ({
  clear: vi.fn(),
}));

describe('useWallet.js', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('wallet state', () => {
    it('should have initial null wallet', () => {
      const { wallet } = useWallet();
      expect(wallet.value.publicKey).toBe(null);
      expect(wallet.value.privateKey).toBe(null);
    });

    it('should compute walletID from publicKey', () => {
      const { wallet, walletID } = useWallet();
      
      wallet.value = {
        publicKey: 'test_key',
        privateKey: 'test_private',
        id: 'hash_test_key'
      };

      expect(walletID.value).toBe('hash_test_key');
    });

    it('should return null walletID when no wallet', () => {
      const { wallet, walletID } = useWallet();
      
      wallet.value = {
        publicKey: null,
        privateKey: null,
        id: null
      };

      expect(walletID.value).toBe(null);
    });

    it('should compute isWalletLoaded correctly', () => {
      const { wallet, isWalletLoaded } = useWallet();

      expect(isWalletLoaded.value).toBe(false);

      wallet.value = {
        publicKey: 'test_key',
        privateKey: 'test_private',
        id: 'hash_test_key'
      };

      expect(isWalletLoaded.value).toBe(true);
    });
  });

  describe('createWallet', () => {
    it('should create a new wallet', async () => {
      const walletService = await import('../../services/wallet.service.js');
      const { createWallet, wallet, walletID } = useWallet();

      const result = await createWallet();

      expect(walletService.generateKeyPair).toHaveBeenCalled();
      expect(walletService.deriveWalletID).toHaveBeenCalledWith('mock_public_key_123');
      expect(walletService.saveWallet).toHaveBeenCalledWith(
        'mock_public_key_123',
        'mock_private_key_456'
      );
      
      expect(wallet.value.publicKey).toBe('mock_public_key_123');
      expect(wallet.value.privateKey).toBe('mock_private_key_456');
      expect(wallet.value.id).toBe('hash_mock_public_key_123');
      expect(walletID.value).toBe('hash_mock_public_key_123');
      expect(result).toEqual(wallet.value);
    });

    it('should handle errors during creation', async () => {
      const walletService = await import('../../services/wallet.service.js');
      walletService.generateKeyPair.mockRejectedValueOnce(new Error('Creation failed'));

      const { createWallet } = useWallet();

      await expect(createWallet()).rejects.toThrow('Creation failed');
    });
  });

  describe('importWallet', () => {
    it('should import wallet from file', async () => {
      const walletService = await import('../../services/wallet.service.js');
    const { importWallet, wallet } = useWallet();

    const mockFile = new Blob(['{}'], { type: 'application/json' });
    await importWallet(mockFile);

    expect(walletService.importWalletFromFile).toHaveBeenCalledWith(mockFile);
    expect(wallet.value.publicKey).toBe('imported_public');
    expect(wallet.value.privateKey).toBe('imported_private');
    expect(wallet.value.id).toBe('hash_imported_public');
    expect(walletService.saveWallet).toHaveBeenCalledWith(
      'imported_public',
      'imported_private'
    );
    });

    it('should handle import errors', async () => {
      const walletService = await import('../../services/wallet.service.js');
      walletService.importWalletFromFile.mockRejectedValueOnce(new Error('Import failed'));

      const { importWallet } = useWallet();

      const mockFile = new Blob(['{}'], { type: 'application/json' });

      await expect(importWallet(mockFile)).rejects.toThrow('Import failed');
    });
  });
});
