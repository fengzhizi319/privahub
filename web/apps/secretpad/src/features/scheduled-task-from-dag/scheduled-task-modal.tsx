/**
 * DAG 内周期任务创建组件。
 *
 * 旧前端对应 `main-dag/periodic-task-entry`：在 DAG 画布中针对当前图
 * 直接创建 Quartz 周期任务，无需跳转到独立的周期任务页面。
 *
 * 实现要点：
 * 1. 复用 `apiClient.createScheduledGraph`（`POST /api/v1alpha1/scheduled/graph/create`）。
 * 2. 默认选中当前图中全部节点；允许用户调整 Cron 表达式与任务名称。
 * 3. 后端要求图至少成功运行过一次（`getScheduledOnceSuccess`），创建前做校验，
 *    避免对未运行过的图创建周期任务时报错。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Modal, toast } from '@secretpad/design-system';
import type { DAGNode } from '@secretpad/dag-next';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

interface ScheduledTaskFromDagModalProps {
  isOpen: boolean;
  onClose: () => void;
  projectId: string;
  graphId: string;
  graphName?: string;
  nodes: DAGNode[];
}

export const ScheduledTaskFromDagModal: React.FC<ScheduledTaskFromDagModalProps> = ({
  isOpen,
  onClose,
  projectId,
  graphId,
  graphName,
  nodes,
}) => {
  const { t } = useTranslation();
  const [scheduleDesc, setScheduleDesc] = useState('');
  const [cron, setCron] = useState('0 0 2 * * ?');
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);

  useEffect(() => {
    if (isOpen) {
      setScheduleDesc(graphName ? `${graphName}_schedule` : '');
      setCron('0 0 2 * * ?');
      setSelectedNodeIds(nodes.map((n) => n.id));
    }
  }, [isOpen, graphName, nodes]);

  const onceSuccessQuery = useQuery({
    queryKey: ['scheduled-once-success', projectId, graphId],
    queryFn: () => apiClient.getScheduledOnceSuccess(projectId, graphId),
    enabled: isOpen && !!projectId && !!graphId,
  });

  const createMutation = useMutation({
    mutationFn: () =>
      apiClient.createScheduledGraph({
        projectId,
        graphId,
        nodes: selectedNodeIds,
        scheduleDesc: scheduleDesc || undefined,
        cron: { scheduleTime: cron },
      }),
    onSuccess: () => {
      toast.success(t('periodicTasks.createSuccess'));
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const allNodeIds = useMemo(() => nodes.map((n) => n.id), [nodes]);
  const allSelected = selectedNodeIds.length === allNodeIds.length;

  const toggleAll = () => {
    setSelectedNodeIds(allSelected ? [] : allNodeIds);
  };

  const toggleNode = (id: string) => {
    setSelectedNodeIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  const isValid = !!cron && selectedNodeIds.length > 0;
  const onceSuccess = onceSuccessQuery.data ?? false;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('dag.createPeriodicTask')}
      width="max-w-lg"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => createMutation.mutate()}
            loading={createMutation.isPending || onceSuccessQuery.isLoading}
            disabled={!isValid || !onceSuccess}
          >
            {t('common.create')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        {!onceSuccess && !onceSuccessQuery.isLoading && (
          <div className="p-2.5 rounded-lg bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 text-amber-700 dark:text-amber-400">
            {t('dag.periodicTaskOnceSuccessHint')}
          </div>
        )}

        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('periodicTasks.name')}
          </label>
          <input
            type="text"
            value={scheduleDesc}
            onChange={(e) => setScheduleDesc(e.target.value)}
            placeholder={graphName ? `${graphName}_schedule` : ''}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          />
        </div>

        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('periodicTasks.cronLabel')}
          </label>
          <input
            type="text"
            value={cron}
            onChange={(e) => setCron(e.target.value)}
            placeholder={t('periodicTasks.cronPlaceholder')}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
          />
        </div>

        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="block font-semibold text-gray-700 dark:text-gray-300">
              {t('dag.periodicTaskNodes')}
            </label>
            <button
              type="button"
              onClick={toggleAll}
              className="text-blue-600 dark:text-blue-400 hover:underline"
            >
              {allSelected ? t('dag.periodicTaskDeselectAll') : t('dag.periodicTaskSelectAll')}
            </button>
          </div>
          <div className="max-h-48 overflow-y-auto space-y-1 p-2 rounded-lg border border-gray-200 dark:border-gray-700">
            {nodes.length === 0 && (
              <div className="text-gray-400 text-center py-2">{t('dag.emptyCanvas')}</div>
            )}
            {nodes.map((node) => (
              <label
                key={node.id}
                className="flex items-center gap-2 p-1.5 rounded hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
              >
                <input
                  type="checkbox"
                  checked={selectedNodeIds.includes(node.id)}
                  onChange={() => toggleNode(node.id)}
                  className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-gray-900 dark:text-gray-100">
                  {node.name} ({node.codeName || node.id})
                </span>
              </label>
            ))}
          </div>
        </div>
      </div>
    </Modal>
  );
};

ScheduledTaskFromDagModal.displayName = 'ScheduledTaskFromDagModal';
