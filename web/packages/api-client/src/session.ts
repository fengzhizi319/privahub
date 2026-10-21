/**
 * Browser session helpers (HttpOnly cookie session + CSRF double submit).
 *
 * The session token is no longer stored in localStorage: the backend sets it as
 * an HttpOnly cookie (`__Host-privahub_session`, or `privahub_session` over
 * plain-HTTP development) that page scripts cannot read. State-changing
 * requests must echo the readable CSRF cookie in the `X-CSRF-Token` header.
 * Only the non-secret user context (`secretpad-user`) is kept in localStorage
 * as the "logged in" marker for route guards.
 */

/** Legacy key of the bearer token (removed on load; never written again). */
export const LEGACY_TOKEN_KEY = 'secretpad-token';
/** Non-secret persisted user context (login marker for route guards). */
export const USER_KEY = 'secretpad-user';

export const CSRF_HEADER = 'X-CSRF-Token';
export const SESSION_MODE_HEADER = 'X-Privahub-Session';
const CSRF_COOKIES = ['__Host-privahub_csrf', 'privahub_csrf'];

/** Backend code: the session is restricted until the password is changed. */
export const PASSWORD_CHANGE_REQUIRED_CODE = 202011605;
export const CHANGE_PASSWORD_PATH = '/change-password';

const REFRESH_PATH = '/api/user/refresh';

/** Read the CSRF double-submit token from its (non-HttpOnly) cookie. */
export function readCsrfToken(): string {
  if (typeof document === 'undefined' || !document.cookie) return '';
  const jar = new Map<string, string>();
  for (const part of document.cookie.split(';')) {
    const i = part.indexOf('=');
    if (i < 0) continue;
    jar.set(part.slice(0, i).trim(), decodeURIComponent(part.slice(i + 1).trim()));
  }
  for (const name of CSRF_COOKIES) {
    const v = jar.get(name);
    if (v) return v;
  }
  return '';
}

export function isStateChanging(method: string): boolean {
  return !['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase());
}

/** Headers for a raw `fetch` (multipart upload / download). */
export function sessionHeaders(method = 'POST'): Record<string, string> {
  const headers: Record<string, string> = {};
  if (isStateChanging(method)) {
    const csrf = readCsrfToken();
    if (csrf) headers[CSRF_HEADER] = csrf;
  }
  return headers;
}

/** `fetch` with the session cookie and CSRF header attached. */
export function sessionFetch(input: string, init: RequestInit = {}): Promise<Response> {
  const method = (init.method || 'GET').toUpperCase();
  const headers = new Headers(init.headers || {});
  for (const [k, v] of Object.entries(sessionHeaders(method))) headers.set(k, v);
  return fetch(input, { ...init, headers, credentials: 'include' });
}

/**
 * Remove the bearer token that older builds kept in localStorage (readable by
 * any script on the page). The user has to log in once to obtain a cookie
 * session.
 */
export function migrateLegacyTokenStorage(): void {
  try {
    if (typeof localStorage === 'undefined') return;
    if (localStorage.getItem(LEGACY_TOKEN_KEY) !== null) {
      localStorage.removeItem(LEGACY_TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    }
  } catch {
    /* storage unavailable */
  }
}

/** Whether a (non-secret) user context marks the browser as logged in. */
export function hasStoredSession(): boolean {
  try {
    return typeof localStorage !== 'undefined' && !!localStorage.getItem(USER_KEY);
  } catch {
    return false;
  }
}

export function clearStoredSession(): void {
  try {
    if (typeof localStorage === 'undefined') return;
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(LEGACY_TOKEN_KEY);
  } catch {
    /* storage unavailable */
  }
}

let refreshing: Promise<boolean> | null = null;

/**
 * Rotate the session with the HttpOnly refresh cookie (single flight: parallel
 * 401s share one refresh, because refresh tokens are one-time-use and a second
 * concurrent use would be treated as token theft by the backend).
 */
export function refreshSession(baseUrl = ''): Promise<boolean> {
  if (!refreshing) {
    refreshing = (async () => {
      try {
        const res = await sessionFetch(`${baseUrl}${REFRESH_PATH}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', [SESSION_MODE_HEADER]: 'cookie' },
          body: '{}',
        });
        if (!res.ok) return false;
        const json = (await res.json().catch(() => null)) as { status?: { code?: number } } | null;
        return json?.status?.code === 0;
      } catch {
        return false;
      } finally {
        setTimeout(() => {
          refreshing = null;
        }, 0);
      }
    })();
  }
  return refreshing;
}

export function isAuthPath(pathname: string): boolean {
  return (
    pathname.endsWith('/api/login') ||
    pathname.endsWith('/user/login') ||
    pathname.endsWith('/api/logout') ||
    pathname.endsWith('/user/logout') ||
    pathname.endsWith(REFRESH_PATH)
  );
}
