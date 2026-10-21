/**
 * 审计日志页面（Audit Log）。
 *
 * 对应后端只读接口 `/api/v1alpha1/audit/list` 与 `/api/v1alpha1/audit/verify`
 * （仅 ADMIN / AUDITOR 可访问，服务端强制校验）：
 * 1. 按时间范围、操作者、操作、结果、资源过滤并分页查看防篡改审计记录；
 * 2. “校验哈希链”重算整条 SM3 哈希链（含归档段边界与外部锚点），报告首个断点。
 * 页面不提供任何修改 / 删除能力。
 */
import React, { useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Badge, Button, Card, Empty, Input, Pagination, Select } from '@secretpad/design-system';
import { listAuditLogs, verifyAuditChain, type AuditRecord, type AuditVerifyResult } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import {
  EMPTY_FILTERS,
  PAGE_SIZE,
  isPermissionError,
  resultBadge,
  shortHash,
  toAuditRequest,
  type AuditFilters,
} from './model';

export const AuditLogPage: React.FC = () => {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<AuditFilters>(EMPTY_FILTERS);
  const [filters, setFilters] = useState<AuditFilters>(EMPTY_FILTERS);
  const [page, setPage] = useState(1);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [withArchives, setWithArchives] = useState(false);

  const listQuery = useQuery({
    queryKey: ['audit-list', filters, page],
    queryFn: () => listAuditLogs(toAuditRequest(filters, page)),
    retry: false,
  });

  const verify = useMutation<AuditVerifyResult, Error>({
    mutationFn: () => verifyAuditChain({ archives: withArchives }),
  });

  const set = (k: keyof AuditFilters) => (v: string) => setDraft((d) => ({ ...d, [k]: v }));
  const onInput = (k: keyof AuditFilters) => (e: React.ChangeEvent<HTMLInputElement>) => set(k)(e.target.value);

  const search = () => {
    setFilters(draft);
    setPage(1);
  };
  const reset = () => {
    setDraft(EMPTY_FILTERS);
    setFilters(EMPTY_FILTERS);
    setPage(1);
  };

  if (listQuery.error && isPermissionError(listQuery.error)) {
    return (
      <Card title={t('audit.title')}>
        <Empty>{t('audit.noPermission')}</Empty>
      </Card>
    );
  }

  const data = listQuery.data;
  const rows: AuditRecord[] = data?.list ?? [];
  const vr = verify.data;

  const resultOptions = [
    { value: '', label: t('audit.all') },
    { value: 'success', label: t('audit.success') },
    { value: 'failure', label: t('audit.failure') },
    { value: 'denied', label: t('audit.denied') },
  ];

  return (
    <div className="space-y-4">
      <Card
        title={t('audit.title')}
        extra={
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-1 text-xs text-gray-500">
              <input
                type="checkbox"
                checked={withArchives}
                onChange={(e) => setWithArchives(e.target.checked)}
              />
              {t('audit.verifyArchives')}
            </label>
            <Button variant="primary" size="sm" loading={verify.isPending} onClick={() => verify.mutate()}>
              {verify.isPending ? t('audit.verifying') : t('audit.verify')}
            </Button>
          </div>
        }
      >
        <p className="text-xs text-gray-500 mb-3">{t('audit.subtitle')}</p>
        {verify.error && (
          <div className="mb-3 rounded-md border border-red-300 bg-red-50 dark:bg-red-950/30 p-3 text-sm text-red-700">
            {isPermissionError(verify.error) ? t('audit.noPermission') : verify.error.message}
          </div>
        )}
        {vr && (
          <div
            data-testid="audit-verify-result"
            className={`mb-3 rounded-md border p-3 text-sm ${
              vr.valid
                ? 'border-emerald-300 bg-emerald-50 dark:bg-emerald-950/30 text-emerald-800 dark:text-emerald-300'
                : 'border-red-300 bg-red-50 dark:bg-red-950/30 text-red-700 dark:text-red-300'
            }`}
          >
            <div className="font-semibold">{vr.valid ? t('audit.valid') : t('audit.broken')}</div>
            <div className="text-xs mt-1">
              {t('audit.verifySummary', {
                checked: vr.checked,
                tailSeq: vr.tailSeq,
                segments: vr.segments,
                anchors: vr.anchorsChecked,
              })}
            </div>
            {!vr.valid && (
              <div className="text-xs mt-1">
                {t('audit.brokenAt', { seq: vr.brokenSeq ?? '-', kind: vr.breakKind ?? '' })} {vr.reason}
              </div>
            )}
            {vr.anchorWarning && <div className="text-xs mt-1 text-amber-700">⚠ {vr.anchorWarning}</div>}
            <div className="text-[11px] mt-1 font-mono break-all opacity-70">
              {vr.algorithm} · {vr.tailHash}
            </div>
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.startTime')}</span>
            <Input type="datetime-local" value={draft.start} onChange={onInput('start')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.endTime')}</span>
            <Input type="datetime-local" value={draft.end} onChange={onInput('end')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.actor')}</span>
            <Input value={draft.actor} onChange={onInput('actor')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.result')}</span>
            <Select options={resultOptions} value={draft.result} onChange={set('result')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.action')}</span>
            <Input value={draft.action} onChange={onInput('action')} placeholder="auth.login / POST /api/..." />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.resourceType')}</span>
            <Input value={draft.resourceType} onChange={onInput('resourceType')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.resourceId')}</span>
            <Input value={draft.resourceId} onChange={onInput('resourceId')} />
          </label>
          <label className="text-xs text-gray-500 space-y-1">
            <span>{t('audit.ip')}</span>
            <Input value={draft.ip} onChange={onInput('ip')} />
          </label>
        </div>
        <div className="flex gap-2 mt-3">
          <Button variant="primary" size="sm" onClick={search}>
            {t('audit.search')}
          </Button>
          <Button size="sm" onClick={reset}>
            {t('audit.reset')}
          </Button>
        </div>
      </Card>

      <Card title={data ? t('audit.total', { total: data.total }) : undefined}>
        {listQuery.error ? (
          <Empty>
            {t('audit.loadFailed')}: {(listQuery.error as Error).message}
          </Empty>
        ) : rows.length === 0 && !listQuery.isLoading ? (
          <Empty>{t('audit.empty')}</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-xs">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-800">
                  <th className="py-2 pr-3">#</th>
                  <th className="py-2 pr-3">{t('audit.time')}</th>
                  <th className="py-2 pr-3">{t('audit.actor')}</th>
                  <th className="py-2 pr-3">{t('audit.ip')}</th>
                  <th className="py-2 pr-3">{t('audit.action')}</th>
                  <th className="py-2 pr-3">{t('audit.resourceType')}</th>
                  <th className="py-2 pr-3">{t('audit.result')}</th>
                  <th className="py-2 pr-3">{t('audit.hash')}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <React.Fragment key={r.seq}>
                    <tr
                      className="border-b border-gray-100 dark:border-gray-900 hover:bg-gray-50 dark:hover:bg-gray-900 cursor-pointer"
                      onClick={() => setExpanded(expanded === r.seq ? null : r.seq)}
                    >
                      <td className="py-2 pr-3 font-mono">{r.seq}</td>
                      <td className="py-2 pr-3 whitespace-nowrap">{new Date(r.time).toLocaleString()}</td>
                      <td className="py-2 pr-3">
                        {r.actor || '-'} <span className="text-gray-400">({r.actorType})</span>
                      </td>
                      <td className="py-2 pr-3 font-mono">{r.ip}</td>
                      <td className="py-2 pr-3 font-mono">{r.action}</td>
                      <td className="py-2 pr-3">
                        {r.resourceType}
                        {r.resourceId ? `:${r.resourceId}` : ''}
                      </td>
                      <td className="py-2 pr-3">
                        <Badge status={resultBadge(r.result)}>{t(`audit.${r.result}`)}</Badge>
                      </td>
                      <td className="py-2 pr-3 font-mono text-gray-400">{shortHash(r.hash)}</td>
                    </tr>
                    {expanded === r.seq && (
                      <tr className="bg-gray-50 dark:bg-gray-900/50">
                        <td colSpan={8} className="p-3 font-mono text-[11px] break-all space-y-1">
                          {r.reason && (
                            <div>
                              {t('audit.reason')}: {r.reason}
                            </div>
                          )}
                          <div>eventId: {r.eventId}</div>
                          <div>traceId: {r.traceId || '-'}</div>
                          <div>ownerId: {r.ownerId || '-'}</div>
                          {r.extra && <div>extra: {JSON.stringify(r.extra)}</div>}
                          <div>prevHash: {r.prevHash}</div>
                          <div>hash: {r.hash}</div>
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {data && data.total > PAGE_SIZE && (
          <Pagination page={page} pageSize={PAGE_SIZE} total={data.total} onChange={setPage} className="mt-3" />
        )}
      </Card>
    </div>
  );
};
