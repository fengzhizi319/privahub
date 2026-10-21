import { describe, it, expect, beforeEach } from 'vitest';
import { formatDate, formatBytes, storage, ok, err } from './index';

describe('formatDate', () => {
  it('formats a date string', () => {
    expect(formatDate('2026-07-26T10:00:00.000Z', 'YYYY-MM-DD HH:mm:ss')).toMatch(/2026-07-26 \d{2}:00:00/);
  });

  it('returns "-" for invalid date', () => {
    expect(formatDate('invalid')).toBe('-');
  });
});

describe('formatBytes', () => {
  it('returns 0 B for zero bytes', () => {
    expect(formatBytes(0)).toBe('0 B');
  });

  it('formats KB correctly', () => {
    expect(formatBytes(2048)).toBe('2 KB');
  });
});

describe('storage', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('round-trips JSON values', () => {
    storage.set('key', { value: 42 });
    expect(storage.get('key', { value: 0 })).toEqual({ value: 42 });
  });

  it('returns default value for missing key', () => {
    expect(storage.get('missing', 'default')).toBe('default');
  });
});

describe('Result helpers', () => {
  it('creates an ok result', () => {
    const result = ok(42);
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.value).toBe(42);
  });

  it('creates an err result', () => {
    const result = err('failure');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error).toBe('failure');
  });
});
