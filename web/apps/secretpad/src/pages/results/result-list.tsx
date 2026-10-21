import { QueryFeedback } from '../../shared/ui/query-feedback';
import { FilterBar } from '../../shared/ui/filter-bar';
import { RowActions } from '../../shared/ui/row-actions';
import { TableView } from '../../shared/ui/table-view';
/**
 * 结果管理列表部分（legacy result-manager.view）：
 * - node/result/list 服务端分页；名称搜索、类型筛选、时间排序；
 * - 节点选择：AUTONOMY 在机构节点中筛选（nodeNamesFilter），CENTER 管理员选择节点（ownerId），
 *   节点上下文中固定为该节点；
 * - 批量下载 / 批量删除（表类型结果，datatable/delete）；
 * - TEE：拉取中轮询，拉取失败“重新获取”，未拉取的 TEE 结果可“申请下载”（approval TEE_DOWNLOAD）。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearch } from '@tanstack/react-router';
import { Badge, Button, Card, CheckboxGroup, ConfirmDialog, Input, Pagination, Select, toast } from '@secretpad/design-system';
import {
  apiClient,
  deleteResultTableJava,
  listMyInstNodesJava,
  listNodeResultsJava,
  toTeeResourceType,
  type NodeAllResultsVO,
  type NodeResultJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { useInstNodeIds, usePlatform } from '@/shared/lib/platform';
import { useNodeContext } from '@/shared/lib/use-node-context';
import { useDebounced } from '@/shared/lib/use-debounced';
import { errorText, formatTime } from '@/features/cooperative-node/format';
import { reportCsvBlob, type ReportTab } from './report-csv';
import { TeeApplyModal } from './tee-apply-modal';

const KINDS = ['table', 'report', 'rule', 'model'];
const REMOTE_SOURCES = ['OSS', 'ODPS', 'MYSQL'];

const rowKey = (r: NodeResultJava) => `${r.nodeId}::${r.domainDataId}`;

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export const ResultList: React.FC<{ onOpenDetail: (result: NodeAllResultsVO) => void }> = ({ onOpenDetail }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const platform = usePlatform();
  const instNodeIds = useInstNodeIds();
  const ctx = useNodeContext();

  // Legacy deep link `/results?ownerId=&resultName=` (e.g. from a DAG output).
  const deepLink = useSearch({ strict: false }) as { ownerId?: string; resultName?: string };
  const [search, setSearch] = useState(deepLink.resultName || '');
  const debounced = useDebounced(search);
  const [kinds, setKinds] = useState<string[]>([]);
  const [sortRule, setSortRule] = useState<'descending' | 'ascending'>('descending');
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  const [pickedNode, setPickedNode] = useState(deepLink.ownerId || '');
  const [instFilter, setInstFilter] = useState<string[]>([]);
  const [selected, setSelected] = useState<Record<string, NodeResultJava>>({});
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [teeTarget, setTeeTarget] = useState<NodeResultJava | null>(null);

  const needsPicker = !ctx.nodeId && platform.isCenterAdmin;
  const pickerQuery = useQuery({ queryKey: ['results-picker-nodes'], queryFn: () => apiClient.getNodes(), enabled: needsPicker });
  useEffect(() => {
    if (needsPicker && !pickedNode && pickerQuery.data?.length) setPickedNode(pickerQuery.data[0].nodeId);
  }, [needsPicker, pickedNode, pickerQuery.data]);

  const instNodesQuery = useQuery({
    queryKey: ['results-inst-nodes'],
    queryFn: listMyInstNodesJava,
    enabled: platform.isAutonomy && !ctx.nodeId,
  });
  const instOptions = (instNodesQuery.data || [])
    .filter((n) => n.nodeStatus === 'Ready')
    .map((n) => ({ value: n.nodeId || '', label: n.nodeName || n.nodeId || '' }));

  const ownerId = ctx.nodeId || (needsPicker ? pickedNode : platform.ownerId);
  const showNodeCol = platform.isAutonomy && !ctx.nodeId;
  const showPullStatus = !platform.isAutonomy && ownerId !== 'tee';

  useEffect(() => {
    setPage(1);
    setSelected({});
  }, [debounced, kinds, sortRule, ownerId, instFilter, size]);

  const listQuery = useQuery({
    queryKey: ['result-list', ownerId, page, size, debounced, kinds, sortRule, instFilter],
    queryFn: () =>
      listNodeResultsJava({
        ownerId,
        pageNumber: page,
        pageSize: size,
        nameFilter: debounced,
        kindFilters: kinds,
        timeSortingRule: sortRule,
        nodeNamesFilter: instFilter.length ? instFilter : null,
      }),
    enabled: !!ownerId,
    placeholderData: keepPreviousData,
    // Poll while results are being pulled from TEE (legacy: every 2s).
    refetchInterval: (q) => (q.state.data?.list.some((r) => r.pullFromTeeStatus === 'RUNNING') ? 2000 : false),
  });
  const rows = useMemo(() => listQuery.data?.list ?? [], [listQuery.data]);
  const total = listQuery.data?.total ?? 0;
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['result-list'] });

  const nodeOf = (r: NodeResultJava) => (platform.isAutonomy ? r.nodeId : ownerId) || r.nodeId || '';
  const canDownload = (r: NodeResultJava) =>
    !!r.domainDataId && !REMOTE_SOURCES.includes((r.datasourceType || '').toUpperCase()) && r.pullFromTeeStatus !== 'RUNNING';

  const download = async (r: NodeResultJava) => {
    const nodeId = nodeOf(r);
    if (!nodeId || !r.domainDataId) return;
    if (r.datatableType === 'report') {
      const detail = await apiClient.getNodeResultDetail({ nodeId, domainDataId: r.domainDataId });
      const tabs = (detail.output as { tabs?: ReportTab[] } | undefined)?.tabs;
      if (!tabs?.length) {
        toast.warning(t('results.reportEmpty'));
        return;
      }
      saveBlob(reportCsvBlob(tabs), `${r.domainDataId}.csv`);
      return;
    }
    const blob = await apiClient.downloadData({ nodeId, domainDataId: r.domainDataId });
    saveBlob(new Blob(['﻿', blob], { type: 'text/plain;charset=utf-8' }), r.domainDataId);
  };

  const downloadOne = useMutation({
    mutationFn: download,
    onMutate: () => toast.info(t('results.downloadStart')),
    onSuccess: () => {
      toast.success(t('results.downloadSuccess'));
      invalidate();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const selectedRows = Object.values(selected);
  const batchDownload = useMutation({
    mutationFn: async () => {
      const targets = selectedRows.filter((r) => canDownload(r) || r.datatableType === 'report');
      for (const r of targets) await download(r);
      return targets.length;
    },
    onSuccess: (n) => toast.success(t('results.batchDownloaded', { count: n })),
    onError: (e) => toast.error(errorText(e)),
  });
  const deletable = selectedRows.filter((r) => (r.datatableType || 'table') === 'table');
  const batchDelete = useMutation({
    mutationFn: async () => {
      for (const r of deletable) {
        await deleteResultTableJava({
          nodeId: nodeOf(r),
          datatableId: r.domainDataId!,
          datasourceId: r.datasourceId,
          relativeUri: r.relativeUri,
        });
      }
      return deletable.length;
    },
    onSuccess: (n) => {
      toast.success(t('results.batchDeleted', { count: n }));
      setConfirmDelete(false);
      setSelected({});
      invalidate();
    },
    onError: (e) => {
      setConfirmDelete(false);
      toast.error(errorText(e));
      invalidate();
    },
  });

  const allChecked = rows.length > 0 && rows.every((r) => selected[rowKey(r)]);
  const toggleAll = () =>
    setSelected((s) => {
      const next = { ...s };
      rows.forEach((r) => (allChecked ? delete next[rowKey(r)] : (next[rowKey(r)] = r)));
      return next;
    });
  const toggle = (r: NodeResultJava) =>
    setSelected((s) => {
      const next = { ...s };
      if (next[rowKey(r)]) delete next[rowKey(r)];
      else next[rowKey(r)] = r;
      return next;
    });

  const openDetail = (r: NodeResultJava) => {
    const { nodeId, nodeName, ...vo } = r;
    onOpenDetail({ nodeId: nodeOf(r) || nodeId, nodeName, nodeResultsVO: vo } as NodeAllResultsVO);
  };

  const renderActions = (r: NodeResultJava) => {
    const detailBtn = (
      <Button variant="ghost" size="sm" onClick={() => openDetail(r)}>
        {t('common.detail')}
      </Button>
    );
    if (ownerId === 'tee') return detailBtn;
    if (r.datatableType === 'report') {
      return (
        <>
          {detailBtn}
          <Button variant="outline" size="sm" onClick={() => downloadOne.mutate(r)}>
            {t('common.download')}
          </Button>
        </>
      );
    }
    if (r.pullFromTeeStatus === 'RUNNING') return <span className="text-gray-400">{t('results.pulling')}</span>;
    if (r.pullFromTeeStatus === 'FAILED') {
      return (
        <>
          {detailBtn}
          <Button variant="outline" size="sm" title={r.pullFromTeeErrMsg} onClick={() => downloadOne.mutate(r)}>
            {t('results.retry')}
          </Button>
        </>
      );
    }
    const remote = REMOTE_SOURCES.includes((r.datasourceType || '').toUpperCase());
    return (
      <>
        {detailBtn}
        <span title={remote ? t('results.remoteNoDownload', { type: r.datasourceType || '', uri: r.relativeUri || '' }) : undefined}>
          <Button variant="outline" size="sm" disabled={!canDownload(r)} onClick={() => downloadOne.mutate(r)}>
            {t('common.download')}
          </Button>
        </span>
        {platform.supportsTee && (r.computeMode || '').toUpperCase() === 'TEE' && !r.pullFromTeeStatus && r.jobId && !!toTeeResourceType(r.datatableType) && (
          <Button variant="ghost" size="sm" onClick={() => setTeeTarget(r)}>
            {t('results.teeApply')}
          </Button>
        )}
      </>
    );
  };

  const cols = 7 + (showNodeCol ? 1 : 0) + (showPullStatus ? 1 : 0);

  return (
    <Card data-tour="results-list">
      <FilterBar onReset={()=>{setSearch('');setKinds([]);setInstFilter([]);setPage(1);}}>
        {needsPicker && (
          <Select
            className="md:!w-48"
            value={pickedNode}
            onChange={setPickedNode}
            options={(pickerQuery.data || []).map((n) => ({ value: n.nodeId, label: n.nodeName || n.nodeId }))}
          />
        )}
        <Input className="flex-1" value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('results.searchPlaceholder')} />
        <Select
          className="md:!w-40"
          value={sortRule}
          onChange={(v) => setSortRule(v as 'descending' | 'ascending')}
          options={[
            { value: 'descending', label: t('results.sortDesc') },
            { value: 'ascending', label: t('results.sortAsc') },
          ]}
        />
      </FilterBar>
      <div className="flex flex-wrap items-center gap-3 mb-3 text-xs">
        <span className="text-gray-500">{t('results.kindHeader')}:</span>
        <CheckboxGroup value={kinds} onChange={setKinds} options={KINDS.map((k) => ({ value: k, label: t(`results.kind.${k}`) }))} />
        {showNodeCol && instOptions.length > 0 && (
          <>
            <span className="text-gray-500 ml-2">{t('results.node')}:</span>
            <CheckboxGroup value={instFilter} onChange={setInstFilter} options={instOptions} />
          </>
        )}
      </div>

      {selectedRows.length > 0 && (
        <div className="flex items-center gap-3 mb-3 px-3 py-2 rounded-lg bg-blue-50 dark:bg-blue-950/30 text-xs">
          <span>{t('results.selectedCount', { count: selectedRows.length })}</span>
          <Button size="sm" variant="outline" loading={batchDownload.isPending} onClick={() => batchDownload.mutate()}>
            {t('results.batchDownload')}
          </Button>
          {platform.canWriteNode(ctx.nodeId || (needsPicker ? pickedNode : undefined), { instNodeIds }) && (
            <span title={deletable.length === 0 ? t('results.batchDeleteTableOnly') : undefined}>
              <Button size="sm" variant="danger" disabled={deletable.length === 0} onClick={() => setConfirmDelete(true)}>
                {t('results.batchDelete')}
              </Button>
            </span>
          )}
          <Button size="sm" variant="ghost" onClick={() => setSelected({})}>
            {t('results.clearSelection')}
          </Button>
        </div>
      )}

      {listQuery.error && <QueryFeedback error={listQuery.error} onRetry={()=>void listQuery.refetch()}/>}

      <div className="overflow-x-auto">
        <TableView id="results"><table className="w-full text-xs border-collapse">
          <thead>
            <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-gray-500">
              <th className="py-2 px-3 w-8">
                <input type="checkbox" aria-label="select all" checked={allChecked} onChange={toggleAll} />
              </th>
              <th className="py-2 px-3 font-semibold">{t('results.name')}</th>
              {showNodeCol && <th className="py-2 px-3 font-semibold">{t('results.node')}</th>}
              <th className="py-2 px-3 font-semibold">{t('results.kindHeader')}</th>
              <th className="py-2 px-3 font-semibold">{t('results.project')}</th>
              <th className="py-2 px-3 font-semibold">{t('results.mode')}</th>
              {showPullStatus && <th className="py-2 px-3 font-semibold">{t('results.status')}</th>}
              <th className="py-2 px-3 font-semibold">{t('results.createTime')}</th>
              <th className="py-2 px-3 font-semibold text-right">{t('common.action')}</th>
            </tr>
          </thead>
          <tbody>
            {listQuery.isLoading && (
              <tr>
                <td colSpan={cols} className="py-8 text-center text-gray-400">
                  <QueryFeedback loading/>
                </td>
              </tr>
            )}
            {!listQuery.isLoading && !listQuery.error && rows.length === 0 && (
              <tr>
                <td colSpan={cols} className="py-8 text-center text-gray-400">
                  <QueryFeedback empty label={t('results.noData')}/>
                </td>
              </tr>
            )}
            {rows.map((r) => {
              const status = r.pullFromTeeStatus;
              return (
                <tr key={rowKey(r)} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-800/50">
                  <td className="py-2 px-3">
                    <input type="checkbox" checked={!!selected[rowKey(r)]} onChange={() => toggle(r)} aria-label="select" />
                  </td>
                  <td className="py-2 px-3">
                    <button type="button" className="font-medium text-blue-600 dark:text-blue-400 hover:underline text-left" onClick={() => openDetail(r)}>
                      {r.productName || r.domainDataId || '-'}
                    </button>
                    <div className="font-mono text-[10px] text-gray-400">{r.domainDataId}</div>
                  </td>
                  {showNodeCol && <td className="py-2 px-3">{r.nodeName || r.nodeId || '-'}</td>}
                  <td className="py-2 px-3">{t(`results.kind.${r.datatableType || 'table'}`)}</td>
                  <td className="py-2 px-3">{r.sourceProjectName || r.sourceProjectId || '-'}</td>
                  <td className="py-2 px-3">{r.computeMode || '-'}</td>
                  {showPullStatus && (
                    <td className="py-2 px-3">
                      {status ? (
                        <span title={r.pullFromTeeErrMsg}>
                          <Badge status={status === 'SUCCESS' ? 'success' : status === 'FAILED' ? 'error' : 'processing'}>
                            {t(`results.pull.${status}`)}
                          </Badge>
                        </span>
                      ) : (
                        '-'
                      )}
                    </td>
                  )}
                  <td className="py-2 px-3">{formatTime(r.gmtCreate)}</td>
                  <td className="py-2 px-3">
                    <RowActions>{renderActions(r)}</RowActions>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table></TableView>
      </div>
      <div className="flex items-center justify-end gap-3 mt-3">
        <Select
          className="!w-24"
          value={String(size)}
          onChange={(v) => setSize(Number(v))}
          options={[10, 20, 50].map((n) => ({ value: String(n), label: `${n}/page` }))}
        />
        <Pagination page={page} pageSize={size} total={total} onChange={setPage} />
      </div>

      <ConfirmDialog
        isOpen={confirmDelete}
        title={t('results.batchDelete')}
        message={t('results.batchDeleteConfirm', { count: deletable.length })}
        danger
        loading={batchDelete.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => batchDelete.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
      <TeeApplyModal result={teeTarget} onClose={() => setTeeTarget(null)} />
    </Card>
  );
};
