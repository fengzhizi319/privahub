import { describe, it, expect } from 'vitest';
import { formatBadgeCount } from './header-utils';

describe('formatBadgeCount', () => {
  it('hides zero / negative counts and caps large ones', () => {
    expect(formatBadgeCount(0)).toBe('');
    expect(formatBadgeCount(-1)).toBe('');
    expect(formatBadgeCount(7)).toBe('7');
    expect(formatBadgeCount(120)).toBe('99+');
  });
});
