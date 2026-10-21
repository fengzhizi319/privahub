import { describe, expect, it } from 'vitest';
import { sha256 } from '@secretpad/utils';
import { buildResetPwdRequest, hasEdgePasswordErrors, validateEdgePassword } from './password-validation';

describe('validateEdgePassword', () => {
  it('requires all fields', () => {
    expect(validateEdgePassword({ oldPassword: '', newPassword: '', confirmPassword: '' })).toEqual({
      oldPassword: 'required',
      newPassword: 'required',
      confirmPassword: 'required',
    });
  });

  it('enforces the password policy on old and new passwords', () => {
    const e = validateEdgePassword({ oldPassword: 'short', newPassword: 'alllowercase1', confirmPassword: 'alllowercase1' });
    expect(e.oldPassword).toBe('length');
    expect(e.newPassword).toBe('complexity');
  });

  it('rejects a new password equal to the old one', () => {
    const e = validateEdgePassword({ oldPassword: 'Abcdef123', newPassword: 'Abcdef123', confirmPassword: 'Abcdef123' });
    expect(e.newPassword).toBe('sameAsOld');
  });

  it('rejects a mismatching confirmation', () => {
    const e = validateEdgePassword({ oldPassword: 'Abcdef123', newPassword: 'Newpass123', confirmPassword: 'Newpass124' });
    expect(e).toEqual({ confirmPassword: 'mismatch' });
  });

  it('accepts a valid change', () => {
    const e = validateEdgePassword({ oldPassword: 'Abcdef123', newPassword: 'Newpass123', confirmPassword: 'Newpass123' });
    expect(hasEdgePasswordErrors(e)).toBe(false);
  });
});

describe('buildResetPwdRequest', () => {
  it('hashes both passwords with sha256', async () => {
    const req = await buildResetPwdRequest('alice', 'alice', 'Abcdef123', 'Newpass123');
    expect(req).toEqual({
      nodeId: 'alice',
      name: 'alice',
      passwordHash: await sha256('Abcdef123'),
      newPasswordHash: await sha256('Newpass123'),
    });
    expect(req.passwordHash).toMatch(/^[0-9a-f]{64}$/);
  });
});
