/**
 * Security audit trail (read-only): `/api/v1alpha1/audit/*` (Go backend,
 * AUDITOR / ADMIN only). There are deliberately no update/delete endpoints.
 */
import { api } from '../api';
import { javaPost, type JavaResponse } from './core';

export type AuditResult = 'success' | 'failure' | 'denied';

export interface AuditListRequest {
  /** unix ms, inclusive */
  startTime?: number;
  /** unix ms, exclusive */
  endTime?: number;
  actor?: string;
  /** substring match */
  action?: string;
  result?: AuditResult | string;
  resourceType?: string;
  resourceId?: string;
  ip?: string;
  traceId?: string;
  page?: number;
  size?: number;
}

export interface AuditRecord {
  seq: number;
  eventId: string;
  time: string;
  actor: string;
  actorType: string;
  ownerId: string;
  ip: string;
  action: string;
  resourceType: string;
  resourceId: string;
  result: string;
  reason: string;
  traceId: string;
  extra?: Record<string, string>;
  prevHash: string;
  hash: string;
}

export interface AuditListResponse {
  total: number;
  page: number;
  size: number;
  list: AuditRecord[];
}

export interface AuditVerifyResult {
  valid: boolean;
  algorithm: string;
  checked: number;
  firstSeq: number;
  lastSeq: number;
  tailSeq: number;
  tailHash: string;
  segments: number;
  archivedThrough: number;
  archivesVerified: number;
  archivesMissing: number;
  anchorsChecked: number;
  brokenSeq?: number;
  brokenId?: number;
  breakKind?: string;
  reason?: string;
  verifiedAt: string;
  anchoringEnabled: boolean;
  anchorWarning?: string;
}

/** POST audit/list: filtered, paged audit records (newest first). */
export async function listAuditLogs(req: AuditListRequest): Promise<AuditListResponse> {
  const data = await javaPost<AuditListResponse>('audit/list', req);
  return { total: data?.total ?? 0, page: data?.page ?? 1, size: data?.size ?? 20, list: data?.list ?? [] };
}

/** GET audit/verify: recompute the hash chain and report the first broken link. */
export async function verifyAuditChain(opts: { archives?: boolean } = {}): Promise<AuditVerifyResult> {
  const url = `/api/v1alpha1/audit/verify${opts.archives ? '?archives=true' : ''}`;
  const get = api.GET as unknown as (url: string) => Promise<{ data?: unknown; error?: unknown }>;
  const { data, error } = await get(url);
  if (error) throw new Error(typeof error === 'string' ? error : JSON.stringify(error));
  const res = data as JavaResponse<AuditVerifyResult> | undefined;
  if (res?.status && res.status.code !== 0) throw new Error(res.status.msg || `API error ${res.status.code}`);
  return res?.data as AuditVerifyResult;
}
