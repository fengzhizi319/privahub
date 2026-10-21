/**
 * Java-contract request helpers for the migration extension APIs.
 *
 * Request/response field names follow the legacy Java SecretPad contract
 * (`frontend-src/apps/platform/src/services/secretpad/typings.d.ts`). The Go
 * backend is being aligned to exactly this contract, so ext methods pass
 * bodies through verbatim and only do light, tolerant normalisation.
 */
import { sessionFetch } from '../session';
import { api } from '../api';

export type JavaResponse<T> = { status?: { code: number; msg?: string }; data?: T };

/** Java `SecretPadPageResponse<T>` / `PageResponse<T>` tolerant shape. */
export interface JavaPage<T> {
  list: T[];
  total: number;
}

function errMsg(error: unknown): string {
  if (!error) return 'Unknown error';
  if (typeof error === 'string') return error;
  const e = error as { msg?: string; message?: string; status?: { msg?: string } };
  return e.msg || e.message || e.status?.msg || JSON.stringify(error);
}

/** POST `/api/v1alpha1/<path>` with a JSON body; unwrap `{status,data}`. `data` may be null. */
export async function javaPost<T = unknown>(path: string, body: unknown = {}): Promise<T> {
  const url = path.startsWith('/api') ? path : `/api/v1alpha1/${path.replace(/^\//, '')}`;
  // 动态路径不在 OpenAPI 类型里：以最小签名调用。
  const post = api.POST as unknown as (url: string, init: { body: unknown }) => Promise<{ data?: unknown; error?: unknown }>;
  const { data, error } = await post(url, { body });
  if (error) throw new Error(errMsg(error));
  const res = data as JavaResponse<T> | undefined;
  if (res?.status && res.status.code !== 0) {
    throw new Error(res.status.msg || `API error ${res.status.code}`);
  }
  return res?.data as T;
}

/** Normalise a page payload that may be `{list,total}`, `{data,total}`, `{data,totalCount}` or an array. */
export function toPage<T>(payload: unknown, listKeys: string[] = []): JavaPage<T> {
  if (Array.isArray(payload)) return { list: payload as T[], total: payload.length };
  const p = (payload || {}) as Record<string, unknown>;
  for (const k of [...listKeys, 'list', 'data', 'infos', 'records']) {
    if (Array.isArray(p[k])) {
      const list = p[k] as T[];
      const total = Number(p.total ?? p.totalCount ?? p.totalNum ?? list.length) || list.length;
      return { list, total };
    }
  }
  return { list: [], total: 0 };
}

/** Multipart POST (token header injected) returning the unwrapped `data`. */
export async function javaPostMultipart<T = unknown>(path: string, form: FormData): Promise<T> {
  const url = path.startsWith('/api') ? path : `/api/v1alpha1/${path.replace(/^\//, '')}`;
  const headers: Record<string, string> = {};
  const response = await sessionFetch(url, { method: 'POST', headers, body: form });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const json = (await response.json()) as JavaResponse<T>;
  if (json.status && json.status.code !== 0) throw new Error(json.status.msg || `API error ${json.status.code}`);
  return json.data as T;
}
