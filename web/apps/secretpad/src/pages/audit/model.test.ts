import { describe, expect, it } from 'vitest';
import { EMPTY_FILTERS, isPermissionError, resultBadge, shortHash, toAuditRequest } from './model';

describe('audit page model', () => {
  it('omits blank filters and converts times to unix ms', () => {
    expect(toAuditRequest(EMPTY_FILTERS, 1)).toEqual({ page: 1, size: 20 });
    const req = toAuditRequest(
      { ...EMPTY_FILTERS, actor: ' alice ', result: 'denied', start: '2026-01-02T03:04', action: '' },
      3,
      50,
    );
    expect(req).toMatchObject({ page: 3, size: 50, actor: 'alice', result: 'denied' });
    expect(req.startTime).toBe(new Date('2026-01-02T03:04').getTime());
    expect(req).not.toHaveProperty('action');
    expect(req).not.toHaveProperty('endTime');
  });

  it('maps results to badge statuses', () => {
    expect(resultBadge('success')).toBe('success');
    expect(resultBadge('denied')).toBe('warning');
    expect(resultBadge('failure')).toBe('error');
    expect(resultBadge('x')).toBe('default');
  });

  it('detects permission errors and shortens hashes', () => {
    expect(isPermissionError(new Error('audit log access requires the AUDITOR or ADMIN role'))).toBe(true);
    expect(isPermissionError(new Error('network'))).toBe(false);
    expect(shortHash('a'.repeat(64))).toBe('aaaaaaaa…aaaaaaaa');
  });
});
