/**
 * 项目卡片上的「训练流 / 任务」计数悬浮层（旧版 project-list/components/popover）：
 * - 训练流：graph/list 名称列表；
 * - 任务：最新 10 条运行任务（project/job/list pageSize=10），状态点 + 创建时间 + Job ID。
 * 悬停 / 聚焦时才请求，关闭后不保留（与旧版 onOpenChange 一致）。
 */
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient, listGraphJobsJava } from '@secretpad/api-client';
import { statusBadge } from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';

const DOT: Record<string, string> = {
  success: 'bg-emerald-500',
  processing: 'bg-blue-500 animate-pulse',
  error: 'bg-red-500',
  warning: 'bg-amber-500',
  default: 'bg-gray-400',
};

const Hover: React.FC<{ label: React.ReactNode; title: string; children: (open: boolean) => React.ReactNode }> = ({ label, title, children }) => {
  // 卡片带 overflow-hidden：浮层用 fixed 定位到计数按钮下方，避免被裁剪。
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  const show = (el: HTMLElement) => {
    const r = el.getBoundingClientRect();
    setPos({ top: r.bottom + 4, left: Math.min(r.left, window.innerWidth - 272) });
  };
  return (
    <span className="inline-block" onMouseEnter={(e) => show(e.currentTarget)} onMouseLeave={() => setPos(null)} onFocus={(e) => show(e.currentTarget)} onBlur={() => setPos(null)}>
      <button type="button" className="font-bold text-gray-800 dark:text-gray-200 underline decoration-dotted underline-offset-2" onClick={(e) => e.stopPropagation()}>
        {label}
      </button>
      {pos && (
        <div
          role="tooltip"
          style={{ top: pos.top, left: pos.left }}
          className="fixed z-50 w-64 max-h-72 overflow-y-auto rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-2 shadow-xl text-[11px]"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="font-semibold text-gray-700 dark:text-gray-200 mb-1">{title}</div>
          {children(true)}
        </div>
      )}
    </span>
  );
};

export const GraphCountPopover: React.FC<{ projectId: string; count: React.ReactNode }> = ({ projectId, count }) => {
  const { t } = useTranslation();
  return (
    <Hover label={count} title={t('projects.graphCount')}>
      {(open) => <GraphList projectId={projectId} enabled={open} empty={t('common.empty')} />}
    </Hover>
  );
};

const GraphList: React.FC<{ projectId: string; enabled: boolean; empty: string }> = ({ projectId, enabled, empty }) => {
  const q = useQuery({ queryKey: ['project-card-graphs', projectId], queryFn: () => apiClient.getGraphs(projectId), enabled, staleTime: 10_000 });
  if (q.isLoading) return <div className="text-gray-400">…</div>;
  const list = q.data ?? [];
  if (!list.length) return <div className="text-gray-400">{empty}</div>;
  return (
    <ul className="space-y-0.5">
      {list.map((g) => (
        <li key={g.graphId} className="truncate text-gray-600 dark:text-gray-300" title={g.name}>
          {g.name || g.graphId}
        </li>
      ))}
    </ul>
  );
};

export const JobCountPopover: React.FC<{ projectId: string; count: React.ReactNode }> = ({ projectId, count }) => {
  const { t } = useTranslation();
  return (
    <Hover label={count} title={t('projects.latestJobs')}>
      {(open) => <JobList projectId={projectId} enabled={open} empty={t('common.empty')} />}
    </Hover>
  );
};

const JobList: React.FC<{ projectId: string; enabled: boolean; empty: string }> = ({ projectId, enabled, empty }) => {
  const q = useQuery({ queryKey: ['project-card-jobs', projectId], queryFn: () => listGraphJobsJava({ projectId, pageNum: 1, pageSize: 10 }), enabled, staleTime: 10_000 });
  if (q.isLoading) return <div className="text-gray-400">…</div>;
  const list = q.data?.list ?? [];
  if (!list.length) return <div className="text-gray-400">{empty}</div>;
  return (
    <ul className="space-y-1">
      {list.map((j) => (
        <li key={j.jobId} className="flex items-center gap-1.5 text-gray-600 dark:text-gray-300">
          <span className={`w-2 h-2 rounded-full flex-shrink-0 ${DOT[statusBadge(j.status)]}`} title={j.status} />
          <span className="font-mono">{j.gmtCreate || '-'}</span>
          <span className="truncate text-gray-400" title={j.jobId}>
            ID: {j.jobId}
          </span>
        </li>
      ))}
    </ul>
  );
};
