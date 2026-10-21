/**
 * 结果详情（旧版 result-details/result-details-drawer.tsx）：
 * - 描述：结果类型、来源项目、所属训练流、生成时间、路径、任务 ID（可复制）；
 * - TEE 拉取状态（获取中 / 成功 / 失败）与“重新获取”；
 * - 血缘：训练流 DAG 预览（高亮产出节点及其上游）+ 全屏只读画布 + 上游血缘树；
 * - 报告：按 sf.report 渲染（与 DAG 输出同一渲染器）；
 * - 表：字段表，数据分类分级结果表标注 L1~L5 / 需复核 / 命中标签字段。
 */
import React, { useMemo, useState } from 'react';
import { Badge, Button, Modal, Tabs, toast } from '@secretpad/design-system';
import type { NodeResultDetailVO, NodeAllResultsVO } from '@secretpad/api-client';
import { apiClient, normalizeGraphDetail } from '@secretpad/api-client';
import { CLASSIFICATION_COLUMNS, ChartFrame, DAGNextWorkspace, ReportTabsView, downloadDisabledReason, formatTimestamp, isClassificationTable, parseReportTabs } from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';
import { DagPreview } from '../../features/lineage/dag-preview';
import { lineageIds, producingNodeId } from '../../features/lineage/lineage-utils';
import { LineageTree } from '../../features/lineage/lineage-tree';
import type { LineageNode } from '../../features/lineage/lineage-tree';
import { mapGraphToDAG } from '../dag/adapters';
import { triggerBlobDownload } from '../../features/job-detail';
import { reportCsvBlob, type ReportTab as CsvReportTab } from './report-csv';

function buildLineageTree(nodeId: string, nodes: Array<{ id: string; name: string; codeName?: string }>, edges: Array<{ source: string; target: string }>, seen = new Set<string>()): LineageNode {
  const n = nodes.find((x) => x.id === nodeId);
  seen.add(nodeId);
  const parents = edges.filter((e) => e.target === nodeId && !seen.has(e.source)).map((e) => e.source);
  return {
    id: nodeId,
    icon: n?.codeName?.startsWith('read_data') ? '🗂️' : '⚙️',
    title: n?.name || nodeId,
    subtitle: n?.codeName,
    children: parents.map((p) => buildLineageTree(p, nodes, edges, seen)),
  };
}

export const ResultDetailView: React.FC<{ detail: NodeResultDetailVO; result: NodeAllResultsVO | null }> = ({ detail, result }) => {
  const { t } = useTranslation();
  const [activeTab,setActiveTab] = useState('content');
  const [fullscreen, setFullscreen] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const vo = detail.nodeResultsVO || {};
  const nodeId = result?.nodeId || '';
  const columns = detail.tableColumnVOList || [];
  const classification = isClassificationTable(columns);
  const graph = useMemo(() => (detail.graphDetailVO ? normalizeGraphDetail(detail.graphDetailVO) : null), [detail.graphDetailVO]);
  const dag = useMemo(() => mapGraphToDAG(graph), [graph]);
  const focusId = graph ? producingNodeId(graph.nodes, vo.domainDataId) : undefined;
  const highlight = useMemo(() => (focusId ? lineageIds(focusId, dag.edges) : new Set<string>()), [focusId, dag.edges]);
  const tabs = parseReportTabs(detail.output?.tabs);
  const codeName = detail.output?.codeName || '';
  const disabledReason = downloadDisabledReason(vo.datasourceType, vo.relativeUri);
  const isTee = vo.computeMode === 'TEE';

  /** 报告下载：与结果列表相同的 CSV 导出（旧 result-details 报告 CSV 下载）。 */
  const downloadReport = () => {
    if (tabs.length === 0) {
      toast.warning(t('results.reportEmpty'));
      return;
    }
    triggerBlobDownload(reportCsvBlob(tabs as unknown as CsvReportTab[]), `${vo.domainDataId || vo.productName || 'report'}.csv`);
  };

  const download = async () => {
    if (!nodeId || !vo.domainDataId) return;
    setRetrying(true);
    try {
      const blob = await apiClient.downloadData({ nodeId, domainDataId: vo.domainDataId });
      triggerBlobDownload(blob, vo.productName || vo.domainDataId);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setRetrying(false);
    }
  };

  const teeBadge = () => {
    if (!isTee) return null;
    switch (vo.pullFromTeeStatus) {
      case 'SUCCESS':
        return <Badge status="success">{t('results.fetchSuccess')}</Badge>;
      case 'RUNNING':
        return <Badge status="processing">{t('results.fetching')}</Badge>;
      case 'FAILED':
        return <Badge status="error">{t('results.fetchFailed')}</Badge>;
      default:
        return null;
    }
  };

  return (
    <div className="space-y-4 text-xs">
      <div className="flex items-center gap-3">
        <span className="font-semibold text-sm">「{vo.productName || vo.domainDataId}」</span>
        {teeBadge()}
        {vo.pullFromTeeErrMsg && <span className="text-red-500 break-all">{vo.pullFromTeeErrMsg}</span>}
      </div>
      <Tabs activeKey={activeTab} onChange={setActiveTab} items={[{key:'basics',label:t('display.basics')},{key:'content',label:t('display.resultContent')},{key:'lineage',label:t('dagx.lineage')}]}/>
      <div hidden={activeTab !== 'basics'} className="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5">
        <div>
          <span className="text-gray-500">{t('results.resultType')}: </span>
          {t(`results.kind.${vo.datatableType || 'table'}`)}
        </div>
        <div>
          <span className="text-gray-500">{t('results.sourceProject')}: </span>
          {vo.sourceProjectName || vo.sourceProjectId || '-'}
        </div>
        <div>
          <span className="text-gray-500">{t('results.trainFlow')}: </span>
          {vo.trainFlow || graph?.name || '-'}
        </div>
        <div>
          <span className="text-gray-500">{t('results.generatedAt')}: </span>
          {formatTimestamp(vo.gmtCreate) || '-'}
        </div>
        <div className="col-span-2 break-all">
          <span className="text-gray-500">{t('results.path')}: </span>
          {vo.relativeUri || '-'}
        </div>
        <div className="col-span-2 break-all flex items-center gap-2">
          <span className="text-gray-500">{t('results.jobId')}: </span>
          <span className="font-mono">{vo.jobId || '-'}</span>
          {vo.jobId && (
            <button type="button" className="text-blue-500 hover:underline" onClick={() => void navigator.clipboard?.writeText(vo.jobId || '')}>
              {t('dagx.copy')}
            </button>
          )}
        </div>
      </div>

      <div hidden={activeTab !== 'lineage'}>
      {graph && dag.nodes.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="font-semibold">{t('dagx.lineage')}</span>
            <Button size="sm" variant="ghost" onClick={() => setFullscreen(true)}>
              ⛶ {t('common.fullscreen')}
            </Button>
          </div>
          <DagPreview nodes={dag.nodes} edges={dag.edges} highlight={highlight.size ? highlight : new Set(dag.nodes.map((n) => n.id))} focusId={focusId} />
          {focusId && (
            <div className="overflow-x-auto">
              <LineageTree root={buildLineageTree(focusId, dag.nodes, dag.edges)} />
            </div>
          )}
        </div>
      )}

      </div>
      <div hidden={activeTab !== 'content'} className="space-y-4">
      {vo.datatableType === 'report' && (
        <div className="space-y-2">
          <div className="font-semibold">{t('results.reportTitle')}</div>
          {tabs.length > 0 && codeName ? <ReportTabsView tabs={tabs} codeName={codeName} id={vo.domainDataId || ''} /> : <div className="text-gray-400">{t('results.noResult')}</div>}
        </div>
      )}

      {vo.datatableType !== 'report' && columns.length > 0 && (
        <div className="space-y-2">
          <div className="font-semibold">{t('results.tableColumns')}</div>
          {classification && (
            <div className="p-2 rounded-lg bg-blue-50 dark:bg-blue-950/30 border border-blue-200 dark:border-blue-900 text-blue-700 dark:text-blue-300">
              {t('results.classificationNote')}
            </div>
          )}
          <ChartFrame labels={{ fullscreen: t('common.fullscreen'), exitFullscreen: t('results.exitFullscreen') }}>
            {(full) => (
              <div className={`${full ? '' : 'max-h-72'} overflow-auto rounded-lg border border-gray-200 dark:border-gray-700`}>
                <table className="w-full">
                  <thead>
                    <tr className="bg-gray-50 dark:bg-gray-800 text-left">
                      <th className="px-2 py-1">{t('results.fieldName')}</th>
                      <th className="px-2 py-1">{t('results.fieldType')}</th>
                      <th className="px-2 py-1">{t('results.fieldDescription')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {columns.map((c) => (
                      <tr key={c.colName} className="border-t border-gray-100 dark:border-gray-800">
                        <td className="px-2 py-1">
                          {c.colName}
                          {classification && c.colName && CLASSIFICATION_COLUMNS[c.colName] && (
                            <span className="ml-2 px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300 text-[10px]">
                              {CLASSIFICATION_COLUMNS[c.colName]}
                            </span>
                          )}
                        </td>
                        <td className="px-2 py-1">{c.colType}</td>
                        <td className="px-2 py-1">{c.colComment}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </ChartFrame>
        </div>
      )}

      </div>
      <div className="flex justify-end gap-2">
        {vo.datatableType === 'report' && (
          <Button size="sm" variant="primary" disabled={tabs.length === 0} onClick={downloadReport}>
            {t('results.downloadReport')}
          </Button>
        )}
        {vo.datatableType !== 'report' && vo.pullFromTeeStatus === 'FAILED' && (
          <Button size="sm" variant="primary" loading={retrying} disabled={!!disabledReason} title={disabledReason} onClick={() => void download()}>
            {t('results.refetch')}
          </Button>
        )}
        {vo.datatableType !== 'report' && (vo.pullFromTeeStatus === 'SUCCESS' || !vo.pullFromTeeStatus) && (
          <Button size="sm" variant="primary" loading={retrying} disabled={!!disabledReason} title={disabledReason} onClick={() => void download()}>
            {t('results.downloadResult')}
          </Button>
        )}
      </div>

      <Modal isOpen={fullscreen} onClose={() => setFullscreen(false)} width="max-w-6xl" title={graph?.name || t('dagx.lineage')}>
        <div style={{ height: '65vh' }}>{fullscreen && <DAGNextWorkspace readOnly title={graph?.name} initialNodes={dag.nodes} initialEdges={dag.edges} />}</div>
      </Modal>
    </div>
  );
};
