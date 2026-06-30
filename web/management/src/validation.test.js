import { describe, it, expect } from 'vitest';
import { parseTag, validateScope, validateDuration, validateJsonObject, validatePositiveInteger, validateNonNegativeInteger, validateDate } from './validation.js';

describe('Validation Library', () => {
  describe('parseTag', () => {
    it('parses key:value correctly', () => {
      expect(parseTag('region:us')).toEqual({ key: 'region', value: 'us' });
    });

    it('parses key=value correctly', () => {
      expect(parseTag('type=residential')).toEqual({ key: 'type', value: 'residential' });
    });

    it('rejects bare wildcard *', () => {
      expect(() => parseTag('*')).toThrow();
    });

    it('rejects invalid formats', () => {
      expect(() => parseTag('invalidtag')).toThrow();
    });

    it('supports empty value with warning indicator', () => {
      const parsed = parseTag('region:');
      expect(parsed.key).toBe('region');
      expect(parsed.value).toBe('');
      expect(parsed.warning).toBeTruthy();
    });

    it('handles empty string', () => {
      expect(() => parseTag('')).toThrow();
    });

    it('handles whitespace-only string', () => {
      expect(() => parseTag('   ')).toThrow();
    });
  });

  describe('validateScope', () => {
    it('allows * wildcard', () => {
      expect(validateScope('*')).toBe(true);
    });

    it('allows prefix wildcards', () => {
      expect(validateScope('target:*')).toBe(true);
    });

    it('allows suffix wildcards', () => {
      expect(validateScope('*:us')).toBe(true);
    });

    it('rejects malformed wildcards', () => {
      expect(() => validateScope('region:*:us')).toThrow();
    });

    it('rejects empty scope', () => {
      expect(() => validateScope('')).toThrow();
    });

    it('accepts exact tag scopes', () => {
      expect(validateScope('region:us')).toBe(true);
      expect(validateScope('type:residential')).toBe(true);
    });
  });

  describe('validateDuration', () => {
    it('accepts valid Go durations', () => {
      expect(validateDuration('300ms')).toBe(true);
      expect(validateDuration('1.5h')).toBe(true);
      expect(validateDuration('2h45m')).toBe(true);
    });

    it('rejects natural language durations', () => {
      expect(() => validateDuration('30 seconds')).toThrow();
      expect(() => validateDuration('1 minute')).toThrow();
    });

    it('allows empty string', () => {
      expect(validateDuration('')).toBe(true);
    });

    it('accepts seconds and minutes', () => {
      expect(validateDuration('30s')).toBe(true);
      expect(validateDuration('1m')).toBe(true);
    });
  });

  describe('validateDate', () => {
    it('accepts valid YYYY-MM-DD dates', () => {
      expect(validateDate('2026-06-30')).toBe(true);
      expect(validateDate('2026-01-01')).toBe(true);
    });

    it('rejects invalid date formats', () => {
      expect(() => validateDate('06-30-2026')).toThrow();
      expect(() => validateDate('2026/06/30')).toThrow();
      expect(() => validateDate('2026-6-30')).toThrow();
    });

    it('rejects completely invalid dates', () => {
      expect(() => validateDate('not-a-date')).toThrow();
      expect(() => validateDate('2026-abc')).toThrow();
    });

    it('allows empty string', () => {
      expect(validateDate('')).toBe(true);
    });
  });

  describe('validateJsonObject', () => {
    it('accepts correct JSON objects', () => {
      expect(validateJsonObject('{"headers":["User-Agent"]}')).toEqual({ headers: ['User-Agent'] });
    });

    it('rejects arrays', () => {
      expect(() => validateJsonObject('["item"]')).toThrow();
    });

    it('rejects strings', () => {
      expect(() => validateJsonObject('"just a string"')).toThrow();
    });

    it('rejects null', () => {
      expect(() => validateJsonObject('null')).toThrow();
    });

    it('rejects empty string', () => {
      expect(() => validateJsonObject('')).toThrow();
    });

    it('rejects invalid JSON syntax', () => {
      expect(() => validateJsonObject('{invalid}')).toThrow();
    });
  });

  describe('validatePositiveInteger', () => {
    it('accepts positive integers', () => {
      expect(validatePositiveInteger(1, 'Page size')).toBe(1);
      expect(validatePositiveInteger(100, 'Page size')).toBe(100);
    });

    it('rejects zero', () => {
      expect(() => validatePositiveInteger(0, 'Page size')).toThrow();
    });

    it('rejects negative numbers', () => {
      expect(() => validatePositiveInteger(-1, 'Page size')).toThrow();
    });

    it('rejects floats', () => {
      expect(() => validatePositiveInteger(1.5, 'Page size')).toThrow();
    });
  });

  describe('validateNonNegativeInteger', () => {
    it('accepts zero and positive integers', () => {
      expect(validateNonNegativeInteger(0, 'Max retries')).toBe(0);
      expect(validateNonNegativeInteger(5, 'Max retries')).toBe(5);
    });

    it('rejects negative numbers', () => {
      expect(() => validateNonNegativeInteger(-1, 'Max retries')).toThrow();
    });

    it('rejects floats', () => {
      expect(() => validateNonNegativeInteger(1.5, 'Max retries')).toThrow();
    });
  });
});
