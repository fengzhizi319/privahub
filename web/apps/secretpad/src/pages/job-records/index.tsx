import { QueryFeedback } from '../../shared/ui/query-feedback';
import { FilterBar } from '../../shared/ui/filter-bar';
import { StatusBadge } from '../../shared/ui/status-badge';
import { TableView } from '../../shared/ui/table-view';
/**
 * DAG / Pipeline 运行记录页（旧版 pipeline-record-list + dag-record）。
 *
 * - 按项目（可选训练流）分页查询 project/job/list（Java PageResponse）；
 * - 周期任务：选择周期任务与子任务后查询 scheduled/job/list；
 * - 运行中记录可停止（单条 / 多选批量）；存在运行中记录时每 5s 刷新；
 * - 点击记录打开回放（只读画布快照 + 节点日志与结果，与 DAG 画布共用渲染器）。
 *
 * 注：旧版“删除记录”为空实现（record-service.deleteRecords 直接 resolve），后端亦无删除接口，故不提供。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, toast } from '@secretpad/design-system';
import type { ProjectJobSummaryVOJava } from '@secretpad/api-client';
import { apiClient, listGraphJobsJava, listScheduledJobsJava, pageScheduledJava, pageScheduledTasksJava, stopProjectJobJava } from '@secretpad/api-client';
import { NODE_STATUS, formatTimestamp, normalizeStatus } from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';
import { JobDetailModal } from '../../features/job-detail';

const PAGE_SIZE = 10;

function duration(start?: string, end?: string): string {
  if (!start) return '-';
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(s) || Number.isNaN(e)) return '-';
  const sec = Math.max(0, Math.floor((e - s) / 1000));
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return h > 0 ? `${h}h ${m}m ${sec % 60}s` : m > 0 ? `${m}m ${sec % 60}s` : `${sec}s`;
}

export const JobRecordsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [projectId, setProjectId] = useState('');
  const [graphId, setGraphId] = useState('');
  const [mode, setMode] = useState<'normal' | 'periodic'>('normal');
  const [scheduleId, setScheduleId] = useState('');
  const [scheduleTaskId, setScheduleTaskId] = useState('');
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [searchKey, setSearchKey] = useState('');
  const [status, setStatus] = useState('');
  const [startTime, setStartTime] = useState('');
  const [endTime, setEndTime] = useState('');
  useEffect(() => { const timer=setTimeout(()=>{setSearchKey(search.trim());setPage(1);},300);return ()=>clearTimeout(timer); },[search]);
  const invalidDates = !!startTime && !!endTime && endTime < startTime;
  const filters = {searchKey,status,startTime:startTime ? new Date(startTime).toISOString() : undefined,endTime:endTime ? new Date(endTime).toISOString() : undefined};

  const [selected, setSelected] = useState<Set<string>>(new Set());
  useEffect(() => {setSelected(new Set());}, [projectId,graphId,mode,scheduleTaskId,page,searchKey,status,startTime,endTime]);

  const [detail, setDetail] = useState<ProjectJobSummaryVOJava | null>(null);

  const projectsQuery = useQuery({ queryKey: ['projects'], queryFn: () => apiClient.getProjects() });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);
  useEffect(() => {
    if (!projectId && projects.length > 0) setProjectId(projects[0].projectId);
  }, [projects, projectId]);
  const project = projects.find((p) => p.projectId === projectId);

  const graphsQuery = useQuery({ queryKey: ['graphs', projectId], queryFn: () => apiClient.getGraphs(projectId), enabled: !!projectId });
  const schedulesQuery = useQuery({
    queryKey: ['job-records-schedules', projectId],
    queryFn: () => pageScheduledJava({ projectId, page: 1, size: 100 }),
    enabled: mode === 'periodic' && !!projectId,
  });
  const tasksQuery = useQuery({
    queryKey: ['job-records-schedule-tasks', scheduleId],
    queryFn: () => pageScheduledTasksJava({ scheduleId, page: 1, size: 100 }),
    enabled: mode === 'periodic' && !!scheduleId,
  });

  const recordsQuery = useQuery({
    queryKey: ['job-records', projectId, graphId, mode, scheduleTaskId, page, searchKey, status, startTime, endTime],
    queryFn: () =>
      mode === 'periodic'
        ? listScheduledJobsJava({ projectId, graphId, scheduleTaskId, pageNum: page, pageSize: PAGE_SIZE, ...filters })
        : listGraphJobsJava({ projectId, graphId: graphId || undefined, pageNum: page, pageSize: PAGE_SIZE, ...filters }),
    enabled: !!projectId && !invalidDates && (mode === 'normal' || !!scheduleTaskId),
    refetchInterval: (q) => ((q.state.data as { list: ProjectJobSummaryVOJava[] } | undefined)?.list || []).some((j) => normalizeStatus(j.status) === NODE_STATUS.RUNNING) ? 5000 : false,
  });

  const stopMutation = useMutation({
    mutationFn: async (jobIds: string[]) => {
      for (const id of jobIds) await stopProjectJobJava(projectId, id);
    },
    onSuccess: () => {
      setSelected(new Set());
      void queryClient.invalidateQueries({ queryKey: ['job-records', projectId] });
      toast.success(t('jobRecords.stopSuccess'));
    },
    onError: (e) => toast.error(t('jobRecords.stopError', { message: e instanceof Error ? e.message : String(e) })),
  });

  const list = recordsQuery.data?.list ?? [];
  const total = recordsQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const running = list.filter((j) => normalizeStatus(j.status) === NODE_STATUS.RUNNING);
  const selectCls = 'px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100';

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('jobRecords.title')}</h2>
          <p className="text-xs text-gray-500">{t('jobRecords.subtitle')}</p>
        </div>
      <FilterBar onReset={()=>{setSearch('');setSearchKey('');setStatus('');setStartTime('');setEndTime('');setPage(1);}}>
          <select aria-label={t('jobRecords.project')} className={selectCls} value={projectId} onChange={(e) => { setProjectId(e.target.value); setGraphId(''); setScheduleId(''); setScheduleTaskId(''); setPage(1); }}>
            {projects.map((p) => (
              <option key={p.projectId} value={p.projectId}>
                {p.projectName || p.projectId}
              </option>
            ))}
          </select>
          <select aria-label="graph" className={selectCls} value={graphId} onChange={(e) => { setGraphId(e.target.value); setPage(1); }}>
            <option value="">{t('dagx.allGraphs')}</option>
            {(graphsQuery.data ?? []).map((g) => (
              <option key={g.graphId} value={g.graphId || ''}>
                {g.name}
              </option>
            ))}
          </select>
          <div className="inline-flex rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden text-xs">
            <button type="button" className={`px-2 py-1.5 ${mode === 'normal' ? 'bg-blue-600 text-white' : ''}`} onClick={() => { setMode('normal'); setPage(1); }}>
              {t('dagx.records')}
            </button>
            <button type="button" className={`px-2 py-1.5 ${mode === 'periodic' ? 'bg-blue-600 text-white' : ''}`} onClick={() => { setMode('periodic'); setPage(1); }}>
              {t('dagx.periodicRecords')}
            </button>
          </div>
          {mode === 'periodic' && (
            <>
              <select aria-label="schedule" className={selectCls} value={scheduleId} onChange={(e) => { setScheduleId(e.target.value); setScheduleTaskId(''); }}>
                <option value="">{t('dagx.periodicRecords')}</option>
                {(schedulesQuery.data?.list ?? []).map((s) => (
                  <option key={s.scheduleId} value={s.scheduleId}>
                    {s.scheduleId}
                  </option>
                ))}
              </select>
              <select aria-label={t('dagx.periodicTask')} className={selectCls} value={scheduleTaskId} onChange={(e) => { setScheduleTaskId(e.target.value); setPage(1); }}>
                <option value="">{t('dagx.periodicTask')}</option>
                {(tasksQuery.data?.list ?? []).map((task) => (
                  <option key={task.scheduleTaskId} value={task.scheduleTaskId}>
                    {task.scheduleTaskId} · {task.scheduleTaskStatus}
                  </option>
                ))}
              </select>
            </>
          )}
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label={t('display.search')}
            placeholder={t('jobRecords.searchPlaceholder')}
            className={selectCls}
          />
          <select aria-label={t('display.statusFilter')} value={status} onChange={event=>{setStatus(event.target.value);setPage(1);}} className={selectCls}>
            <option value="">{t('display.allStatus')}</option>{['RUNNING','SUCCESS','FAILED','STOPPED','STAGING'].map(value=><option key={value} value={value}>{t(`display.status.${value}`)}</option>)}
          </select>
          <label className="text-xs text-gray-500">{t('display.startTime')} <input aria-label={t('display.startTime')} type="datetime-local" value={startTime} onChange={event=>{setStartTime(event.target.value);setPage(1);}} className={selectCls}/></label>
          <label className="text-xs text-gray-500">{t('display.endTime')} <input aria-label={t('display.endTime')} type="datetime-local" value={endTime} onChange={event=>{setEndTime(event.target.value);setPage(1);}} className={selectCls}/></label>
          <Button variant="outline" disabled={invalidDates} onClick={() => void recordsQuery.refetch()} loading={recordsQuery.isFetching}>
            {t('common.refresh')}
          </Button>
          <AccessGuard access={{ types: [Platform.CENTER, Platform.AUTONOMY, Platform.P2P] }}>
            <Button variant="danger" disabled={selected.size === 0} loading={stopMutation.isPending} onClick={() => stopMutation.mutate([...selected])}>
              {t('jobRecords.stop')} ({selected.size})
            </Button>
          </AccessGuard>
        </FilterBar>
      </div>

      {invalidDates && <p role="alert" className="text-sm text-rose-600">{t('display.dateError')}</p>}
      <QueryFeedback error={recordsQuery.error} onRetry={()=>void recordsQuery.refetch()}/>

      <Card bodyClassName="p-0">
        <TableView id="job-records"><table className="w-full text-left text-xs">
          <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold uppercase border-b border-gray-200 dark:border-gray-800">
            <tr>
              <th className="p-4 w-8">
                <input
                  type="checkbox"
                  aria-label="select all running"
                  checked={running.length > 0 && running.every((j) => selected.has(j.jobId || ''))}
                  onChange={(e) => setSelected(e.target.checked ? new Set(running.map((j) => j.jobId || '')) : new Set())}
                />
              </th>
              <th className="p-4">{t('jobRecords.name')}</th>
              <th className="p-4">{t('jobRecords.status')}</th>
              <th className="p-4">{t('jobRecords.tasks')}</th>
              <th className="p-4">{t('jobRecords.duration')}</th>
              <th className="p-4">{t('jobRecords.createTime')}</th>
              <th className="p-4">{t('common.action')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {recordsQuery.isLoading && (
              <tr>
                <td colSpan={7} className="p-4 text-center text-gray-400">
                  <QueryFeedback loading/>
                </td>
              </tr>
            )}
            {!recordsQuery.isLoading && !recordsQuery.error && list.length === 0 && (
              <tr>
                <td colSpan={7} className="p-4 text-center text-gray-400">
                  <QueryFeedback empty label={t('jobRecords.noData')}/>
                </td>
              </tr>
            )}
            {list.map((job) => {
              const st = normalizeStatus(job.status);
              const isRunning = st === NODE_STATUS.RUNNING;
              const graphName = (job as { graph?: { name?: string } }).graph?.name;
              return (
                <tr key={job.jobId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4">
                    <input
                      type="checkbox"
                      aria-label={`select ${job.jobId}`}
                      disabled={!isRunning}
                      checked={selected.has(job.jobId || '')}
                      onChange={(e) =>
                        setSelected((prev) => {
                          const next = new Set(prev);
                          if (e.target.checked) next.add(job.jobId || '');
                          else next.delete(job.jobId || '');
                          return next;
                        })
                      }
                    />
                  </td>
                  <td className="p-4 font-semibold text-gray-800 dark:text-gray-200">
                    <button type="button" className="text-left hover:text-blue-600" onClick={() => setDetail(job)}>
                      <div>{graphName || job.name || job.jobId}</div>
                      <div className="font-mono text-[10px] text-gray-400 truncate max-w-64" title={job.jobId}>{job.jobId}</div>
                    </button>
                    {job.errMsg && <div title={job.errMsg} className="text-red-500 text-xs truncate max-w-72">{job.errMsg}</div>}
                  </td>
                  <td className="p-4">
                    <StatusBadge value={job.status} />
                  </td>
                  <td className="p-4 text-gray-500">
                    {job.finishedTaskCount ?? 0}/{job.taskCount ?? 0}
                  </td>
                  <td className="p-4 text-gray-500">{duration(job.gmtCreate, job.gmtFinished)}</td>
                  <td className="p-4 text-gray-500">{formatTimestamp(job.gmtCreate)}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="ghost" onClick={() => setDetail(job)}>
                        {t('common.detail')}
                      </Button>
                      {isRunning && (
                        <AccessGuard access={{ types: [Platform.CENTER, Platform.AUTONOMY, Platform.P2P] }}>
                          <Button size="sm" variant="danger" loading={stopMutation.isPending} onClick={() => stopMutation.mutate([job.jobId || ''])}>
                            {t('jobRecords.stop')}
                          </Button>
                        </AccessGuard>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table></TableView>
        {totalPages > 1 && (
          <div className="flex items-center justify-end gap-2 p-3 text-xs">
            <Button size="sm" variant="ghost" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              {t('common.previous')}
            </Button>
            <span>
              {page} / {totalPages}
            </span>
            <Button size="sm" variant="ghost" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
              {t('common.next')}
            </Button>
          </div>
        )}
      </Card>

      <JobDetailModal
        projectId={projectId}
        jobId={detail?.jobId || ''}
        summary={detail ?? undefined}
        computeMode={project?.computeMode}
        isOpen={!!detail?.jobId}
        onClose={() => setDetail(null)}
      />
    </div>
  );
};
