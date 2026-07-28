import createClient from 'openapi-fetch';
import type { paths } from './generated/secretpad';

// Avoid coupling the shared API client to Vite's import.meta typings.
// Paths in the generated client already include the `/api` prefix (e.g. `/api/login`,
// `/api/v1alpha1/node/list`). In dev Vite proxies `/api/*` to the backend; in production
// the Spring Boot app serves `/api/*` directly, so an empty base URL keeps the paths absolute.
const API_BASE_URL =
  (typeof import.meta !== 'undefined' && (import.meta as any).env?.VITE_API_BASE_URL) || '';

export const api = createClient<paths>({
  baseUrl: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

function generateTraceId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

api.use({
  onRequest({ request }) {
    const token = typeof localStorage !== 'undefined' ? localStorage.getItem('secretpad-token') : null;
    if (token) {
      request.headers.set('User-Token', token);
    }
    request.headers.set('Trace-Id', generateTraceId());
    return request;
  },
  onResponse({ response }) {
    if (response.status === 401) {
      if (typeof localStorage !== 'undefined') {
        localStorage.removeItem('secretpad-token');
        localStorage.removeItem('secretpad-user');
      }
      if (typeof window !== 'undefined') {
        // Use replace so the broken route is not kept in the history stack.
        window.location.replace('/login');
      }
    }
    return response;
  },
});

export type ApiClient = typeof api;
