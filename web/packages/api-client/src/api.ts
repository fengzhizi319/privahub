import createClient from 'openapi-fetch';
import type { paths } from './generated/secretpad';
import { ownerFieldFor, storedDataResourceUser, withDataResourceOwner } from './data-resource';
import {
  CHANGE_PASSWORD_PATH,
  CSRF_HEADER,
  PASSWORD_CHANGE_REQUIRED_CODE,
  SESSION_MODE_HEADER,
  clearStoredSession,
  isAuthPath,
  isStateChanging,
  migrateLegacyTokenStorage,
  readCsrfToken,
  refreshSession,
} from './session';

// Avoid coupling the shared API client to Vite's import.meta typings.
// Paths in the generated client already include the `/api` prefix (e.g. `/api/login`,
// `/api/v1alpha1/node/list`). In dev Vite proxies `/api/*` to the backend; in production
// the Spring Boot app serves `/api/*` directly, so an empty base URL keeps the paths absolute.
const API_BASE_URL =
  (typeof import.meta !== 'undefined' && (import.meta as { env?: Record<string, string | undefined> }).env?.VITE_API_BASE_URL) || '';

export const api = createClient<paths>({
  baseUrl: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
  // The session is an HttpOnly cookie: send it (also to a cross-origin API base).
  credentials: 'include',
});

// Older builds kept the bearer token in localStorage: drop it on load.
migrateLegacyTokenStorage();

function generateTraceId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

/** 后端限流（REQUEST_FREQUENCY_ERROR）。 */
export const RATE_LIMIT_CODE = 2020111012;
export const RATE_LIMIT_MESSAGE = '请求过于频繁，请稍后再试（Too many requests, please retry later）';

/**
 * 全局错误改写：限流时把后端原始 msg 换成友好提示，所有调用方（toast / 错误条）自动生效；
 * 同时派发 `secretpad:rate-limited` 事件供页面按需提示。
 */
async function rewriteRateLimited(response: Response): Promise<Response> {
  const isJson = (response.headers.get('content-type') || '').includes('application/json');
  if (!isJson && response.status !== 429) return response;
  let body: { status?: { code?: number; msg?: string } } | null;
  try {
    body = isJson ? await response.clone().json() : null;
  } catch {
    return response;
  }
  if (response.status !== 429 && body?.status?.code !== RATE_LIMIT_CODE) return response;
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('secretpad:rate-limited'));
  const next = { ...(body || {}), status: { ...(body?.status || {}), code: body?.status?.code ?? RATE_LIMIT_CODE, msg: RATE_LIMIT_MESSAGE } };
  return new Response(JSON.stringify(next), { status: response.status, statusText: response.statusText, headers: response.headers });
}

/**
 * Owner-scoped @DataResource routes: fill the caller's own id (ownerId / instId /
 * initiatorId / voteParticipantId) when the JSON body omits it, so non-CENTER
 * accounts are not rejected with AUTH_FAILED. See ./data-resource.ts.
 */
async function ensureDataResourceOwner(request: Request): Promise<Request> {
  if (request.method !== 'POST' || !(request.headers.get('content-type') || '').includes('application/json')) return request;
  const user = storedDataResourceUser();
  const route = new URL(request.url, 'http://local').pathname;
  if (!ownerFieldFor(route, user)) return request;
  let body: unknown;
  try {
    const text = await request.clone().text();
    body = text ? JSON.parse(text) : {};
  } catch {
    return request;
  }
  const next = withDataResourceOwner(route, body, user);
  if (next === body) return request;
  return new Request(request, { body: JSON.stringify(next) });
}

/** Clones of in-flight requests, so a request can be replayed once after a refresh. */
const replayable = new WeakMap<Request, Request>();

function redirectTo(path: string) {
  if (typeof window === 'undefined') return;
  if (window.location.pathname === path) return;
  // Use replace so the broken route is not kept in the history stack.
  window.location.replace(path);
}

async function isPasswordChangeRequired(response: Response): Promise<boolean> {
  if (response.status !== 403) return false;
  try {
    const body = (await response.clone().json()) as { status?: { code?: number } };
    return body?.status?.code === PASSWORD_CHANGE_REQUIRED_CODE;
  } catch {
    return false;
  }
}

api.use({
  async onRequest({ request }) {
    request.headers.set('Trace-Id', generateTraceId());
    const path = new URL(request.url, 'http://local').pathname;
    if (isAuthPath(path)) {
      // Browser session: tokens only as HttpOnly cookies, never in the body.
      request.headers.set(SESSION_MODE_HEADER, 'cookie');
    }
    if (isStateChanging(request.method)) {
      const csrf = readCsrfToken();
      if (csrf) request.headers.set(CSRF_HEADER, csrf);
    }
    const next = await ensureDataResourceOwner(request);
    if (!isAuthPath(path)) {
      try {
        replayable.set(next, next.clone());
      } catch {
        /* body not clonable */
      }
    }
    return next;
  },
  async onResponse({ request, response }) {
    const path = new URL(request.url, 'http://local').pathname;
    if (response.status === 401 && !isAuthPath(path)) {
      // Access token expired: rotate once with the refresh cookie and replay.
      const replay = replayable.get(request);
      if (replay && (await refreshSession(API_BASE_URL))) {
        if (isStateChanging(replay.method)) {
          const csrf = readCsrfToken();
          if (csrf) replay.headers.set(CSRF_HEADER, csrf);
        }
        const retried = await fetch(replay, { credentials: 'include' });
        if (retried.status !== 401) return rewriteRateLimited(retried);
      }
      clearStoredSession();
      redirectTo('/login');
    } else if (await isPasswordChangeRequired(response)) {
      redirectTo(CHANGE_PASSWORD_PATH);
    }
    return rewriteRateLimited(response);
  },
});

export type ApiClient = typeof api;
