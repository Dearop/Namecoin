import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  savePendingDomain,
  getPendingDomains,
  updateDomainStatus,
  removePendingDomain,
  clearAllPendingDomains
} from '../../utils/domainStorage.js';

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn()
};

global.localStorage = localStorageMock;

describe('domainStorage.js', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.getItem.mockReturnValue(null);
  });

  describe('savePendingDomain', () => {
    it('should save a new pending domain', () => {
      localStorageMock.getItem.mockReturnValue('[]');
      
      savePendingDomain('miner123', 'example.com', 'salt123', 'commitment123');
      
      expect(localStorageMock.getItem).toHaveBeenCalledWith('pending_domains_miner123');
      expect(localStorageMock.setItem).toHaveBeenCalled();
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData).toHaveLength(1);
      expect(savedData[0]).toMatchObject({
        domain: 'example.com',
        salt: 'salt123',
        commitment: 'commitment123',
        status: 'pending'
      });
      expect(savedData[0]).toHaveProperty('timestamp');
    });

    it('should append to existing pending domains', () => {
      const existing = [{
        domain: 'existing.com',
        salt: 'salt456',
        commitment: 'commit456',
        timestamp: Date.now(),
        status: 'pending'
      }];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(existing));
      
      savePendingDomain('miner123', 'new.com', 'salt789', 'commit789');
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData).toHaveLength(2);
      expect(savedData[1].domain).toBe('new.com');
    });
  });

  describe('getPendingDomains', () => {
    it('should return empty array when no domains exist', () => {
      localStorageMock.getItem.mockReturnValue(null);
      
      const result = getPendingDomains('miner123');
      
      expect(result).toEqual([]);
      expect(localStorageMock.getItem).toHaveBeenCalledWith('pending_domains_miner123');
    });

    it('should return stored pending domains', () => {
      const stored = [
        { domain: 'test1.com', salt: 'salt1', commitment: 'commit1', status: 'pending', timestamp: Date.now() },
        { domain: 'test2.com', salt: 'salt2', commitment: 'commit2', status: 'revealed', timestamp: Date.now() }
      ];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(stored));
      
      const result = getPendingDomains('miner123');
      
      expect(result).toEqual(stored);
    });
  });

  describe('updateDomainStatus', () => {
    it('should update status of a specific domain', () => {
      const existing = [
        { domain: 'test1.com', salt: 'salt1', commitment: 'commit1', status: 'pending', timestamp: Date.now() },
        { domain: 'test2.com', salt: 'salt2', commitment: 'commit2', status: 'pending', timestamp: Date.now() }
      ];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(existing));
      
      updateDomainStatus('miner123', 'test1.com', 'revealed');
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData[0].status).toBe('revealed');
      expect(savedData[1].status).toBe('pending');
    });

    it('should not modify other domains', () => {
      const existing = [
        { domain: 'test1.com', salt: 'salt1', commitment: 'commit1', status: 'pending', timestamp: 123 },
        { domain: 'test2.com', salt: 'salt2', commitment: 'commit2', status: 'pending', timestamp: 456 }
      ];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(existing));
      
      updateDomainStatus('miner123', 'test1.com', 'revealed');
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData[1]).toEqual(existing[1]);
    });
  });

  describe('removePendingDomain', () => {
    it('should remove a specific domain', () => {
      const existing = [
        { domain: 'test1.com', salt: 'salt1', commitment: 'commit1', status: 'pending', timestamp: Date.now() },
        { domain: 'test2.com', salt: 'salt2', commitment: 'commit2', status: 'pending', timestamp: Date.now() }
      ];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(existing));
      
      removePendingDomain('miner123', 'test1.com');
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData).toHaveLength(1);
      expect(savedData[0].domain).toBe('test2.com');
    });

    it('should handle removing non-existent domain', () => {
      const existing = [
        { domain: 'test1.com', salt: 'salt1', commitment: 'commit1', status: 'pending', timestamp: Date.now() }
      ];
      localStorageMock.getItem.mockReturnValue(JSON.stringify(existing));
      
      removePendingDomain('miner123', 'nonexistent.com');
      
      const savedData = JSON.parse(localStorageMock.setItem.mock.calls[0][1]);
      expect(savedData).toHaveLength(1);
      expect(savedData[0].domain).toBe('test1.com');
    });
  });

  describe('clearAllPendingDomains', () => {
    it('should remove all pending domains for a miner', () => {
      clearAllPendingDomains('miner123');
      
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('pending_domains_miner123');
    });

    it('should use correct key format', () => {
      clearAllPendingDomains('different-miner-id');
      
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('pending_domains_different-miner-id');
    });
  });
});
