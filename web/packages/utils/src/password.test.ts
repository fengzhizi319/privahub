import { describe, it, expect } from 'vitest';
import { checkPasswordStrength, validatePasswordChange } from './password';

describe('password policy', () => {
  it('rejects too short / too long passwords', () => {
    expect(checkPasswordStrength('Ab1')).toBe('length');
    expect(checkPasswordStrength('Ab1' + 'x'.repeat(20))).toBe('length');
  });

  it('requires upper, lower and digit', () => {
    expect(checkPasswordStrength('abcdefgh1')).toBe('complexity');
    expect(checkPasswordStrength('ABCDEFGH1')).toBe('complexity');
    expect(checkPasswordStrength('Abcdefghi')).toBe('complexity');
    expect(checkPasswordStrength('Abcdefg1')).toBeNull();
  });

  it('rejects new password equal to old and confirmation mismatch', () => {
    expect(validatePasswordChange({ oldPassword: 'Abcdefg1', newPassword: 'Abcdefg1' })).toBe('sameAsOld');
    expect(validatePasswordChange({ oldPassword: 'x', newPassword: 'Abcdefg1', confirmPassword: 'Abcdefg2' })).toBe(
      'mismatch',
    );
    expect(validatePasswordChange({ oldPassword: 'x', newPassword: 'Abcdefg1', confirmPassword: 'Abcdefg1' })).toBeNull();
  });
});
