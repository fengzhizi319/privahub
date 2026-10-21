/**
 * 数据表树侧栏（旧版 data-table-tree）：项目已授权数据表按参与方分组，
 * 展开可查看字段（project/datatable/get）；可拖拽到画布创建 read_data/datatable 样本表组件。
 * P2P 下本方节点显示「去添加数据」（旧版 gotoDataManagerDisabled 控制）。
 */
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import type { ProjectDetailJava } from '@secretpad/api-client';
import { getProjectDatatableColumnsJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { READ_DATA_COMPONENT } from './adapters';

const TableColumns: React.FC<{ projectId: string; nodeId: string; datatableId: string }> = ({ projectId, nodeId, datatableId }) => {
  const q = useQuery({
    queryKey: ['project-datatable-columns', projectId, nodeId, datatableId],
    queryFn: () => getProjectDatatableColumnsJava({ projectId, nodeId, datatableId }),
  });
  if (q.isLoading) return <div className="pl-6 text-gray-500">…</div>;
  return (
    <div className="pl-6 space-y-0.5">
      {(q.data || []).map((c) => (
        <div key={c.colName} className="flex justify-between text-[10px] text-gray-400">
          <span className="truncate">{c.colName}</span>
          <span className="text-gray-600">{c.colType}</span>
        </div>
      ))}
    </div>
  );
};

export const DataTableTree: React.FC<{
  projectId: string;
  project?: ProjectDetailJava | null;
  readOnly?: boolean;
  /** 显示「去添加数据」入口的节点（P2P 本方节点且项目可编辑）。 */
  manageNodeId?: string;
}> = ({ projectId, project, readOnly, manageNodeId }) => {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const nodes = project?.nodes || [];
  const total = nodes.reduce((s, n) => s + n.datatables.length, 0);
  return (
    <div className="space-y-3 text-xs text-gray-300">
      {!readOnly && total > 0 && <div className="text-[10px] text-gray-500">{t('dagx.dragTableHint')}</div>}
      {total === 0 && <div className="text-gray-500">{t('dagx.noDataTables')}</div>}
      {nodes.map((n) => (
        <div key={n.nodeId}>
          <div className="flex items-center justify-between mb-1">
            <span className="text-gray-500 font-semibold uppercase text-[10px]">{n.nodeName || n.nodeId}</span>
            {manageNodeId && n.nodeId === manageNodeId && (
              <button type="button" className="text-[10px] text-blue-400 hover:underline" onClick={() => window.open('/data-tables', '_blank', 'noopener')}>
                {t('dagx.gotoDataManager')}
              </button>
            )}
          </div>
          {n.datatables.map((tbl) => {
            const key = `${n.nodeId}/${tbl.datatableId}`;
            const open = expanded.has(key);
            return (
              <div key={key}>
                <div
                  draggable={!readOnly}
                  onDragStart={(e) => {
                    e.dataTransfer.setData(
                      'application/json',
                      JSON.stringify({ ...READ_DATA_COMPONENT, datatableId: tbl.datatableId, datatableName: tbl.datatableName, ownerId: n.nodeId }),
                    );
                  }}
                  className="p-1.5 mt-1 rounded bg-gray-900 border border-gray-800 hover:border-blue-500 flex items-center gap-1 cursor-grab"
                  title={tbl.datatableId}
                >
                  <button
                    type="button"
                    className="text-gray-500 w-3"
                    onClick={() =>
                      setExpanded((prev) => {
                        const next = new Set(prev);
                        if (next.has(key)) next.delete(key);
                        else next.add(key);
                        return next;
                      })
                    }
                  >
                    {open ? '▼' : '▶'}
                  </button>
                  <span>🗂️</span>
                  <span className="truncate">{tbl.datatableName || tbl.datatableId}</span>
                </div>
                {open && <TableColumns projectId={projectId} nodeId={n.nodeId} datatableId={tbl.datatableId} />}
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
};
