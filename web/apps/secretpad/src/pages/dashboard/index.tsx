import React from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueries } from '@tanstack/react-query';
import { Card, Badge, Button } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { useState } from 'react';

/** localStorage 中引导 banner 被关闭的标记键名。 */
const GUIDE_BANNER_DISMISSED_KEY = 'secretpad-guide-banner-dismissed';
/** 与引导页共用的进度键名（读取已完成步骤数，全部完成则不再展示 banner）。 */
const GUIDE_PROGRESS_KEY = 'secretpad-guide-progress';

/**
 * 判断是否需要展示新手引导 banner。
 *
 * 规则：用户手动关闭过则不再弹出；引导步骤已全部完成也不再提示。
 * 该函数在组件初始化时调用一次（useState 惰性初始值），避免每次渲染重复读 localStorage。
 */
function shouldShowGuideBanner(): boolean {
  try {
    if (localStorage.getItem(GUIDE_BANNER_DISMISSED_KEY) === '1') return false;
    const raw = localStorage.getItem(GUIDE_PROGRESS_KEY);
    if (raw) {
      const arr = JSON.parse(raw);
      // 引导共 5 步（node/data/project/dag/result），全部完成则隐藏 banner。
      if (Array.isArray(arr) && arr.length >= 5) return false;
    }
    return true;
  } catch {
    return false;
  }
}

export const DashboardPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  // 新手引导 banner 是否可见（初始值惰性计算，仅读一次 localStorage）。
  const [showGuideBanner, setShowGuideBanner] = useState<boolean>(() => shouldShowGuideBanner());

  /** 关闭引导 banner 并持久化，之后不再自动弹出。 */
  const dismissGuideBanner = () => {
    try {
      localStorage.setItem(GUIDE_BANNER_DISMISSED_KEY, '1');
    } catch {
      // 存储失败时仅在当前会话内隐藏。
    }
    setShowGuideBanner(false);
  };

  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });
  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const jobsQuery = useQuery({
    queryKey: ['recent-jobs'],
    queryFn: () => apiClient.getJobs(),
  });

  const nodes = nodesQuery.data ?? [];
  const projects = projectsQuery.data ?? [];
  const jobs = jobsQuery.data ?? [];

  /**
   * 数据资产统计：并行查询每个节点下的数据表数量。
   *
   * 设计要点：
   * 1. 后端 `/api/v1alpha1/datatable/list` 是按节点(ownerId)分页查询的，
   *    没有一次性返回全平台所有数据表的接口，因此需要对每个节点发一次请求。
   * 2. 使用 `useQueries` 而非循环调用 `useQuery`：React Query 在 render 阶段
   *    必须保持 hooks 顺序稳定，`useQueries` 内部会根据 `nodes` 数组动态生成
   *    一组等长的查询，保证顺序与节点顺序一致，避免违反 Hooks 规则。
   * 3. `enabled: nodes.length > 0` 防止在节点列表尚未加载时发送无意义的请求；
   *    同时当节点数组变化时，React Query 会自动为新增/减少的节点增删查询。
   * 4. 汇总方式：每个查询返回该节点的数据表数组，取所有查询结果长度之和作为
   *    总数据资产数；任一查询失败都会被收集到 `error` 中统一展示。
   */
  const tablesQueries = useQueries({
    queries: nodes.map((node) => ({
      queryKey: ['datatables', node.nodeId],
      queryFn: () => apiClient.getDataTables(node.nodeId),
      enabled: nodes.length > 0,
    })),
  });
  const totalTables = tablesQueries.reduce((sum, q) => sum + (q.data?.length ?? 0), 0);

  const error =
    nodesQuery.error?.message ||
    projectsQuery.error?.message ||
    jobsQuery.error?.message ||
    tablesQueries.find((q) => q.error)?.error?.message ||
    null;

  const readyNodes = nodes.filter((n) => n.nodeStatus === 'Ready' || n.status === 'Ready').length;

  return (
    <div className="space-y-6">
      {/* 新手引导 banner：首次使用或未完成任务时提示，点击跳转引导页。 */}
      {showGuideBanner && (
        <div className="flex items-center justify-between gap-4 px-4 py-3 rounded-xl border border-blue-200 dark:border-blue-900 bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-950/40 dark:to-purple-950/40">
          <button
            onClick={() => navigate({ to: '/guide' })}
            className="flex items-center gap-2 text-xs font-medium text-blue-700 dark:text-blue-300 hover:underline underline-offset-2 text-left"
          >
            <span className="text-base">🚀</span>
            {t('guide.dashboardBanner')}
          </button>
          <button
            onClick={dismissGuideBanner}
            aria-label="dismiss"
            className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-sm flex-shrink-0"
          >
            ✕
          </button>
        </div>
      )}

      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {/* Stat Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
        <Card className="hover:border-blue-500/50 transition-all cursor-pointer" bodyClassName="p-4">
          <div className="flex items-center justify-between text-xs text-gray-500 font-medium">
            <span>{t('dashboard.projects')}</span>
            <span className="p-1.5 rounded-lg bg-blue-50 dark:bg-blue-950 text-blue-600">📁</span>
          </div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{projects.length}</div>
          <div className="mt-1 text-[11px] text-emerald-600 flex items-center gap-1 font-medium">
            <span>↑ {t('dashboard.activeMode')}</span>
          </div>
        </Card>

        <Card className="hover:border-blue-500/50 transition-all cursor-pointer" bodyClassName="p-4">
          <div className="flex items-center justify-between text-xs text-gray-500 font-medium">
            <span>{t('dashboard.nodes')}</span>
            <span className="p-1.5 rounded-lg bg-purple-50 dark:bg-purple-950 text-purple-600">🖥️</span>
          </div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{nodes.length}</div>
          <div className="mt-1 text-[11px] text-gray-500">
            {t('dashboard.ready', { ready: readyNodes, total: nodes.length })}
          </div>
        </Card>

        <Card className="hover:border-blue-500/50 transition-all cursor-pointer" bodyClassName="p-4">
          <div className="flex items-center justify-between text-xs text-gray-500 font-medium">
            <span>{t('dashboard.dataTables')}</span>
            <span className="p-1.5 rounded-lg bg-indigo-50 dark:bg-indigo-950 text-indigo-600">🗄️</span>
          </div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{totalTables}</div>
          <div className="mt-1 text-[11px] text-gray-500">
            {nodes.length > 0 ? t('dashboard.avgTables', { avg: (totalTables / nodes.length).toFixed(1) }) : t('dashboard.noNodes')}
          </div>
        </Card>

        <Card className="hover:border-blue-500/50 transition-all cursor-pointer" bodyClassName="p-4">
          <div className="flex items-center justify-between text-xs text-gray-500 font-medium">
            <span>{t('dashboard.jobs')}</span>
            <span className="p-1.5 rounded-lg bg-emerald-50 dark:bg-emerald-950 text-emerald-600">⚡</span>
          </div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{jobs.length}</div>
          <div className="mt-1 text-[11px] text-emerald-600 font-medium">
            {jobs.length > 0 ? `${Math.round((jobs.filter((j) => j.status === 'SUCCEEDED').length / jobs.length) * 100)}% ${t('dashboard.successRate')}` : t('dashboard.noJobs')}
          </div>
        </Card>

        <Card className="hover:border-blue-500/50 transition-all cursor-pointer" bodyClassName="p-4">
          <div className="flex items-center justify-between text-xs text-gray-500 font-medium">
            <span>{t('dashboard.clusterLoad')}</span>
            <span className="p-1.5 rounded-lg bg-amber-50 dark:bg-amber-950 text-amber-600">🛡️</span>
          </div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{nodes.length > 0 ? (readyNodes / nodes.length * 100).toFixed(1) : '0.0'}%</div>
          <div className="mt-1 text-[11px] text-gray-500">
            {t('dashboard.ready', { ready: readyNodes, total: nodes.length })}
          </div>
        </Card>
      </div>

      {/* Main Content Layout */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Recent Jobs & Project List */}
        <div className="lg:col-span-2 space-y-6">
          <Card
            title={t('dashboard.recentJobs')}
            extra={<Button size="sm" variant="link" onClick={() => navigate({ to: '/dag' })}>{t('dashboard.launchDag')}</Button>}
          >
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {jobs.length === 0 && (
                <div className="py-4 text-xs text-gray-400 text-center">{t('dashboard.noJobs')}</div>
              )}
              {jobs.map((job) => (
                <div key={job.jobId} className="py-3 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`w-8 h-8 rounded-lg flex items-center justify-center text-xs ${
                      job.status === 'RUNNING' ? 'bg-blue-50 text-blue-600 dark:bg-blue-950/50 animate-spin' :
                      job.status === 'SUCCEEDED' ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/50' : 'bg-gray-100'
                    }`}>
                      {job.status === 'RUNNING' ? '🔄' : '✓'}
                    </div>
                    <div>
                      <div className="font-semibold text-sm text-gray-800 dark:text-gray-200">{job.name}</div>
                      <div className="text-xs text-gray-400 font-mono">Job ID: {job.jobId} • {t('common.duration') || 'Duration'}: {job.duration || '-'}</div>
                    </div>
                  </div>
                  <Badge status={job.status === 'RUNNING' ? 'processing' : job.status === 'FAILED' ? 'error' : 'success'}>
                    {job.status}
                  </Badge>
                </div>
              ))}
            </div>
          </Card>

          <Card
            title={t('dashboard.activeProjects')}
            extra={<Button size="sm" variant="link" onClick={() => navigate({ to: '/projects' })}>{t('dashboard.viewAllProjects')}</Button>}
          >
            <div className="space-y-3">
              {projects.length === 0 && (
                <div className="text-xs text-gray-400 text-center py-4">{t('projects.noProjects')}</div>
              )}
              {projects.map((proj) => (
                <div
                  key={proj.projectId}
                  onClick={() => navigate({ to: '/projects' })}
                  className="p-3.5 rounded-xl border border-gray-100 dark:border-gray-800 hover:border-blue-500/40 bg-gray-50/50 dark:bg-gray-850/50 cursor-pointer transition-all flex items-center justify-between"
                >
                  <div>
                    <div className="font-semibold text-sm text-gray-900 dark:text-gray-100 flex items-center gap-2">
                      {proj.projectName}
                      <span className="px-2 py-0.5 rounded text-[10px] bg-blue-100 dark:bg-blue-900/60 text-blue-700 dark:text-blue-300 font-mono">
                        {proj.computeMode} Mode
                      </span>
                    </div>
                    <div className="text-xs text-gray-500 mt-1 line-clamp-1">{proj.description}</div>
                  </div>
                  <div className="text-right text-xs text-gray-400">
                    {/* 防御性取值：后端可能不返回 nodes 字段，避免读取 undefined.length 导致整页崩溃。 */}
                    <div>{proj.nodes?.length ?? 0} {t('projects.joinedNodes')}</div>
                    <div className="text-[11px] text-blue-600 font-medium mt-0.5">{proj.jobCount ?? 0} Jobs Ran</div>
                  </div>
                </div>
              ))}
            </div>
          </Card>
        </div>

        {/* Right Column: Node Topology & Quick Actions */}
        <div className="space-y-6">
          <Card title={t('dashboard.nodeTopology')}>
            <div className="space-y-3">
              {nodes.length === 0 && (
                <div className="text-xs text-gray-400 text-center py-2">{t('nodes.noNodes')}</div>
              )}
              {nodes.map((node) => (
                <div key={node.nodeId} className="p-3 rounded-lg border border-gray-100 dark:border-gray-800 flex items-center justify-between">
                  <div className="flex items-center gap-2.5">
                    <div className={`w-2.5 h-2.5 rounded-full ${node.nodeStatus === 'Ready' ? 'bg-emerald-500' : 'bg-gray-400'}`} />
                    <div>
                      <div className="font-semibold text-xs text-gray-800 dark:text-gray-200">{node.nodeName}</div>
                      <div className="text-[10px] text-gray-400 font-mono">{node.netAddress || node.ip || '-'}</div>
                    </div>
                  </div>
                  <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300">
                    {node.type}
                  </span>
                </div>
              ))}
            </div>
          </Card>

          <Card title={t('dashboard.quickActions')}>
            <div className="grid grid-cols-2 gap-2.5">
              <Button onClick={() => navigate({ to: '/dag' })} variant="outline" className="justify-start text-xs p-3">
                ⚡ {t('dashboard.createDag')}
              </Button>
              <Button onClick={() => navigate({ to: '/projects' })} variant="outline" className="justify-start text-xs p-3">
                ➕ {t('dashboard.newProject')}
              </Button>
              <Button onClick={() => navigate({ to: '/data-tables' })} variant="outline" className="justify-start text-xs p-3">
                🗄️ {t('dashboard.importTable')}
              </Button>
              <Button onClick={() => navigate({ to: '/nodes' })} variant="outline" className="justify-start text-xs p-3">
                🖥️ {t('dashboard.registerNode')}
              </Button>
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
};
