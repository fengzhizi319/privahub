import { describe, it, expect } from 'vitest';
import { sha256 } from './crypto';

describe('sha256', () => {
  it('hashes the default dev password the same way the legacy crypto-js implementation did', async () => {
    const hash = await sha256('12345678');
    expect(hash).toBe('ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f');
  });

  it('returns lowercase hex string', async () => {
    const hash = await sha256('SecretPad');
    expect(hash).toMatch(/^[a-f0-9]{64}$/);
  });
});
