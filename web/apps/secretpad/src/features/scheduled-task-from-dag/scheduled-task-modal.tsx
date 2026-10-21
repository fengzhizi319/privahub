/**
 * DAG 内周期任务创建组件。
 *
 * 旧前端对应 `main-dag/periodic-task-entry` + `periodic-task-drawer`：在 DAG
 * 画布中针对当前图直接部署周期任务。
 *
 * 实现要点：
 * 1. 打开时预取 `scheduled/id`（任务 ID）并检查 `scheduled/graph/once/success`
 *    （画布需至少成功跑通一次才可部署）。
 * 2. 结构化调度表单（周期 D/W/M、指定日期、指定时刻、调度时间范围）由
 *    `features/schedule-form` 提供，校验规则与旧版一致。
 * 3. 默认选中当前图中全部节点（旧版固定为全部节点），允许调整。
 */
import React, { useEffect, useMemo, useState } from 'react';
import type { DAGNode } from '@secretpad/dag-next';
import { FormField } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';
import { ScheduleCreateDialog } from '../schedule-form';

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
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);

  useEffect(() => {
    if (isOpen) setSelectedNodeIds(nodes.map((n) => n.id));
  }, [isOpen, nodes]);

  const allNodeIds = useMemo(() => nodes.map((n) => n.id), [nodes]);
  const allSelected = selectedNodeIds.length === allNodeIds.length;

  const toggleNode = (id: string) => {
    setSelectedNodeIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  return (
    <ScheduleCreateDialog
      isOpen={isOpen}
      onClose={onClose}
      projectId={projectId}
      graphId={graphId}
      nodeIds={selectedNodeIds}
      title={graphName ? `${t('scheduleForm.title')} · ${graphName}` : t('scheduleForm.title')}
    >
      <FormField
        label={
          <span className="flex items-center justify-between w-full">
            <span>{t('dag.periodicTaskNodes')}</span>
            <button
              type="button"
              onClick={() => setSelectedNodeIds(allSelected ? [] : allNodeIds)}
              className="text-blue-600 dark:text-blue-400 hover:underline font-normal"
            >
              {allSelected ? t('dag.periodicTaskDeselectAll') : t('dag.periodicTaskSelectAll')}
            </button>
          </span>
        }
        required
        error={selectedNodeIds.length === 0 ? t('scheduleForm.errors.nodesRequired') : undefined}
      >
        <div className="max-h-40 overflow-y-auto space-y-1 p-2 rounded-lg border border-gray-200 dark:border-gray-700">
          {nodes.length === 0 && <div className="text-gray-400 text-center py-2">{t('dag.emptyCanvas')}</div>}
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
      </FormField>
    </ScheduleCreateDialog>
  );
};

ScheduledTaskFromDagModal.displayName = 'ScheduledTaskFromDagModal';
