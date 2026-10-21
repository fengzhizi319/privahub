import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { http, HttpResponse } from 'msw';

import { api } from './api';
import { apiClient } from './client';
import { migrateLegacyTokenStorage, readCsrfToken, sessionHeaders } from './session';
import { server } from '../../../test/mocks/server';

const BASE = 'http://localhost';
const ok = (data: unknown) => HttpResponse.json({ status: { code: 0, msg: 'success' }, data });

function clearCookies() {
  for (const c of document.cookie.split(';')) {
    const name = c.split('=')[0].trim();
    if (name) document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
  }
}

let replace: ReturnType<typeof vi.fn>;
const originalLocation = window.location;

beforeEach(() => {
  localStorage.clear();
  clearCookies();
  replace = vi.fn();
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...originalLocation, pathname: '/dashboard', replace },
  });
});

afterEach(() => {
  Object.defineProperty(window, 'location', { configurable: true, value: originalLocation });
});

describe('session helpers', () => {
  it('reads the CSRF cookie (secure name first) and builds headers for state-changing methods only', () => {
    document.cookie = 'privahub_csrf=plain';
    expect(readCsrfToken()).toBe('plain');
    expect(sessionHeaders('POST')).toEqual({ 'X-CSRF-Token': 'plain' });
    expect(sessionHeaders('GET')).toEqual({});
  });

  it('migrates a legacy localStorage token away', () => {
    localStorage.setItem('secretpad-token', 'leaked');
    localStorage.setItem('secretpad-user', '{"name":"a"}');
    migrateLegacyTokenStorage();
    expect(localStorage.getItem('secretpad-token')).toBeNull();
    expect(localStorage.getItem('secretpad-user')).toBeNull();
  });
});

describe('api middleware', () => {
  it('sends the CSRF header and no token headers', async () => {
    document.cookie = 'privahub_csrf=csrf-1';
    let seen: Headers | undefined;
    server.use(
      http.post(`${BASE}/api/v1alpha1/node/list`, ({ request }) => {
        seen = request.headers;
        return ok([]);
      })
    );
    await apiClient.getNodes();
    expect(seen?.get('X-CSRF-Token')).toBe('csrf-1');
    expect(seen?.get('User-Token')).toBeNull();
    expect(seen?.get('Authorization')).toBeNull();
  });

  it('asks for a cookie-only session on login', async () => {
    let mode: string | null = null;
    server.use(
      http.post(`${BASE}/api/login`, ({ request }) => {
        mode = request.headers.get('X-Privahub-Session');
        return ok({ name: 'admin', ownerId: 'k', platformType: 'CENTER', mustChangePassword: true });
      })
    );
    const user = await apiClient.login('admin', 'h');
    expect(mode).toBe('cookie');
    expect(user.mustChangePassword).toBe(true);
  });

  it('refreshes once on 401 and replays the request', async () => {
    let calls = 0;
    let refreshes = 0;
    server.use(
      http.post(`${BASE}/api/v1alpha1/node/list`, () => {
        calls += 1;
        return calls === 1 ? HttpResponse.json({ status: { code: 202011602 } }, { status: 401 }) : ok([{ nodeId: 'alice' }]);
      }),
      http.post(`${BASE}/api/user/refresh`, () => {
        refreshes += 1;
        return ok({ token_type: 'Cookie' });
      })
    );
    const nodes = await apiClient.getNodes();
    expect(nodes[0].nodeId).toBe('alice');
    expect(refreshes).toBe(1);
    expect(replace).not.toHaveBeenCalled();
  });

  it('clears the session and redirects to /login when the refresh fails', async () => {
    localStorage.setItem('secretpad-user', '{"name":"a"}');
    server.use(
      http.post(`${BASE}/api/v1alpha1/node/list`, () => HttpResponse.json({ status: { code: 202011602 } }, { status: 401 })),
      http.post(`${BASE}/api/user/refresh`, () => HttpResponse.json({ status: { code: 202011602 } }, { status: 401 }))
    );
    await api.POST('/api/v1alpha1/node/list', { body: {} as never });
    expect(localStorage.getItem('secretpad-user')).toBeNull();
    expect(replace).toHaveBeenCalledWith('/login');
  });

  it('redirects to the change-password screen when the session is sandboxed', async () => {
    server.use(
      http.post(`${BASE}/api/v1alpha1/node/list`, () =>
        HttpResponse.json({ status: { code: 202011605, msg: 'Password change required' } }, { status: 403 })
      )
    );
    await api.POST('/api/v1alpha1/node/list', { body: {} as never });
    expect(replace).toHaveBeenCalledWith('/change-password');
  });
});
