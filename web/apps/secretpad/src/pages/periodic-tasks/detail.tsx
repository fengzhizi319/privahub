/**
 * Periodic task detail (legacy `/periodic-task-detail` layout).
 *
 * - periodic task: `scheduled/info {scheduleId}` → graph/job snapshot
 * - sub task (`?scheduleTaskId=`): `scheduled/task/info {scheduleId, scheduleTaskId}`
 * - run records: `scheduled/job/list {projectId, graphId, scheduleTaskId?, pageNum, pageSize}`
 *
 * The graph snapshot is rendered as a read-only DAG preview (node coordinates +
 * edges, status-highlighted) plus the node status list.
 */
import React, { useState } from 'react';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { useLocation, useNavigate } from '@tanstack/react-router';
import { Badge, Button, Card, Empty, Pagination } from '@secretpad/design-system';
import {
  getScheduledInfoJava,
  getScheduledTaskInfoJava,
  listScheduledJobsJava,
  type ScheduledJobVOJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { jobStatusBadge } from './helpers';
import { NODE_STATUS, normalizeStatus } from '@secretpad/dag-next';
import { DagPreview, type PreviewEdge, type PreviewNode } from '../../features/lineage/dag-preview';
import type { ScheduledGraphNodeVOJava } from '@secretpad/api-client';

const PAGE_SIZE = 10;

const Field: React.FC<{ label: React.ReactNode; children: React.ReactNode }> = ({ label, children }) => (
  <div className="min-w-0">
    <div className="text-gray-400">{label}</div>
    <div className="text-gray-800 dark:text-gray-200 break-all">{children}</div>
  </div>
);

export const PeriodicTaskDetailPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const search = (location.search || {}) as Record<string, unknown>;
  const scheduleId = String(search.scheduleId ?? '');
  const projectId = String(search.projectId ?? '');
  const scheduleTaskId = search.scheduleTaskId ? String(search.scheduleTaskId) : '';
  const searchGraphId = search.graphId ? String(search.graphId) : '';
  const [page, setPage] = useState(1);

  const infoQuery = useQuery<ScheduledJobVOJava>({
    queryKey: ['scheduled-detail-info', scheduleId, scheduleTaskId],
    queryFn: () =>
      scheduleTaskId ? getScheduledTaskInfoJava({ scheduleId, scheduleTaskId }) : getScheduledInfoJava(scheduleId),
    enabled: !!scheduleId,
  });
  const info = infoQuery.data;
  const graphId = info?.graph?.graphId || searchGraphId;

  const jobsQuery = useQuery({
    queryKey: ['scheduled-detail-jobs', projectId, graphId, scheduleTaskId, page],
    queryFn: () =>
      listScheduledJobsJava({
        projectId,
        graphId,
        ...(scheduleTaskId ? { scheduleTaskId } : {}),
        pageNum: page,
        pageSize: PAGE_SIZE,
      }),
    enabled: !!projectId && !!graphId,
    placeholderData: keepPreviousData,
  });
  const jobs = jobsQuery.data?.list ?? [];
  const nodes = info?.graph?.nodes ?? [];
  const preview = toPreview(nodes, info?.graph?.edges);

  return (
    <div className="space-y-6 text-xs">
      <div className="flex items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div className="min-w-0">
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">
            {scheduleTaskId ? t('periodicTasks.subTaskDetailTitle') : t('periodicTasks.detailTitle')}
          </h2>
          <p className="text-gray-500 font-mono truncate">
            {scheduleId}
            {scheduleTaskId ? ` / ${scheduleTaskId}` : ''}
          </p>
        </div>
        <Button
          variant="ghost"
          onClick={() => navigate({ to: '/periodic-tasks', search: { projectId: projectId || undefined } })}
        >
          {t('periodicTasks.back')}
        </Button>
      </div>

      {!scheduleId && <Empty>{t('periodicTasks.missingScheduleId')}</Empty>}
      {infoQuery.error && (
        <div className="text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: infoQuery.error.message })}
        </div>
      )}

      {info && (
        <Card title={t('periodicTasks.snapshot')}>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <Field label={t('periodicTasks.graphName')}>{info.graph?.name || '-'}</Field>
            <Field label={t('periodicTasks.graphId')}>
              <span className="font-mono">{graphId || '-'}</span>
            </Field>
            <Field label={t('periodicTasks.jobId')}>
              <span className="font-mono">{info.jobId || '-'}</span>
            </Field>
            <Field label={t('periodicTasks.status')}>
              {info.status ? <Badge status={jobStatusBadge(info.status)}>{info.status}</Badge> : '-'}
            </Field>
            <Field label={t('periodicTasks.startTime')}>{info.gmtCreate || '-'}</Field>
            <Field label={t('periodicTasks.endTime')}>{info.gmtFinished || '-'}</Field>
          </div>
          {info.errMsg && <div className="mt-3 text-rose-500 break-all">{info.errMsg}</div>}
        </Card>
      )}

      {info && preview.nodes.length > 0 && (
        <Card title={t('periodicTasks.graphName')}>
          <DagPreview nodes={preview.nodes} edges={preview.edges} highlight={preview.done} className="w-full max-h-80" />
        </Card>
      )}

      {info && (
        <Card title={`${t('periodicTasks.nodes')} (${nodes.length})`} bodyClassName="p-0">
          {nodes.length === 0 ? (
            <div className="p-4 text-center text-gray-400">{t('common.empty')}</div>
          ) : (
            <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead className="text-gray-500 border-b border-gray-200 dark:border-gray-800">
                <tr>
                  <th className="p-3">{t('periodicTasks.nodeLabel')}</th>
                  <th className="p-3">{t('periodicTasks.codeName')}</th>
                  <th className="p-3">{t('periodicTasks.parties')}</th>
                  <th className="p-3">{t('periodicTasks.status')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {nodes.map((n) => (
                  <tr key={n.graphNodeId}>
                    <td className="p-3 text-gray-800 dark:text-gray-200">
                      {n.label || n.graphNodeId}
                      <div className="text-gray-400 font-mono">{n.graphNodeId}</div>
                    </td>
                    <td className="p-3 font-mono text-gray-500">{n.codeName || '-'}</td>
                    <td className="p-3 text-gray-500">
                      {(n.parties || []).map((p) => p.nodeName || p.nodeId).join(', ') || '-'}
                    </td>
                    <td className="p-3">
                      {n.status ? <Badge status={jobStatusBadge(n.status)}>{n.status}</Badge> : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          )}
        </Card>
      )}

      <Card title={t('periodicTasks.records')} bodyClassName="p-0">
        {jobsQuery.error && (
          <div className="p-3 text-rose-500">{t('common.error', { message: jobsQuery.error.message })}</div>
        )}
        <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead className="text-gray-500 border-b border-gray-200 dark:border-gray-800">
            <tr>
              <th className="p-3">{t('periodicTasks.jobId')}</th>
              <th className="p-3">{t('periodicTasks.status')}</th>
              <th className="p-3">{t('periodicTasks.progress')}</th>
              <th className="p-3">{t('periodicTasks.outputs')}</th>
              <th className="p-3">{t('periodicTasks.startTime')}</th>
              <th className="p-3">{t('periodicTasks.endTime')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {jobsQuery.isLoading && (
              <tr>
                <td colSpan={6} className="p-3 text-center text-gray-400">
                  {t('common.loading')}
                </td>
              </tr>
            )}
            {!jobsQuery.isLoading && jobs.length === 0 && (
              <tr>
                <td colSpan={6} className="p-3 text-center text-gray-400">
                  {t('periodicTasks.noRecords')}
                </td>
              </tr>
            )}
            {jobs.map((job) => (
              <tr key={job.jobId}>
                <td className="p-3 font-mono text-gray-800 dark:text-gray-200">
                  {job.jobId}
                  {job.errMsg && <div className="text-rose-500 font-sans truncate max-w-xs">{job.errMsg}</div>}
                </td>
                <td className="p-3">
                  <Badge status={jobStatusBadge(job.status)}>{job.status || '-'}</Badge>
                </td>
                <td className="p-3 text-gray-500">
                  {job.finishedTaskCount ?? 0}/{job.taskCount ?? 0}
                </td>
                <td className="p-3 text-gray-500">
                  {t('periodicTasks.outputCounts', {
                    table: job.tableCount ?? 0,
                    model: job.modelCount ?? 0,
                    report: job.reportCount ?? 0,
                  })}
                </td>
                <td className="p-3 text-gray-500">{job.gmtCreate || '-'}</td>
                <td className="p-3 text-gray-500">{job.gmtFinished || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
        </div>
        {(jobsQuery.data?.total ?? 0) > PAGE_SIZE && (
          <div className="p-3 border-t border-gray-100 dark:border-gray-800">
            <Pagination page={page} pageSize={PAGE_SIZE} total={jobsQuery.data?.total ?? 0} onChange={setPage} />
          </div>
        )}
      </Card>
    </div>
  );
};

/** 快照节点 / 边 → DagPreview（无坐标时按序网格排布；边优先取 graph.edges，否则由 outputs→inputs 推导）。 */
function toPreview(nodes: ScheduledGraphNodeVOJava[], rawEdges: unknown): { nodes: PreviewNode[]; edges: PreviewEdge[]; done: Set<string> } {
  const pn: PreviewNode[] = nodes.map((n, i) => ({
    id: n.graphNodeId || String(i),
    name: n.label || n.codeName || n.graphNodeId || '',
    x: typeof n.x === 'number' ? n.x : (i % 4) * 180,
    y: typeof n.y === 'number' ? n.y : Math.floor(i / 4) * 100,
  }));
  let edges: PreviewEdge[] = [];
  if (Array.isArray(rawEdges)) {
    edges = (rawEdges as Array<{ edgeId?: string; source?: string; target?: string }>)
      .filter((e) => e.source && e.target)
      .map((e, i) => ({ id: e.edgeId || `e${i}`, source: e.source!, target: e.target! }));
  } else {
    const owner = new Map<string, string>();
    nodes.forEach((n) => (n.outputs || []).forEach((o) => owner.set(o, n.graphNodeId || '')));
    nodes.forEach((n) =>
      (n.inputs || []).forEach((inp, i) => {
        const src = owner.get(inp);
        if (src) edges.push({ id: `${src}-${n.graphNodeId}-${i}`, source: src, target: n.graphNodeId || '' });
      }),
    );
  }
  const done = new Set(nodes.filter((n) => normalizeStatus(n.status) === NODE_STATUS.SUCCEED).map((n) => n.graphNodeId || ''));
  return { nodes: pn, edges, done };
}
