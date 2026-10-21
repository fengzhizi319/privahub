import { describe, expect, it } from 'vitest';
import { TOKEN_TTL_MS, getTokenExpiry, parseTime, tokenRefetchInterval } from './token-expiry';

describe('token expiry', () => {
  const issued = Date.parse('2026-09-24T10:00:00Z');

  it('parses several time formats', () => {
    expect(parseTime('2026-09-24T10:00:00Z')).toBe(issued);
    expect(parseTime(String(issued))).toBe(issued);
    expect(parseTime(issued)).toBe(issued);
    expect(parseTime('')).toBeNull();
    expect(parseTime('garbage')).toBeNull();
    expect(parseTime('2026-09-24 10:00:00')).not.toBeNull();
  });

  it('computes minutes left within the 30 minute validity', () => {
    const e = getTokenExpiry(issued, issued + 10 * 60 * 1000);
    expect(e.expired).toBe(false);
    expect(e.minutesLeft).toBe(20);
    expect(e.expiresAt).toBe(issued + TOKEN_TTL_MS);
  });

  it('flags expired tokens', () => {
    const e = getTokenExpiry(issued, issued + 31 * 60 * 1000);
    expect(e.expired).toBe(true);
    expect(e.minutesLeft).toBe(0);
  });

  it('unknown issue time is never expired', () => {
    expect(getTokenExpiry(undefined)).toEqual({ expiresAt: null, expired: false, minutesLeft: null });
  });

  it('refetch interval follows the expiry, bounded', () => {
    expect(tokenRefetchInterval(undefined)).toBe(TOKEN_TTL_MS);
    expect(tokenRefetchInterval(issued, issued + 25 * 60 * 1000)).toBe(5 * 60 * 1000);
    expect(tokenRefetchInterval(issued, issued + 40 * 60 * 1000)).toBe(10_000);
    expect(tokenRefetchInterval(issued, issued)).toBe(TOKEN_TTL_MS);
  });
});
