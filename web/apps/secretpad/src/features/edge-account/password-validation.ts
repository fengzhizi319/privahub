import { checkPasswordStrength, sha256, validatePasswordChange, type PasswordPolicyError } from '@secretpad/utils';
import type { ResetNodeUserPwdRequestJava } from '@secretpad/api-client';

export interface EdgePasswordInput {
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export type EdgePasswordError = PasswordPolicyError | 'required';

export interface EdgePasswordErrors {
  oldPassword?: EdgePasswordError;
  newPassword?: EdgePasswordError;
  confirmPassword?: EdgePasswordError;
}

/**
 * Field-level validation of the edge-account password dialog (legacy my-node
 * "设置密码"): all three fields required; old & new must satisfy the password
 * policy (8–20 chars, upper + lower + digit); new must differ from old and
 * match the confirmation.
 */
export function validateEdgePassword(input: EdgePasswordInput): EdgePasswordErrors {
  const errors: EdgePasswordErrors = {};
  if (!input.oldPassword) errors.oldPassword = 'required';
  else {
    const s = checkPasswordStrength(input.oldPassword);
    if (s) errors.oldPassword = s;
  }
  if (!input.newPassword) errors.newPassword = 'required';
  if (!input.confirmPassword) errors.confirmPassword = 'required';
  if (input.newPassword) {
    const e = validatePasswordChange({ oldPassword: input.oldPassword || undefined, newPassword: input.newPassword });
    if (e) errors.newPassword = e;
  }
  if (input.confirmPassword && input.newPassword && input.confirmPassword !== input.newPassword) {
    errors.confirmPassword = 'mismatch';
  }
  return errors;
}

export const hasEdgePasswordErrors = (e: EdgePasswordErrors) => Object.keys(e).length > 0;

/** Build the Java `ResetNodeUserPwdRequest` (passwords hashed with sha256, like the account page). */
export async function buildResetPwdRequest(
  nodeId: string,
  name: string,
  oldPassword: string,
  newPassword: string,
): Promise<ResetNodeUserPwdRequestJava> {
  const [passwordHash, newPasswordHash] = await Promise.all([sha256(oldPassword), sha256(newPassword)]);
  return { nodeId, name, passwordHash, newPasswordHash };
}
