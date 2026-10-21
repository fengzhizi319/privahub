/**
 * Periodic tasks (legacy `periodic-task-list` + `periodic-child-task-list`).
 *
 * - server paging / search (scheduleId) / status filter (UP / DOWN) / createTime sort
 * - offline (disabled while a sub task runs) / delete (offline tasks only)
 * - sub-task history drawer: stop, rerun (failed part `type=1` or all `type=0`)
 * - create: structured schedule form for a successfully-run graph
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { useLocation, useNavigate } from '@tanstack/react-router';
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  Drawer,
  FormField,
  Input,
  Modal,
  Pagination,
  RadioGroup,
  Select,
  toast,
} from '@secretpad/design-system';
import {
  apiClient,
  deleteScheduledJava,
  offlineScheduledJava,
  pageScheduledJava,
  pageScheduledTasksJava,
  rerunScheduledTaskJava,
  stopScheduledTaskJava,
  type PageScheduledVOJava,
  type RerunTypeJava,
  type TaskPageScheduledVOJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { ScheduleCreateDialog } from '../../features/schedule-form';
import {
  canOperateSchedule,
  scheduleAction,
  scheduleStatusBadge,
  shouldPollSubTasks,
  subTaskActions,
  subTaskStatusBadge,
} from './helpers';

const PAGE_SIZE = 10;

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export const PeriodicTasksPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const platform = usePlatform();
  const urlProjectId = String((location.search as Record<string, unknown>)?.projectId ?? '');

  const [selectedProjectId, setSelectedProjectId] = useState<string>(urlProjectId);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [status, setStatus] = useState('');
  const [sortDesc, setSortDesc] = useState<boolean | null>(null);
  const [page, setPage] = useState(1);

  const [createOpen, setCreateOpen] = useState(false);
  const [createGraphId, setCreateGraphId] = useState('');
  const [offlineTarget, setOfflineTarget] = useState<PageScheduledVOJava | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PageScheduledVOJava | null>(null);
  const [runsTask, setRunsTask] = useState<PageScheduledVOJava | null>(null);

  useEffect(() => {
    const id = setTimeout(() => {
      setDebouncedSearch(search);
      setPage(1);
    }, 300);
    return () => clearTimeout(id);
  }, [search]);

  const projectsQuery = useQuery({ queryKey: ['projects'], queryFn: () => apiClient.getProjects() });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);

  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) setSelectedProjectId(projects[0].projectId);
  }, [projects, selectedProjectId]);

  const tasksKey = ['scheduled-page', selectedProjectId, debouncedSearch, status, sortDesc, page];
  const tasksQuery = useQuery({
    queryKey: tasksKey,
    queryFn: () =>
      pageScheduledJava({
        projectId: selectedProjectId,
        search: debouncedSearch,
        status,
        page,
        size: PAGE_SIZE,
        sort: sortDesc === null ? {} : { createTime: sortDesc ? 'DESC' : 'ASC' },
      }),
    enabled: !!selectedProjectId,
    placeholderData: keepPreviousData,
  });
  const tasks = tasksQuery.data?.list ?? [];
  const total = tasksQuery.data?.total ?? 0;

  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: createOpen && !!selectedProjectId,
  });
  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', selectedProjectId, createGraphId],
    queryFn: () => apiClient.getGraphDetail(selectedProjectId, createGraphId),
    enabled: createOpen && !!createGraphId,
  });
  const createNodeIds = useMemo(
    () => (graphDetailQuery.data?.nodes || []).map((n) => n.graphNodeId || '').filter(Boolean),
    [graphDetailQuery.data],
  );

  const invalidateTasks = () => queryClient.invalidateQueries({ queryKey: ['scheduled-page'] });

  const offlineMutation = useMutation({
    mutationFn: (task: PageScheduledVOJava) => offlineScheduledJava(task.scheduleId!),
    onSuccess: () => {
      setOfflineTarget(null);
      toast.success(t('periodicTasks.offlineSuccess'));
      invalidateTasks();
    },
    onError: (e) => toast.error(errText(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (task: PageScheduledVOJava) => deleteScheduledJava(task.scheduleId!),
    onSuccess: () => {
      setDeleteTarget(null);
      toast.success(t('periodicTasks.deleteSuccess'));
      invalidateTasks();
    },
    onError: (e) => toast.error(errText(e)),
  });

  const openDetail = (task: PageScheduledVOJava) =>
    navigate({
      to: '/periodic-tasks/detail',
      search: { scheduleId: task.scheduleId || '', projectId: selectedProjectId, graphId: undefined },
    });

  const canOperate = (task: PageScheduledVOJava) =>
    canOperateSchedule(task, { isP2p: platform.isP2p, ownerId: platform.ownerId });

  const queryError = tasksQuery.error || projectsQuery.error;

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('periodicTasks.title')}</h2>
          <p className="text-xs text-gray-500">{t('periodicTasks.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <Select
            aria-label={t('periodicTasks.project')}
            value={selectedProjectId}
            onChange={(v) => {
              setSelectedProjectId(v);
              setPage(1);
            }}
            options={projects.map((p) => ({ value: p.projectId, label: p.projectName }))}
          />
          <Button
            variant="primary"
            disabled={!selectedProjectId}
            onClick={() => {
              setCreateGraphId('');
              setCreateOpen(true);
            }}
          >
            {t('periodicTasks.create')}
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Input
          className="w-56"
          placeholder={t('periodicTasks.searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <RadioGroup
          name={t('periodicTasks.status')}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: '', label: t('periodicTasks.statusAll') },
            { value: 'UP', label: t('periodicTasks.statusUP') },
            { value: 'DOWN', label: t('periodicTasks.statusDOWN') },
          ]}
        />
      </div>

      {queryError && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: queryError.message })}
        </div>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
        <table className="w-full text-left text-xs">
          <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
            <tr>
              <th className="p-4">{t('periodicTasks.scheduleId')}</th>
              <th className="p-4">{t('periodicTasks.desc')}</th>
              <th className="p-4">{t('periodicTasks.status')}</th>
              <th className="p-4">{t('periodicTasks.creator')}</th>
              {platform.isP2p && <th className="p-4">{t('periodicTasks.owner')}</th>}
              <th className="p-4">
                <button
                  type="button"
                  className="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-300"
                  onClick={() => {
                    setSortDesc((s) => (s === null ? true : s ? false : null));
                    setPage(1);
                  }}
                >
                  {t('periodicTasks.deployTime')}
                  <span aria-hidden>{sortDesc === null ? '↕' : sortDesc ? '↓' : '↑'}</span>
                </button>
              </th>
              <th className="p-4">{t('common.action')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {tasksQuery.isLoading && (
              <tr>
                <td colSpan={7} className="p-4 text-center text-gray-400">
                  {t('common.loading')}
                </td>
              </tr>
            )}
            {tasks.length === 0 && !tasksQuery.isLoading && !queryError && (
              <tr>
                <td colSpan={7} className="p-4 text-center text-gray-400">
                  {t('periodicTasks.noData')}
                </td>
              </tr>
            )}
            {tasks.map((task) => {
              const action = scheduleAction(task);
              return (
                <tr key={task.scheduleId}>
                  <td className="p-4">
                    <button
                      type="button"
                      className="font-mono text-blue-600 dark:text-blue-400 hover:underline"
                      onClick={() => openDetail(task)}
                    >
                      {task.scheduleId}
                    </button>
                  </td>
                  <td className="p-4 text-gray-700 dark:text-gray-300 max-w-xs truncate">{task.scheduleDesc || '-'}</td>
                  <td className="p-4">
                    <Badge status={scheduleStatusBadge(task.scheduleStats)}>
                      {task.scheduleStats === 'UP' || task.scheduleStats === 'DOWN'
                        ? t(`periodicTasks.status${task.scheduleStats}`)
                        : task.scheduleStats || '-'}
                    </Badge>
                  </td>
                  <td className="p-4 text-gray-500">{task.creator || '-'}</td>
                  {platform.isP2p && <td className="p-4 text-gray-500">{task.ownerName || '-'}</td>}
                  <td className="p-4 text-gray-500">{task.createTime || '-'}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="ghost" onClick={() => setRunsTask(task)}>
                        {t('periodicTasks.history')}
                      </Button>
                      {canOperate(task) && action !== 'delete' && (
                        <span title={action === 'offlineDisabled' ? t('periodicTasks.offlineRunningHint') : undefined}>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={action === 'offlineDisabled'}
                            onClick={() => setOfflineTarget(task)}
                          >
                            {t('periodicTasks.offline')}
                          </Button>
                        </span>
                      )}
                      {canOperate(task) && action === 'delete' && (
                        <Button size="sm" variant="danger" onClick={() => setDeleteTarget(task)}>
                          {t('common.delete')}
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        </div>
        {total > 0 && (
          <div className="p-3 border-t border-gray-100 dark:border-gray-800">
            <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
          </div>
        )}
      </Card>

      <ScheduleCreateDialog
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        projectId={selectedProjectId}
        graphId={createGraphId}
        nodeIds={createNodeIds}
        onCreated={invalidateTasks}
      >
        <FormField label={t('periodicTasks.selectGraph')} required help={t('periodicTasks.createHint')}>
          <Select
            className="w-full"
            value={createGraphId}
            placeholder="-"
            onChange={setCreateGraphId}
            options={(graphsQuery.data ?? []).map((g) => ({ value: g.graphId || '', label: g.name || g.graphId || '' }))}
          />
        </FormField>
      </ScheduleCreateDialog>

      {runsTask && (
        <SubTaskDrawer
          task={runsTask}
          canOperate={canOperate(runsTask)}
          projectId={selectedProjectId}
          onClose={() => {
            setRunsTask(null);
            invalidateTasks();
          }}
        />
      )}

      <ConfirmDialog
        isOpen={!!offlineTarget}
        title={t('periodicTasks.offline')}
        message={t('periodicTasks.offlineConfirm')}
        danger
        loading={offlineMutation.isPending}
        confirmText={t('periodicTasks.offline')}
        cancelText={t('common.cancel')}
        onConfirm={() => offlineTarget && offlineMutation.mutate(offlineTarget)}
        onCancel={() => setOfflineTarget(null)}
      />

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('periodicTasks.deleteTitle', { id: deleteTarget?.scheduleId || '' })}
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

/** Sub-task (schedule history) drawer with stop / rerun. */
const SubTaskDrawer: React.FC<{
  task: PageScheduledVOJava;
  canOperate: boolean;
  projectId: string;
  onClose: () => void;
}> = ({ task, canOperate, projectId, onClose }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const scheduleId = task.scheduleId || '';
  const [page, setPage] = useState(1);
  const [rerunTarget, setRerunTarget] = useState<TaskPageScheduledVOJava | null>(null);
  const [stopTarget, setStopTarget] = useState<TaskPageScheduledVOJava | null>(null);

  const runsQuery = useQuery({
    queryKey: ['scheduled-task-page', scheduleId, page],
    queryFn: () =>
      pageScheduledTasksJava({
        scheduleId,
        page,
        size: PAGE_SIZE,
        sort: { scheduleTaskExpectStartTime: 'ASC' },
        search: '',
      }),
    enabled: !!scheduleId,
    placeholderData: keepPreviousData,
    refetchInterval: (q) => (shouldPollSubTasks(q.state.data?.list ?? []) ? 10000 : false),
  });
  const runs = runsQuery.data?.list ?? [];
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['scheduled-task-page', scheduleId] });

  const rerunMutation = useMutation({
    mutationFn: (input: { run: TaskPageScheduledVOJava; type: RerunTypeJava }) =>
      rerunScheduledTaskJava({ scheduleId, scheduleTaskId: input.run.scheduleTaskId!, type: input.type }),
    onSuccess: () => {
      setRerunTarget(null);
      toast.success(t('periodicTasks.rerunSuccess'));
      refresh();
    },
    onError: (e) => toast.error(errText(e)),
  });

  const stopMutation = useMutation({
    mutationFn: (run: TaskPageScheduledVOJava) => stopScheduledTaskJava({ scheduleId, scheduleTaskId: run.scheduleTaskId! }),
    onSuccess: () => {
      setStopTarget(null);
      toast.success(t('periodicTasks.stopSuccess'));
      refresh();
    },
    onError: (e) => toast.error(errText(e)),
  });

  const rerunActions = rerunTarget ? subTaskActions(rerunTarget) : null;

  return (
    <Drawer isOpen onClose={onClose} width="max-w-4xl" title={t('periodicTasks.historyTitle', { id: scheduleId })}>
      <div className="space-y-3 text-xs">
        {runsQuery.error && <div className="text-rose-500">{t('common.error', { message: runsQuery.error.message })}</div>}
        <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead className="text-gray-500 border-b border-gray-200 dark:border-gray-800">
            <tr>
              <th className="p-2">{t('periodicTasks.subTaskId')}</th>
              <th className="p-2">{t('periodicTasks.expectStart')}</th>
              <th className="p-2">{t('periodicTasks.actualStart')}</th>
              <th className="p-2">{t('periodicTasks.endTime')}</th>
              <th className="p-2">{t('periodicTasks.status')}</th>
              <th className="p-2">{t('common.action')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {runsQuery.isLoading && (
              <tr>
                <td colSpan={6} className="p-3 text-center text-gray-400">
                  {t('common.loading')}
                </td>
              </tr>
            )}
            {!runsQuery.isLoading && runs.length === 0 && (
              <tr>
                <td colSpan={6} className="p-3 text-center text-gray-400">
                  {t('periodicTasks.noSubTasks')}
                </td>
              </tr>
            )}
            {runs.map((run) => {
              const actions = subTaskActions(run);
              return (
                <tr key={run.scheduleTaskId}>
                  <td className="p-2">
                    <button
                      type="button"
                      className="font-mono text-blue-600 dark:text-blue-400 hover:underline"
                      onClick={() =>
                        navigate({
                          to: '/periodic-tasks/detail',
                          search: {
                            scheduleId,
                            projectId,
                            graphId: undefined,
                            scheduleTaskId: run.scheduleTaskId,
                          },
                        })
                      }
                    >
                      {run.scheduleTaskId}
                    </button>
                  </td>
                  <td className="p-2 text-gray-500">{run.scheduleTaskExpectStartTime || '-'}</td>
                  <td className="p-2 text-gray-500">{run.scheduleTaskStartTime || '-'}</td>
                  <td className="p-2 text-gray-500">{run.scheduleTaskEndTime || '-'}</td>
                  <td className="p-2">
                    <Badge status={subTaskStatusBadge(run.scheduleTaskStatus)}>
                      {run.scheduleTaskStatus ? t(`periodicTasks.subStatus.${run.scheduleTaskStatus}`) : '-'}
                    </Badge>
                  </td>
                  <td className="p-2">
                    {!canOperate || (!actions.stop && actions.rerun.length === 0) ? (
                      '-'
                    ) : (
                      <div className="flex gap-2">
                        {actions.stop && (
                          <Button size="sm" variant="danger" onClick={() => setStopTarget(run)}>
                            {t('periodicTasks.stop')}
                          </Button>
                        )}
                        {actions.rerun.length > 0 && (
                          <Button size="sm" variant="ghost" onClick={() => setRerunTarget(run)}>
                            {t('periodicTasks.rerun')}
                          </Button>
                        )}
                      </div>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        </div>
        {(runsQuery.data?.total ?? 0) > 0 && (
          <Pagination page={page} pageSize={PAGE_SIZE} total={runsQuery.data?.total ?? 0} onChange={setPage} />
        )}
      </div>

      <Modal
        isOpen={!!rerunTarget}
        onClose={() => setRerunTarget(null)}
        title={t('periodicTasks.rerun')}
        width="max-w-md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setRerunTarget(null)}>
              {t('common.cancel')}
            </Button>
            {rerunActions?.rerun.map((opt) => (
              <Button
                key={opt.type}
                variant={opt.type === rerunActions.rerun[0].type ? 'primary' : 'outline'}
                disabled={opt.disabled}
                loading={rerunMutation.isPending && rerunMutation.variables?.type === opt.type}
                onClick={() => rerunTarget && rerunMutation.mutate({ run: rerunTarget, type: opt.type })}
              >
                {t(opt.labelKey)}
              </Button>
            ))}
          </>
        }
      >
        <p className="text-sm text-gray-700 dark:text-gray-300">
          {rerunActions?.confirmKey ? t(rerunActions.confirmKey) : ''}
        </p>
      </Modal>

      <ConfirmDialog
        isOpen={!!stopTarget}
        title={t('periodicTasks.stop')}
        message={t('periodicTasks.stopConfirm')}
        danger
        loading={stopMutation.isPending}
        confirmText={t('periodicTasks.stop')}
        cancelText={t('common.cancel')}
        onConfirm={() => stopTarget && stopMutation.mutate(stopTarget)}
        onCancel={() => setStopTarget(null)}
      />
    </Drawer>
  );
};
