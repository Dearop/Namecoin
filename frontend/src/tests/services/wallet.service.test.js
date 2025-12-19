import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  generateKeyPair,
  deriveWalletID,
  saveWallet,
  loadWallet,
  exportWalletToFile,
  importWalletFromFile,
} from '../../services/wallet.service.js';
import { sha256, hexToBytes } from '../../utils/hash.js';

// Mock dependencies
vi.mock('../../utils/storage.js', () => ({
  saveWalletData: vi.fn((wallet) => true),
  getWalletData: vi.fn(() => null),
}));

vi.mock('tweetnacl', () => ({
  default: {
    sign: {
      keyPair: vi.fn(() => ({
        secretKey: new Uint8Array(64).fill(1),
        publicKey: new Uint8Array(32).fill(2),
      })),
    },
  },
}));

describe('wallet.service.js', () => {
  describe('generateKeyPair', () => {
    it('should generate a keypair', async () => {
      const keypair = await generateKeyPair();

      expect(keypair).toHaveProperty('privateKey');
      expect(keypair).toHaveProperty('publicKey');
      expect(typeof keypair.privateKey).toBe('string');
      expect(typeof keypair.publicKey).toBe('string');
    });

    it('should generate hex strings', async () => {
      const keypair = await generateKeyPair();

      expect(/^[0-9a-f]+$/.test(keypair.privateKey)).toBe(true);
      expect(/^[0-9a-f]+$/.test(keypair.publicKey)).toBe(true);
    });

    it('should generate correct lengths', async () => {
      const keypair = await generateKeyPair();

      // Private key is 64 bytes = 128 hex chars
      expect(keypair.privateKey).toHaveLength(128);
      // Public key is 32 bytes = 64 hex chars
      expect(keypair.publicKey).toHaveLength(64);
    });
  });

  describe('deriveWalletID', () => {
    it('should hash the public key bytes', async () => {
      const publicKey = 'abc123def456';
      const expected = await sha256(hexToBytes(publicKey));
      const walletID = await deriveWalletID(publicKey);

      expect(walletID).toBe(expected);
    });

    it('should produce deterministic hashes', async () => {
      const publicKey = '0123456789abcdef';
      const first = await deriveWalletID(publicKey);
      const second = await deriveWalletID(publicKey);

      expect(first).toBe(second);
    });
  });

  describe('saveWallet', () => {
    it('should save wallet with correct structure', () => {
      const publicKey = 'abc123';
      const privateKey = 'def456';

      const result = saveWallet(publicKey, privateKey);

      expect(result).toBe(true);
    });

    it('should include createdAt timestamp', async () => {
      const { saveWalletData } = await import('../../utils/storage.js');
      
      saveWallet('abc', 'def');

      expect(saveWalletData).toHaveBeenCalledWith(
        expect.objectContaining({
          publicKey: 'abc',
          privateKey: 'def',
          createdAt: expect.any(String),
        })
      );
    });
  });

  describe('exportWalletToFile', () => {
    it('should create a download link', () => {
      // Mock DOM APIs
      const mockLink = {
        href: '',
        download: '',
        click: vi.fn(),
      };
      const createElementSpy = vi.spyOn(document, 'createElement').mockReturnValue(mockLink);
      const createObjectURLSpy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock-url');
      const revokeObjectURLSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});

      const wallet = { publicKey: 'abc', privateKey: 'def' };
      exportWalletToFile(wallet);

      expect(createElementSpy).toHaveBeenCalledWith('a');
      expect(createObjectURLSpy).toHaveBeenCalled();
      expect(mockLink.click).toHaveBeenCalled();
      expect(revokeObjectURLSpy).toHaveBeenCalledWith('blob:mock-url');

      createElementSpy.mockRestore();
      createObjectURLSpy.mockRestore();
      revokeObjectURLSpy.mockRestore();
    });

    it('should set correct filename pattern', () => {
      const mockLink = {
        href: '',
        download: '',
        click: vi.fn(),
      };
      vi.spyOn(document, 'createElement').mockReturnValue(mockLink);
      vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:mock-url');
      vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});

      const wallet = { publicKey: 'abc', privateKey: 'def' };
      exportWalletToFile(wallet);

      expect(mockLink.download).toMatch(/^peerster-wallet-\d+\.json$/);
    });
  });

  describe('importWalletFromFile', () => {
    it('should import valid wallet file', async () => {
      const walletData = { publicKey: 'abc123', privateKey: 'def456' };
      const file = new Blob([JSON.stringify(walletData)], { type: 'application/json' });

      const wallet = await importWalletFromFile(file);

      expect(wallet).toEqual(walletData);
    });

    it('should reject invalid wallet file', async () => {
      const invalidData = { something: 'else' };
      const file = new Blob([JSON.stringify(invalidData)], { type: 'application/json' });

      await expect(importWalletFromFile(file)).rejects.toThrow('Invalid wallet file');
    });

    it('should reject malformed JSON', async () => {
      const file = new Blob(['{ invalid json }'], { type: 'application/json' });

      await expect(importWalletFromFile(file)).rejects.toThrow();
    });

    it('should require both publicKey and privateKey', async () => {
      const walletData = { publicKey: 'abc123' }; // missing privateKey
      const file = new Blob([JSON.stringify(walletData)], { type: 'application/json' });

      await expect(importWalletFromFile(file)).rejects.toThrow('Invalid wallet file');
    });
  });
});
