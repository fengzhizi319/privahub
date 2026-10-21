/** Java-contract extension APIs: user. */
import { javaPost } from './core';

/** Java `UserUpdatePwdRequest` plus SM3 twins of the hashes for legacy (SM3-stored) accounts. */
export interface UserUpdatePwdRequest {
  name?: string;
  oldPasswordHash: string;
  newPasswordHash: string;
  confirmPasswordHash: string;
  oldPasswordHashSm3?: string;
  newPasswordHashSm3?: string;
  confirmPasswordHashSm3?: string;
}

/** POST user/updatePwd → boolean. */
export async function updateUserPassword(req: UserUpdatePwdRequest): Promise<boolean> {
  const res = await javaPost<boolean | null>('user/updatePwd', req);
  return res !== false;
}
