import { describe, it, expect } from 'vitest';
import {
  isValidDomain,
  isValidFee,
  isValidHex,
  isValidWalletID,
} from '../../utils/validation.js';

describe('validation.js', () => {
  describe('isValidDomain', () => {
    it('should reject invalid domains', () => {
      expect(isValidDomain('example')).toBe(false);
      expect(isValidDomain('.example.com')).toBe(false);
      expect(isValidDomain('example..com')).toBe(false);
      expect(isValidDomain('example.c')).toBe(false);
      expect(isValidDomain('-example.com')).toBe(false);
      expect(isValidDomain('')).toBe(false);
    });

    it('should reject domains with special characters', () => {
      expect(isValidDomain('exam ple.com')).toBe(false);
      expect(isValidDomain('exam@ple.com')).toBe(false);
      expect(isValidDomain('exam_ple.com')).toBe(false);
    });
  });

  describe('isValidFee', () => {
    it('should accept valid fees', () => {
      expect(isValidFee(1)).toBe(true);
      expect(isValidFee(10)).toBe(true);
      expect(isValidFee(1000)).toBe(true);
    });

    it('should reject invalid fees', () => {
      expect(isValidFee(0)).toBe(false);
      expect(isValidFee(-1)).toBe(false);
      expect(isValidFee(1.5)).toBe(false);
      expect(isValidFee('1')).toBe(false);
      expect(isValidFee(null)).toBe(false);
      expect(isValidFee(undefined)).toBe(false);
    });

    it('should require positive integers', () => {
      expect(isValidFee(0.5)).toBe(false);
      expect(isValidFee(-10)).toBe(false);
      expect(isValidFee(NaN)).toBe(false);
      expect(isValidFee(Infinity)).toBe(false);
    });
  });

  describe('isValidHex', () => {
    it('should accept valid hex strings', () => {
      expect(isValidHex('abc123')).toBe(true);
      expect(isValidHex('ABC123')).toBe(true);
      expect(isValidHex('0123456789abcdefABCDEF')).toBe(true);
      expect(isValidHex('00')).toBe(true);
    });

    it('should reject invalid hex strings', () => {
      expect(isValidHex('xyz')).toBe(false);
      expect(isValidHex('abc 123')).toBe(false);
      expect(isValidHex('abc-123')).toBe(false);
      expect(isValidHex('g123')).toBe(false);
      expect(isValidHex('')).toBe(false);
    });

    it('should handle edge cases', () => {
      expect(isValidHex('0x123')).toBe(false); // 0x prefix not allowed
      expect(isValidHex('!')).toBe(false);
      expect(isValidHex('abc123def456')).toBe(true);
    });
  });

  describe('isValidWalletID', () => {
    it('should accept valid wallet IDs', () => {
      const validID = 'abc123def456abc123def456abc123de'; // 32 chars
      expect(isValidWalletID(validID)).toBe(true);

      const longerID = 'abc123def456abc123def456abc123def456'; // 36 chars
      expect(isValidWalletID(longerID)).toBe(true);
    });

    it('should reject invalid wallet IDs', () => {
      expect(isValidWalletID('short')).toBe(false); // too short
      expect(isValidWalletID('xyz123def456abc123def456abc123de')).toBe(false); // invalid hex
      expect(isValidWalletID(123)).toBe(false); // not a string
    });

    it('should require minimum length of 32 characters', () => {
      const id31 = 'abc123def456abc123def456abc123d'; // 31 chars
      expect(isValidWalletID(id31)).toBe(false);

      const id32 = 'abc123def456abc123def456abc123de'; // 32 chars
      expect(isValidWalletID(id32)).toBe(true);
    });

    it('should require hex characters only', () => {
      const nonHexID = 'ghijklmnopqrstuvwxyz123456789012'; // 32 chars but not hex
      expect(isValidWalletID(nonHexID)).toBe(false);
    });
  });
});
