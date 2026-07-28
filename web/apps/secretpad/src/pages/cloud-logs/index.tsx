/**
 * 云端日志 SLS 页面（Cloud Logs）。
 *
 * 对应旧前端“云端日志”模块与后端 `/api/v1alpha1/cloud_log/sls` 接口：
 * 从阿里云 SLS（Simple Log Service）检索跨节点任务的运行日志。
 *
 * 设计要点：
 * 1. 用户选择项目（必填），并可选填 Job ID / Task ID / 图节点 ID / 节点进行过滤；
 * 2. 勾选“同时查询各参与方”后，后端会聚合所有参与方的日志（queryParties=true）；
 * 3. 返回结果包含任务状态（status）、参与节点列表（nodeParties）与日志行（logs）；
 * 4. 日志展示复用 `@secretpad/dag-next` 的 `LogViewer`（Monaco 风格查看器），
 *    天然具备行号、级别高亮、搜索、筛选、复制等能力，避免重复造轮子。
 */
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Button, Badge } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { LogViewer } from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';

/** 任务状态到 Badge 状态的映射。 */
function statusBadge(status?: string): 'success' | 'processing' | 'error' | 'default' {
  switch ((status || '').toUpperCase()) {
    case 'SUCCEED':
      return 'success';
    case 'RUNNING':
    case 'INITIALIZED':
    case 'STAGING':
      return 'processing';
    case 'FAILED':
      return 'error';
    default:
      return 'default';
  }
}

export const CloudLogsPage: React.FC = () => {
  const { t } = useTranslation();

  // 查询条件表单状态。
  const [projectId, setProjectId] = useState('');
  const [jobId, setJobId] = useState('');
  const [taskId, setTaskId] = useState('');
  const [graphNodeId, setGraphNodeId] = useState('');
  const [nodeId, setNodeId] = useState('');
  const [queryParties, setQueryParties] = useState(true);

  // 是否已触发过查询（用于区分“未查询”与“查询结果为空”两种空态）。
  const [hasFetched, setHasFetched] = useState(false);

  // 项目列表：用于项目下拉选择。
  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const projects = projectsQuery.data ?? [];

  /**
   * 云端日志查询。
   *
   * 仅在用户点击“拉取日志”后启用（enabled: hasFetched && !!projectId）。
   * 空字符串的过滤条件不传给后端（传 undefined），避免后端按空串做精确匹配。
   */
  const logsQuery = useQuery({
    queryKey: ['cloud-logs', projectId, jobId, taskId, graphNodeId, nodeId, queryParties],
    queryFn: () =>
      apiClient.getCloudLogs({
        projectId,
        jobId: jobId.trim() || undefined,
        taskId: taskId.trim() || undefined,
        graphNodeId: graphNodeId.trim() || undefined,
        nodeId: nodeId.trim() || undefined,
        queryParties,
      }),
    enabled: hasFetched && !!projectId,
  });

  const result = logsQuery.data;
  const logs = result?.logs ?? [];
  const nodeParties = result?.nodeParties ?? [];

  /** 点击“拉取日志”：标记已查询，触发 useQuery 重新拉取。 */
  const handleFetch = () => {
    if (!projectId) return;
    setHasFetched(true);
    // 手动刷新（条件未变时 invalidate 不会自动重查，这里直接 refetch）。
    logsQuery.refetch();
  };

  const loading = logsQuery.isLoading;
  const error = logsQuery.error?.message || null;

  return (
    <div className="space-y-6">
      {/* 页头 */}
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('cloudLogs.title')}</h2>
          <p className="text-xs text-gray-500">{t('cloudLogs.subtitle')}</p>
        </div>
      </div>

      {/* 查询条件表单 */}
      <Card>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 text-xs">
          {/* 项目选择（必填） */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
              {t('cloudLogs.project')} <span className="text-red-500">*</span>
            </label>
            <select
              value={projectId}
              onChange={(e) => setProjectId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              <option value="">{t('cloudLogs.selectProject')}</option>
              {projects.map((p) => (
                <option key={p.projectId} value={p.projectId}>
                  {p.projectName}
                </option>
              ))}
            </select>
          </div>

          {/* Job ID（可选） */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('cloudLogs.jobId')}</label>
            <input
              type="text"
              value={jobId}
              onChange={(e) => setJobId(e.target.value)}
              placeholder="job-xxxx"
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* Task ID（可选） */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('cloudLogs.taskId')}</label>
            <input
              type="text"
              value={taskId}
              onChange={(e) => setTaskId(e.target.value)}
              placeholder="task-xxxx"
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 图节点 ID（可选） */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('cloudLogs.graphNodeId')}</label>
            <input
              type="text"
              value={graphNodeId}
              onChange={(e) => setGraphNodeId(e.target.value)}
              placeholder="node-xxxx"
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 节点（可选） */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('cloudLogs.nodeId')}</label>
            <input
              type="text"
              value={nodeId}
              onChange={(e) => setNodeId(e.target.value)}
              placeholder="alice / bob"
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 同时查询各参与方 */}
          <div className="flex items-end">
            <label className="flex items-center gap-2 cursor-pointer select-none p-2.5">
              <input
                type="checkbox"
                checked={queryParties}
                onChange={(e) => setQueryParties(e.target.checked)}
                className="w-4 h-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="font-semibold text-gray-700 dark:text-gray-300">{t('cloudLogs.queryParties')}</span>
            </label>
          </div>
        </div>

        {/* 拉取按钮 */}
        <div className="mt-4 flex items-center gap-3">
          <Button variant="primary" onClick={handleFetch} disabled={!projectId} loading={loading}>
            {loading ? t('cloudLogs.fetching') : t('cloudLogs.fetch')}
          </Button>
          {!projectId && <span className="text-[11px] text-gray-400">{t('cloudLogs.selectProject')}</span>}
        </div>
      </Card>

      {/* 错误提示 */}
      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {/* 查询结果：状态 + 参与节点 + 日志查看器 */}
      {hasFetched && !error && (
        <div className="space-y-4">
          {/* 任务状态与参与节点概要 */}
          <Card>
            <div className="flex flex-wrap items-center gap-4 text-xs">
              <div className="flex items-center gap-2">
                <span className="text-gray-500">{t('cloudLogs.status')}:</span>
                <Badge status={statusBadge(result?.status)}>{result?.status || '-'}</Badge>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-gray-500">{t('cloudLogs.parties')}:</span>
                {nodeParties.length === 0 ? (
                  <span className="text-gray-400">{t('cloudLogs.noParties')}</span>
                ) : (
                  <div className="flex flex-wrap gap-1.5">
                    {nodeParties.map((p) => (
                      <span
                        key={p.nodeId}
                        className="px-2 py-0.5 rounded bg-blue-50 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300 font-mono text-[10px]"
                      >
                        {p.nodeName || p.nodeId}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <span className="ml-auto text-gray-400">{t('cloudLogs.logCount', { count: logs.length })}</span>
            </div>
          </Card>

          {/* 日志查看器（复用 dag-next 的 Monaco 风格 LogViewer） */}
          <Card bodyClassName="p-4">
            <div className="h-[480px]">
              <LogViewer
                logs={logs}
                loading={loading}
                emptyText={logs.length === 0 && !loading ? t('cloudLogs.emptyHint') : undefined}
              />
            </div>
          </Card>
        </div>
      )}

      {/* 未查询时的引导空态 */}
      {!hasFetched && (
        <div className="text-center text-xs text-gray-400 py-16">{t('cloudLogs.emptyHint')}</div>
      )}
    </div>
  );
};
