/**
 * Password policy shared by account password change and edge-account reset.
 * Mirrors the legacy SecretPad rule: 8–20 chars, must contain upper-case,
 * lower-case letters and digits; the new password must differ from the old.
 */
export type PasswordPolicyError = 'length' | 'complexity' | 'sameAsOld' | 'mismatch';

export const PASSWORD_MIN_LENGTH = 8;
export const PASSWORD_MAX_LENGTH = 20;

export function checkPasswordStrength(password: string): PasswordPolicyError | null {
  if (password.length < PASSWORD_MIN_LENGTH || password.length > PASSWORD_MAX_LENGTH) return 'length';
  if (!/[A-Z]/.test(password) || !/[a-z]/.test(password) || !/\d/.test(password)) return 'complexity';
  return null;
}

export function validatePasswordChange(input: {
  oldPassword?: string;
  newPassword: string;
  confirmPassword?: string;
}): PasswordPolicyError | null {
  const strength = checkPasswordStrength(input.newPassword);
  if (strength) return strength;
  if (input.oldPassword !== undefined && input.oldPassword === input.newPassword) return 'sameAsOld';
  if (input.confirmPassword !== undefined && input.confirmPassword !== input.newPassword) return 'mismatch';
  return null;
}
