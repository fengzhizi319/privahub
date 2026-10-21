/**
 * DAG 全局（高级）配置（旧版 advanced-config-drawer）：
 * - 最大并行度 maxParallelism；
 * - 各参与方默认存储数据源（project/datasource/list），随 graph/update 的 dataSourceConfig 保存。
 */
import React, { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button, Modal } from '@secretpad/design-system';
import type { GraphDataSourceConfigJava } from '@secretpad/api-client';
import { listProjectGraphDatasourcesJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

export interface AdvancedConfigValue {
  maxParallelism?: number;
  dataSourceConfig: GraphDataSourceConfigJava[];
}

export const AdvancedConfigModal: React.FC<{
  open: boolean;
  projectId: string;
  initial: AdvancedConfigValue;
  readOnly?: boolean;
  saving?: boolean;
  onClose: () => void;
  onSave: (value: AdvancedConfigValue) => void | Promise<void>;
}> = ({ open, projectId, initial, readOnly, saving, onClose, onSave }) => {
  const { t } = useTranslation();
  const [maxParallelism, setMaxParallelism] = useState<number>(initial.maxParallelism ?? 1);
  const [sources, setSources] = useState<Record<string, string>>({});
  const dsQuery = useQuery({
    queryKey: ['project-graph-datasources', projectId],
    queryFn: () => listProjectGraphDatasourcesJava(projectId),
    enabled: open && !!projectId,
  });

  useEffect(() => {
    if (!open) return;
    setMaxParallelism(initial.maxParallelism ?? 1);
    const map: Record<string, string> = {};
    (initial.dataSourceConfig || []).forEach((c) => {
      if (c.nodeId && c.dataSourceId) map[c.nodeId] = c.dataSourceId;
    });
    setSources(map);
  }, [open, initial]);

  const nodes = dsQuery.data ?? [];
  const valid = Number.isInteger(maxParallelism) && maxParallelism >= 1 && maxParallelism <= 10;

  return (
    <Modal
      isOpen={open}
      onClose={onClose}
      title={t('dagx.advancedConfig')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          {!readOnly && (
            <Button
              variant="primary"
              loading={saving}
              disabled={!valid}
              onClick={() =>
                void onSave({
                  maxParallelism,
                  dataSourceConfig: nodes.map((n) => ({
                    nodeId: n.nodeId,
                    dataSourceId: sources[n.nodeId] || n.dataSources[0]?.dataSourceId,
                  })),
                })
              }
            >
              {t('common.save')}
            </Button>
          )}
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <div>
          <label className="block font-semibold mb-1" htmlFor="dag-max-parallelism">
            {t('dagx.maxParallelism')}
          </label>
          <input
            id="dag-max-parallelism"
            type="number"
            min={1}
            max={10}
            step={1}
            disabled={readOnly}
            value={maxParallelism}
            onChange={(e) => setMaxParallelism(Number(e.target.value))}
            className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700"
          />
          <div className={`mt-1 ${valid ? 'text-gray-500' : 'text-red-500'}`}>{t('dagx.maxParallelismHint')}</div>
        </div>
        <div>
          <div className="font-semibold mb-1">{t('dagx.defaultStorage')}</div>
          <div className="text-gray-500 mb-2">{t('dagx.defaultStorageHint')}</div>
          {dsQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
          <div className="space-y-2">
            {nodes.map((n) => {
              const locked = initial.dataSourceConfig.find((c) => c.nodeId === n.nodeId)?.editEnable === false;
              return (
              <label key={n.nodeId} className="flex items-center gap-2">
                <span className="w-28 truncate">{n.nodeName || n.nodeId}</span>
                <select
                  aria-label={n.nodeName || n.nodeId}
                  disabled={readOnly || locked}
                  value={sources[n.nodeId] || n.dataSources[0]?.dataSourceId || ''}
                  onChange={(e) => setSources((prev) => ({ ...prev, [n.nodeId]: e.target.value }))}
                  className="flex-1 p-1.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700"
                >
                  {n.dataSources.map((d) => (
                    <option key={d.dataSourceId} value={d.dataSourceId}>
                      {d.dataSourceName || d.dataSourceId}
                      {d.type ? ` (${d.type})` : ''}
                    </option>
                  ))}
                </select>
              </label>
              );
            })}
          </div>
        </div>
      </div>
    </Modal>
  );
};
