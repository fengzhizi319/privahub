/**
 * 审计日志页面的纯函数（便于单元测试）：过滤条件 → 请求体、结果徽标映射。
 */
import type { AuditListRequest } from '@secretpad/api-client';

export interface AuditFilters {
  /** datetime-local 值（本地时区），空串表示不限 */
  start: string;
  end: string;
  actor: string;
  action: string;
  result: string;
  resourceType: string;
  resourceId: string;
  ip: string;
}

export const EMPTY_FILTERS: AuditFilters = {
  start: '',
  end: '',
  actor: '',
  action: '',
  result: '',
  resourceType: '',
  resourceId: '',
  ip: '',
};

export const PAGE_SIZE = 20;

function toMillis(local: string): number | undefined {
  if (!local) return undefined;
  const ms = new Date(local).getTime();
  return Number.isNaN(ms) ? undefined : ms;
}

/** Build the audit/list request: blank filters are omitted, times become unix ms. */
export function toAuditRequest(f: AuditFilters, page: number, size = PAGE_SIZE): AuditListRequest {
  const req: AuditListRequest = { page, size };
  const start = toMillis(f.start);
  const end = toMillis(f.end);
  if (start !== undefined) req.startTime = start;
  if (end !== undefined) req.endTime = end;
  const text: (keyof AuditFilters & keyof AuditListRequest)[] = [
    'actor',
    'action',
    'result',
    'resourceType',
    'resourceId',
    'ip',
  ];
  for (const k of text) {
    const v = f[k].trim();
    if (v) (req as Record<string, unknown>)[k] = v;
  }
  return req;
}

export type BadgeStatus = 'success' | 'processing' | 'warning' | 'error' | 'default';

export function resultBadge(result: string): BadgeStatus {
  switch (result) {
    case 'success':
      return 'success';
    case 'denied':
      return 'warning';
    case 'failure':
      return 'error';
    default:
      return 'default';
  }
}

/** Backend AUTH_FAILED (202011602) / permission messages. */
export function isPermissionError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err ?? '');
  return /202011602|permission|AUDITOR|权限/i.test(msg);
}

/** Shorten a hash for display. */
export function shortHash(h: string): string {
  return h && h.length > 16 ? `${h.slice(0, 8)}…${h.slice(-8)}` : h;
}
