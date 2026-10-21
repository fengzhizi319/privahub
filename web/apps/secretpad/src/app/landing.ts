import type { User } from '@secretpad/api-client';
import { FIRST_LOGIN_KEY, getStoredUser } from '../features/auth/model/auth-store';
import { toPlatformContext } from '../shared/lib/platform';
import { resolveHomePath, type HomeTarget } from '../shared/lib/access';

/** Landing target after login (CENTER first login → /guide). Marks the first login as consumed. */
export function landingAfterLogin(user?: User | null): HomeTarget {
  const firstLogin = !localStorage.getItem(FIRST_LOGIN_KEY);
  const target = resolveHomePath(toPlatformContext(user ?? getStoredUser()), { firstLogin });
  localStorage.setItem(FIRST_LOGIN_KEY, 'true');
  return target;
}
