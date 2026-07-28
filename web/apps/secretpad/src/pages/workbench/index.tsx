/**
 * EDGE / P2P 工作台聚合页（Workbench）。
 *
 * 对应旧前端 `pages/edge.tsx` + `modules/p2p-workbench/workbench.view.tsx`：
 * 旧版工作台由“欢迎头部（可展开引导流程图）+ 申请事项（消息中心）+ 我的项目”
 * 三大块组成。新前端将其迁移为一个聚合页，组合现有数据与页面入口：
 *
 * - 欢迎头部：保留“🍵 欢迎语 + 标语 + 展开/收起”，展开后呈现
 *   “一张图看懂概念关系与任务流程”的六步引导流水线（注册节点 → 导入数据 →
 *   创建项目 → 编排 DAG → 运行任务 → 查看结果）；
 * - 统计行：聚合节点 / 数据表 / 项目 / 待处理申请四项核心指标；
 * - 申请事项：复用消息接口，展示最近的待处理申请并提供跳转；
 * - 我的项目：P2P 模式展示 P2P 项目，CENTER/EDGE 模式展示协作项目；
 * - 快捷入口：提供 DAG 画布、导入数据、注册节点、隐私场景四个常用入口。
 *
 * 该页面本身不新增后端接口，全部复用既有 apiClient 方法，属于“组合现有页面”
 * 的聚合视图，降低用户在各功能页之间来回切换的成本。
 */
import React, { useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useQueries } from '@tanstack/react-query';
import { Card, Badge, Button } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { useAuthStore } from '../../features/auth/model/auth-store';

/** 引导流水线的六个步骤（图标 + i18n 键 + 跳转目标）。 */
const PIPELINE_STEPS: Array<{ icon: string; labelKey: string; target: string }> = [
  { icon: '🖥️', labelKey: 'workbench.flowNode', target: '/nodes' },
  { icon: '🗄️', labelKey: 'workbench.flowData', target: '/data-tables' },
  { icon: '📁', labelKey: 'workbench.flowProject', target: '/projects' },
  { icon: '⚡', labelKey: 'workbench.flowDag', target: '/dag' },
  { icon: '🚀', labelKey: 'workbench.flowRun', target: '/dag' },
  { icon: '📦', labelKey: 'workbench.flowResult', target: '/results' },
];

/** 待处理（未审批）消息状态集合，与 messages 页面保持一致。 */
const PENDING_STATUSES = ['PENDING', 'WAITING', 'REVIEWING'];

export const WorkbenchPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { isP2p } = usePlatform();
  const { user } = useAuthStore();

  // 当前用户所属节点（ownerId），消息类接口需要该参数。
  const ownerId = user?.ownerId || '';

  // 欢迎头部引导流程图的展开/收起状态（默认收起，保持首屏简洁）。
  const [unfold, setUnfold] = useState(false);

  /* ------------------------------ 数据查询 ------------------------------ */

  // 节点列表：用于统计节点数量与就绪数。
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });
  const nodes = nodesQuery.data ?? [];
  const readyNodes = nodes.filter((n) => n.nodeStatus === 'Ready' || n.status === 'Ready').length;

  // 数据表统计：与 dashboard 一致，按节点并行查询后求和（后端无全平台一次性接口）。
  const tablesQueries = useQueries({
    queries: nodes.map((node) => ({
      queryKey: ['datatables', node.nodeId],
      queryFn: () => apiClient.getDataTables(node.nodeId),
      enabled: nodes.length > 0,
    })),
  });
  const totalTables = tablesQueries.reduce((sum, q) => sum + (q.data?.length ?? 0), 0);

  // 项目列表：P2P 模式用 P2P 项目接口，其余模式用协作项目接口。
  const p2pProjectsQuery = useQuery({
    queryKey: ['p2p-projects'],
    queryFn: () => apiClient.listP2pProjects(),
    enabled: isP2p,
  });
  const centerProjectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
    enabled: !isP2p,
  });
  const projectCount = isP2p
    ? (p2pProjectsQuery.data?.length ?? 0)
    : (centerProjectsQuery.data?.length ?? 0);

  // 待处理申请数量 + 最近消息列表（仅在有 ownerId 时启用）。
  const pendingCountQuery = useQuery({
    queryKey: ['pending-message-count', ownerId],
    queryFn: () => apiClient.getPendingMessageCount(ownerId),
    enabled: !!ownerId,
  });
  const pendingCount = pendingCountQuery.data ?? 0;

  const messagesQuery = useQuery({
    queryKey: ['messages', ownerId],
    queryFn: () => apiClient.getMessages(ownerId, 1, 100),
    enabled: !!ownerId,
  });
  const messages = messagesQuery.data ?? [];
  // 仅取“待处理”消息用于工作台展示，最多 5 条。
  const pendingMessages = messages
    .filter((m) => PENDING_STATUSES.includes((m.status || '').toUpperCase()))
    .slice(0, 5);

  // P2P 项目预览（最多 5 条），用于“我的项目”卡片。
  const previewProjects = (p2pProjectsQuery.data ?? []).slice(0, 5);

  const error =
    nodesQuery.error?.message ||
    messagesQuery.error?.message ||
    (isP2p ? p2pProjectsQuery.error?.message : centerProjectsQuery.error?.message) ||
    null;

  /* ------------------------------ 渲染 ------------------------------ */

  return (
    <div className="space-y-6">
      {/* 欢迎头部：保留旧版“🍵 欢迎语 + 标语 + 展开/收起”结构。 */}
      <div className="bg-gradient-to-r from-blue-50 via-white to-purple-50 dark:from-blue-950/40 dark:via-gray-900 dark:to-purple-950/40 p-6 rounded-xl border border-gray-200 dark:border-gray-800">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">
              <span className="mr-2">🍵</span>
              {t('workbench.welcome')}
            </h2>
            <p className="text-xs text-gray-500 mt-1">{t('workbench.slogan')}</p>
          </div>
          {/* 展开 / 收起引导流程图 */}
          <button
            onClick={() => setUnfold((v) => !v)}
            className="flex items-center gap-1.5 text-xs text-blue-600 dark:text-blue-400 hover:underline underline-offset-2"
          >
            <span>{unfold ? '▲' : '▼'}</span>
            {unfold ? t('workbench.fold') : t('workbench.expand')}
          </button>
        </div>

        {/* 展开后：一张图看懂概念关系与任务流程（六步引导流水线）。 */}
        {unfold && (
          <div className="mt-5">
            <div className="text-[11px] text-gray-500 font-semibold mb-3">{t('workbench.pipelineTitle')}</div>
            <div className="flex items-center gap-2 overflow-x-auto pb-1">
              {PIPELINE_STEPS.map((step, idx) => (
                <React.Fragment key={step.labelKey}>
                  {/* 步骤节点：点击跳转对应功能页。 */}
                  <button
                    onClick={() => navigate({ to: step.target })}
                    className="flex flex-col items-center gap-1.5 px-4 py-3 rounded-xl bg-white dark:bg-gray-850 border border-gray-200 dark:border-gray-700 hover:border-blue-500/60 hover:shadow-sm transition-all flex-shrink-0 min-w-[92px]"
                  >
                    <span className="text-xl">{step.icon}</span>
                    <span className="text-[11px] font-medium text-gray-700 dark:text-gray-300">
                      {t(step.labelKey)}
                    </span>
                  </button>
                  {/* 步骤间箭头（最后一步不渲染）。 */}
                  {idx < PIPELINE_STEPS.length - 1 && (
                    <span className="text-gray-300 dark:text-gray-600 flex-shrink-0">→</span>
                  )}
                </React.Fragment>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {/* 核心指标统计行 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card bodyClassName="p-4">
          <div className="text-xs text-gray-500 font-medium">{t('workbench.statNodes')}</div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{nodes.length}</div>
          <div className="mt-1 text-[11px] text-emerald-600 font-medium">
            {readyNodes} {t('workbench.readySuffix')}
          </div>
        </Card>
        <Card bodyClassName="p-4">
          <div className="text-xs text-gray-500 font-medium">{t('workbench.statTables')}</div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{totalTables}</div>
          <div className="mt-1 text-[11px] text-gray-400">—</div>
        </Card>
        <Card bodyClassName="p-4">
          <div className="text-xs text-gray-500 font-medium">{t('workbench.statProjects')}</div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{projectCount}</div>
          <div className="mt-1 text-[11px] text-gray-400">—</div>
        </Card>
        <Card bodyClassName="p-4">
          <div className="text-xs text-gray-500 font-medium">{t('workbench.statPending')}</div>
          <div className="mt-2 text-2xl font-bold text-gray-900 dark:text-gray-100">{pendingCount}</div>
          <div className="mt-1 text-[11px] text-gray-400">
            {pendingCount === 0 ? t('workbench.noPending') : '—'}
          </div>
        </Card>
      </div>

      {/* 主体两列布局：左侧申请事项 + 我的项目，右侧快捷入口。 */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-6">
          {/* 申请事项：展示最近的待处理消息。 */}
          <Card
            title={t('workbench.messagesTitle')}
            extra={
              <Button size="sm" variant="link" onClick={() => navigate({ to: '/messages' })}>
                {t('workbench.viewAllMessages')}
              </Button>
            }
          >
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {pendingMessages.length === 0 && (
                <div className="py-4 text-xs text-gray-400 text-center">{t('workbench.messagesEmpty')}</div>
              )}
              {pendingMessages.map((msg) => (
                <div key={msg.voteID || msg.messageName} className="py-3 flex items-center justify-between">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-gray-800 dark:text-gray-200 truncate">
                      {msg.messageName}
                    </div>
                    <div className="text-xs text-gray-400 mt-0.5">
                      {msg.type} • {msg.createTime}
                    </div>
                  </div>
                  <Badge status="warning">{msg.status}</Badge>
                </div>
              ))}
            </div>
          </Card>

          {/* 我的项目：P2P 模式展示 P2P 项目列表预览。 */}
          <Card
            title={t('workbench.projectsTitle')}
            extra={
              <div className="flex items-center gap-2">
                <Button size="sm" variant="link" onClick={() => navigate({ to: isP2p ? '/p2p/projects' : '/projects' })}>
                  {t('workbench.viewAllProjects')}
                </Button>
                {isP2p && (
                  <Button size="sm" variant="primary" onClick={() => navigate({ to: '/p2p/projects' })}>
                    ＋ {t('workbench.createProject')}
                  </Button>
                )}
              </div>
            }
          >
            <div className="space-y-3">
              {isP2p && previewProjects.length === 0 && (
                <div className="text-xs text-gray-400 text-center py-4">{t('workbench.projectsEmpty')}</div>
              )}
              {isP2p &&
                previewProjects.map((proj) => (
                  <div
                    key={proj.projectId}
                    onClick={() => navigate({ to: '/p2p/projects' })}
                    className="p-3.5 rounded-xl border border-gray-100 dark:border-gray-800 hover:border-blue-500/40 bg-gray-50/50 dark:bg-gray-850/50 cursor-pointer transition-all flex items-center justify-between"
                  >
                    <div>
                      <div className="font-semibold text-sm text-gray-900 dark:text-gray-100">
                        {proj.projectName || '-'}
                      </div>
                      <div className="text-xs text-gray-400 font-mono mt-0.5">{proj.projectId}</div>
                    </div>
                    <Badge status={proj.status === 'APPROVED' || proj.status === 'AGREE' ? 'success' : 'default'}>
                      {proj.status || '-'}
                    </Badge>
                  </div>
                ))}
              {/* 非 P2P 模式：展示协作项目数量提示，引导前往项目页。 */}
              {!isP2p && (
                <div
                  onClick={() => navigate({ to: '/projects' })}
                  className="p-3.5 rounded-xl border border-gray-100 dark:border-gray-800 hover:border-blue-500/40 bg-gray-50/50 dark:bg-gray-850/50 cursor-pointer transition-all text-xs text-gray-500"
                >
                  {t('workbench.statProjects')}: {projectCount} → {t('workbench.viewAllProjects')}
                </div>
              )}
            </div>
          </Card>
        </div>

        {/* 快捷入口 */}
        <div className="space-y-6">
          <Card title={t('workbench.quickTitle')}>
            <div className="grid grid-cols-1 gap-2.5">
              <Button onClick={() => navigate({ to: '/dag' })} variant="outline" className="justify-start text-xs p-3">
                ⚡ {t('workbench.quickDag')}
              </Button>
              <Button onClick={() => navigate({ to: '/data-tables' })} variant="outline" className="justify-start text-xs p-3">
                🗄️ {t('workbench.quickData')}
              </Button>
              <Button onClick={() => navigate({ to: '/nodes' })} variant="outline" className="justify-start text-xs p-3">
                🖥️ {t('workbench.quickNode')}
              </Button>
              <Button onClick={() => navigate({ to: '/privacy-scenes' })} variant="outline" className="justify-start text-xs p-3">
                🛡️ {t('workbench.quickScenes')}
              </Button>
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
};
