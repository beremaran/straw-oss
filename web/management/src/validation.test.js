import { describe, it, expect } from 'vitest';
import { parseTag, validateScope, validateDuration, validateJsonObject, validatePositiveInteger } from './validation.js';

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
  });

  describe('validateJsonObject', () => {
    it('accepts correct JSON objects', () => {
      expect(validateJsonObject('{"headers":["User-Agent"]}')).toEqual({ headers: ['User-Agent'] });
    });

    it('rejects arrays or strings', () => {
      expect(() => validateJsonObject('["item"]')).toThrow();
      expect(() => validateJsonObject('"just a string"')).toThrow();
    });
  });
});
