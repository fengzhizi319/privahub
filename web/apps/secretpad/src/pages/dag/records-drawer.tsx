/**
 * DAG 运行记录抽屉（旧版 pipeline-record-list）：
 * - project/job/list?graphId 分页（每页 5 条），存在运行中记录时每 5s 刷新；
 * - 运行中记录可停止；点击记录打开回放（只读画布快照 + 节点结果）；
 * - 周期任务：scheduled/page 选择周期任务 → scheduled/task/page 选择子任务 → scheduled/job/list。
 */
import React, { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Modal, Tour, toast } from '@secretpad/design-system';
import { ExecutionTimeline, NODE_STATUS, normalizeStatus } from '@secretpad/dag-next';
import type { ProjectJobSummaryVOJava } from '@secretpad/api-client';
import {
  listGraphJobsJava,
  pageScheduledJava,
  listScheduledJobsJava,
  pageScheduledTasksJava,
  stopProjectJobJava,
} from '@secretpad/api-client';
import { PAGE_TOUR_KEYS, usePageTour } from '../../features/guide-tour';
import { useTranslation } from '../../shared/lib/i18n';
import { JobReplayView } from '../../features/job-detail';
import { toExecutionRecord } from './adapters';

const PAGE_SIZE = 5;

export const RecordsDrawer: React.FC<{
  open: boolean;
  onClose: () => void;
  projectId: string;
  graphId: string;
  computeMode?: string;
  readOnly?: boolean;
}> = ({ open, onClose, projectId, graphId, computeMode, readOnly }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [mode, setMode] = useState<'normal' | 'periodic'>('normal');
  const [scheduleTaskId, setScheduleTaskId] = useState('');
  const [replay, setReplay] = useState<ProjectJobSummaryVOJava | null>(null);
  const [tourOpen, setTourOpen] = useState(false);

  const [scheduleId, setScheduleId] = useState('');
  const schedulesQuery = useQuery({
    queryKey: ['dag-schedules', projectId],
    queryFn: () => pageScheduledJava({ projectId, page: 1, size: 50 }),
    enabled: open && mode === 'periodic' && !!projectId,
  });
  const schedules = schedulesQuery.data?.list ?? [];

  const tasksQuery = useQuery({
    queryKey: ['dag-schedule-tasks', scheduleId],
    queryFn: () => pageScheduledTasksJava({ scheduleId, page: 1, size: 50 }),
    enabled: open && mode === 'periodic' && !!scheduleId,
  });

  const recordsKey = ['dag-records', projectId, graphId, mode, scheduleTaskId, page];
  const recordsQuery = useQuery({
    queryKey: recordsKey,
    queryFn: () =>
      mode === 'periodic'
        ? listScheduledJobsJava({ projectId, graphId, scheduleTaskId: scheduleTaskId || undefined, pageNum: page, pageSize: PAGE_SIZE })
        : listGraphJobsJava({ projectId, graphId, pageNum: page, pageSize: PAGE_SIZE }),
    enabled: open && !!projectId && !!graphId && (mode === 'normal' || !!scheduleTaskId),
    refetchInterval: (q) => {
      const list = (q.state.data as { list: ProjectJobSummaryVOJava[] } | undefined)?.list || [];
      return list.some((j) => normalizeStatus(j.status) === NODE_STATUS.RUNNING) ? 5000 : false;
    },
  });

  const stopMutation = useMutation({
    mutationFn: (jobId: string) => stopProjectJobJava(projectId, jobId),
    onSuccess: () => {
      toast.success(t('jobRecords.stopSuccess'));
      void queryClient.invalidateQueries({ queryKey: ['dag-records', projectId, graphId] });
      void queryClient.invalidateQueries({ queryKey: ['graph-node-status', projectId, graphId] });
    },
    onError: (e) => toast.error(t('jobRecords.stopError', { message: e instanceof Error ? e.message : String(e) })),
  });

  const list = recordsQuery.data?.list ?? [];
  // 旧版 dag-record-guide-tour：首次打开且有运行记录时自动提示（localStorage RecordGuideTour）。
  const recordTour = usePageTour(PAGE_TOUR_KEYS.record, open && list.length > 0);
  if (!open) return null;
  const total = recordsQuery.data?.total ?? 0;

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-96 h-full bg-gray-900 border-l border-gray-800 shadow-2xl overflow-y-auto p-4 space-y-3" data-tour="dag-records-list">
        <div className="flex items-center justify-between text-xs">
          <div className="inline-flex rounded border border-gray-700 overflow-hidden">
            <button type="button" className={`px-2 py-1 ${mode === 'normal' ? 'bg-blue-600 text-white' : 'text-gray-300'}`} onClick={() => { setMode('normal'); setPage(1); }}>
              {t('dagx.records')}
            </button>
            <button type="button" className={`px-2 py-1 ${mode === 'periodic' ? 'bg-blue-600 text-white' : 'text-gray-300'}`} onClick={() => { setMode('periodic'); setPage(1); }}>
              {t('dagx.periodicRecords')}
            </button>
          </div>
          <div className="flex items-center gap-2">
            <button type="button" className="text-gray-400 hover:text-blue-400" onClick={() => setTourOpen(true)} title={t('dagx.tour.start')}>
              ?
            </button>
            <button type="button" className="text-gray-400 hover:text-white" onClick={onClose} aria-label="close">
              ✕
            </button>
          </div>
        </div>
        {mode === 'periodic' && (
          <div className="text-xs">
            {schedules.length === 0 ? (
              <div className="text-gray-500">{t('dagx.noPeriodic')}</div>
            ) : (
              <div className="space-y-1">
                <select
                  className="w-full p-1.5 rounded bg-gray-800 border border-gray-700 text-gray-200"
                  value={scheduleId}
                  aria-label="schedule"
                  onChange={(e) => {
                    setScheduleId(e.target.value);
                    setScheduleTaskId('');
                    setPage(1);
                  }}
                >
                  <option value="">{t('dagx.periodicRecords')}</option>
                  {schedules.map((sc) => (
                    <option key={sc.scheduleId} value={sc.scheduleId}>
                      {sc.scheduleId} {sc.scheduleDesc ? `· ${sc.scheduleDesc}` : ''}
                    </option>
                  ))}
                </select>
                {scheduleId && (
                  <select
                    className="w-full p-1.5 rounded bg-gray-800 border border-gray-700 text-gray-200"
                    value={scheduleTaskId}
                    aria-label={t('dagx.periodicTask')}
                    onChange={(e) => {
                      setScheduleTaskId(e.target.value);
                      setPage(1);
                    }}
                  >
                    <option value="">{t('dagx.periodicTask')}</option>
                    {(tasksQuery.data?.list || []).map((task) => (
                      <option key={task.scheduleTaskId} value={task.scheduleTaskId}>
                        {task.scheduleTaskId} · {task.scheduleTaskStatus} · {task.scheduleTaskExpectStartTime || ''}
                      </option>
                    ))}
                  </select>
                )}
              </div>
            )}
          </div>
        )}
        <ExecutionTimeline
          records={list.map(toExecutionRecord)}
          loading={recordsQuery.isLoading}
          pagination={{ current: page, pageSize: PAGE_SIZE, total }}
          onPageChange={setPage}
          selectedJobId={replay?.jobId}
          labels={{ title: t('dagx.records'), empty: t('common.empty'), loading: t('common.loading'), stop: t('dag.stop') }}
          onSelect={(jobId) => setReplay(list.find((j) => j.jobId === jobId) ?? { jobId })}
          onStop={readOnly ? undefined : (jobId) => stopMutation.mutate(jobId)}
        />
      </div>
      <Modal isOpen={!!replay} onClose={() => setReplay(null)} width="max-w-6xl" title={t('dagx.recordReplay')} footer={<Button onClick={() => setReplay(null)}>{t('common.close')}</Button>}>
        {replay?.jobId && <JobReplayView projectId={projectId} jobId={replay.jobId} summary={replay} computeMode={computeMode} height="60vh" />}
      </Modal>
      <Tour
        open={tourOpen || recordTour.open}
        onClose={() => {
          setTourOpen(false);
          recordTour.close();
        }}
        steps={[{ target: '[data-tour="dag-records-list"]', title: t('dagx.tour.recordList.title'), content: t('dagx.tour.recordList.desc') }]}
        labels={{ next: t('common.next'), prev: t('common.previous'), finish: t('common.close'), skip: t('common.cancel') }}
      />
    </div>
  );
};
