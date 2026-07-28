import React, { useEffect, useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Badge, Button, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { PageScheduledVO, TaskPageScheduledVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';

export const PeriodicTasksPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [selectedProjectId, setSelectedProjectId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  // Create modal
  const [createOpen, setCreateOpen] = useState(false);
  const [createGraphId, setCreateGraphId] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createCron, setCreateCron] = useState('0 0 2 * * ?');

  // Delete confirm
  const [deleteTarget, setDeleteTarget] = useState<PageScheduledVO | null>(null);

  // Task runs drawer
  const [runsTask, setRunsTask] = useState<PageScheduledVO | null>(null);

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);

  // Default the selected project to the first one once projects load.
  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) {
      setSelectedProjectId(projects[0].projectId);
    }
  }, [projects, selectedProjectId]);

  const tasksQuery = useQuery({
    queryKey: ['scheduled-tasks', selectedProjectId],
    queryFn: () => apiClient.getScheduledTasks(selectedProjectId),
    enabled: !!selectedProjectId,
  });
  const tasks = tasksQuery.data ?? [];

  // Graphs for the create form
  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: createOpen && !!selectedProjectId,
  });
  const graphs = graphsQuery.data ?? [];

  // Graph detail to collect node ids for create
  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', selectedProjectId, createGraphId],
    queryFn: () => apiClient.getGraphDetail(selectedProjectId, createGraphId),
    enabled: createOpen && !!createGraphId,
  });

  // Task runs for the drawer
  const runsQuery = useQuery({
    queryKey: ['scheduled-task-runs', runsTask?.scheduleId],
    queryFn: () => apiClient.getScheduledTaskPage(runsTask!.scheduleId!),
    enabled: !!runsTask?.scheduleId,
  });
  const runs: TaskPageScheduledVO[] = runsQuery.data ?? [];

  const invalidateTasks = () =>
    queryClient.invalidateQueries({ queryKey: ['scheduled-tasks', selectedProjectId] });

  const createMutation = useMutation({
    mutationFn: () => {
      const nodeIds = (graphDetailQuery.data?.nodes || []).map((n) => n.graphNodeId || '').filter(Boolean);
      return apiClient.createScheduledGraph({
        projectId: selectedProjectId,
        graphId: createGraphId,
        nodes: nodeIds,
        scheduleDesc: createDesc || undefined,
        cron: { scheduleTime: createCron },
      });
    },
    onSuccess: () => {
      setCreateOpen(false);
      setCreateGraphId('');
      setCreateDesc('');
      invalidateTasks();
      toast.success(t('periodicTasks.createSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const offlineMutation = useMutation({
    mutationFn: (task: PageScheduledVO) => apiClient.offlineScheduledTask(task.scheduleId!),
    onSuccess: invalidateTasks,
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (task: PageScheduledVO) => apiClient.deleteScheduledTask(task.scheduleId!),
    onSuccess: () => {
      setDeleteTarget(null);
      invalidateTasks();
    },
    onError: (e) => {
      setDeleteTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const rerunMutation = useMutation({
    mutationFn: (run: TaskPageScheduledVO) =>
      apiClient.rerunScheduledTask({ scheduleId: runsTask!.scheduleId!, scheduleTaskId: run.scheduleTaskId! }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scheduled-task-runs', runsTask?.scheduleId] });
      toast.success(t('periodicTasks.rerunSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const stopRunMutation = useMutation({
    mutationFn: (run: TaskPageScheduledVO) =>
      apiClient.stopScheduledTask({ scheduleId: runsTask!.scheduleId!, scheduleTaskId: run.scheduleTaskId! }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scheduled-task-runs', runsTask?.scheduleId] });
      toast.success(t('periodicTasks.stopSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const statusBadge = (status?: string) => {
    switch (status?.toUpperCase()) {
      case 'ONLINE':
      case 'ACTIVE':
      case 'SUCCEED':
        return 'success';
      case 'OFFLINE':
      case 'PAUSED':
        return 'warning';
      case 'RUNNING':
        return 'processing';
      case 'FAILED':
        return 'error';
      default:
        return 'default';
    }
  };

  const loading = tasksQuery.isLoading;
  const queryError = tasksQuery.error?.message || projectsQuery.error?.message || null;

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('periodicTasks.title')}</h2>
          <p className="text-xs text-gray-500">{t('periodicTasks.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={selectedProjectId}
            onChange={(e) => setSelectedProjectId(e.target.value)}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {projects.map((p) => (
              <option key={p.projectId} value={p.projectId}>{p.projectName}</option>
            ))}
          </select>
          <AccessGuard access={{ types: [Platform.CENTER] }}>
            <Button variant="primary" onClick={() => { setCreateGraphId(''); setCreateDesc(''); setCreateOpen(true); }}>
              {t('periodicTasks.create')}
            </Button>
          </AccessGuard>
        </div>
      </div>

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <Card bodyClassName="p-0">
        <table className="w-full text-left text-xs">
          <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold uppercase border-b border-gray-200 dark:border-gray-800">
            <tr>
              <th className="p-4">{t('periodicTasks.name')}</th>
              <th className="p-4">{t('periodicTasks.status')}</th>
              <th className="p-4">{t('periodicTasks.creator')}</th>
              <th className="p-4">{t('periodicTasks.createTime')}</th>
              <th className="p-4">{t('common.action')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {tasks.length === 0 && !loading && !queryError && (
              <tr>
                <td colSpan={5} className="p-4 text-center text-gray-400">{t('periodicTasks.noData')}</td>
              </tr>
            )}
            {tasks.map((task) => (
              <tr key={task.scheduleId}>
                <td className="p-4 font-semibold text-gray-800 dark:text-gray-200">
                  {task.scheduleDesc || task.scheduleId}
                </td>
                <td className="p-4">
                  <Badge status={statusBadge(task.scheduleStats)}>{task.scheduleStats}</Badge>
                </td>
                <td className="p-4 text-gray-500">{task.creator || '-'}</td>
                <td className="p-4 text-gray-500">{task.createTime}</td>
                <td className="p-4">
                  <div className="flex items-center gap-2">
                    <Button size="sm" variant="ghost" onClick={() => setRunsTask(task)}>{t('messages.detail')}</Button>
                    <AccessGuard access={{ types: [Platform.CENTER] }}>
                      <Button size="sm" variant="ghost" onClick={() => offlineMutation.mutate(task)}>{t('periodicTasks.offline')}</Button>
                      <Button size="sm" variant="danger" onClick={() => setDeleteTarget(task)}>{t('common.delete')}</Button>
                    </AccessGuard>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      {/* Create Schedule Modal */}
      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title={t('periodicTasks.createTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={() => createMutation.mutate()} loading={createMutation.isPending} disabled={!createGraphId}>{t('common.create')}</Button>
          </>
        }
      >
        <div className="text-xs space-y-4">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('periodicTasks.selectGraph')}</label>
            <select
              value={createGraphId}
              onChange={(e) => setCreateGraphId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              <option value="">-</option>
              {graphs.map((g) => (
                <option key={g.graphId} value={g.graphId || ''}>{g.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('periodicTasks.cronLabel')}</label>
            <input
              type="text"
              value={createCron}
              onChange={(e) => setCreateCron(e.target.value)}
              placeholder={t('periodicTasks.cronPlaceholder')}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('periodicTasks.name')}</label>
            <input
              type="text"
              value={createDesc}
              onChange={(e) => setCreateDesc(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>
      </Modal>

      {/* Task Runs Drawer */}
      {runsTask && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-black/40" onClick={() => setRunsTask(null)} />
          <div className="relative w-full max-w-lg bg-white dark:bg-gray-900 h-full shadow-2xl overflow-y-auto p-6 space-y-4">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="text-lg font-bold text-gray-900 dark:text-gray-100">{runsTask.scheduleDesc || runsTask.scheduleId}</h3>
                <p className="text-xs text-gray-500 font-mono mt-1">{runsTask.scheduleId}</p>
              </div>
              <Button size="sm" variant="ghost" onClick={() => setRunsTask(null)}>✕</Button>
            </div>

            {runsQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
            <div className="space-y-2">
              {runs.length === 0 && !runsQuery.isLoading && (
                <div className="text-xs text-gray-400 text-center py-4">{t('projects.noJobs')}</div>
              )}
              {runs.map((run) => (
                <div key={run.scheduleTaskId} className="flex items-center justify-between p-3 rounded-lg border border-gray-200 dark:border-gray-800 text-xs">
                  <div className="min-w-0">
                    <div className="font-mono text-gray-800 dark:text-gray-200 truncate">{run.scheduleTaskId}</div>
                    <div className="text-gray-400 mt-0.5">{run.scheduleTaskStartTime || run.scheduleTaskExpectStartTime || '-'}</div>
                  </div>
                  <div className="flex items-center gap-2 flex-shrink-0">
                    <Badge status={statusBadge(run.scheduleTaskStatus)}>{run.scheduleTaskStatus}</Badge>
                    <AccessGuard access={{ types: [Platform.CENTER] }}>
                      <Button size="sm" variant="ghost" loading={rerunMutation.isPending} onClick={() => rerunMutation.mutate(run)}>{t('periodicTasks.rerun')}</Button>
                      {run.scheduleTaskStatus === 'RUNNING' && (
                        <Button size="sm" variant="danger" loading={stopRunMutation.isPending} onClick={() => stopRunMutation.mutate(run)}>{t('periodicTasks.stop')}</Button>
                      )}
                    </AccessGuard>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('common.delete')}
        message={t('periodicTasks.deleteConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
