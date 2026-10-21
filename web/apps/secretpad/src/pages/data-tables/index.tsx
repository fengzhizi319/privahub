import { QueryFeedback } from '../../shared/ui/query-feedback';
import { FilterBar } from '../../shared/ui/filter-bar';
import { RowActions } from '../../shared/ui/row-actions';
import { TableView } from '../../shared/ui/table-view';
/**
 * 数据表管理页（旧前端 `data-manager` 的迁移版）。
 *
 * - 列表：`datatable/list`（Java `ListDatatableRequest`），服务端分页；表名搜索、状态、数据源类型筛选；
 * - 行操作：详情、授权管理、刷新状态（datatable/get）、删除（已授权 / 内置数据表不可删除）；
 * - 加密上传 TEE（`datatable/pushToTee`）：仅 TEE / ALL-IN-ONE 且非 P2P/AUTONOMY、非 TEE 节点，仅本地数据表；
 * - 批量管理：批量删除、批量授权到项目；
 * - 添加数据：本地上传（data/upload → data/create）或基于已注册数据源注册（datatable/create）。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Button, Card, ConfirmDialog, Input, Modal, Pagination, RadioGroup, Select, Tour, toast } from '@secretpad/design-system';
import { PAGE_TOUR_KEYS, usePageTour } from '../../features/guide-tour';
import {
  ALL_DATATABLE_SOURCE_TYPES,
  PushToTeeStatus,
  authDatatableToProjectJava,
  deleteDatatableJava,
  flattenDatatableNode,
  getDatatableJava,
  listAuthorizableProjectsJava,
  listDatatablesJava,
  pushDatatableToTeeJava,
} from '@secretpad/api-client';
import type { DatatableRowJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { useNodeScope } from '@/features/data-scope';
import { DataUploadModal } from '@/features/data-upload';
import { RegisterDatatableDrawer } from '@/features/datatable-register';
import { DatatableDetailDrawer } from './detail-drawer';
import type { DetailTab } from './detail-drawer';
import { EMBEDDED_SHEETS, canPushToTee, datatableDeleteBlock, summarizeBatch } from './auth-model';

const PAGE_SIZE = 10;
const rowKey = (r: DatatableRowJava) => `${r.datatableId}_${r.nodeId}`;

export const DataTablesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const scope = useNodeScope();
  const { ownerId, platform } = scope;
  const isAutonomy = platform.isAutonomy;

  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [page, setPage] = useState(1);
  const [statusOverride, setStatusOverride] = useState<Record<string, string | undefined>>({});

  const [batchMode, setBatchMode] = useState(false);
  const [selected, setSelected] = useState<Record<string, DatatableRowJava>>({});
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchAuthOpen, setBatchAuthOpen] = useState(false);
  const [batchProjectId, setBatchProjectId] = useState('');

  const [deleteTarget, setDeleteTarget] = useState<DatatableRowJava | null>(null);
  const [pushTarget, setPushTarget] = useState<DatatableRowJava | null>(null);
  const [detail, setDetail] = useState<{ row: DatatableRowJava; tab: DetailTab } | null>(null);
  const [addChoiceOpen, setAddChoiceOpen] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [registerOpen, setRegisterOpen] = useState(false);

  useEffect(() => {
    const id = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 300);
    return () => clearTimeout(id);
  }, [searchInput]);

  /** Node id used for per-table requests (AUTONOMY rows carry their own node). */
  const nodeOf = (r: DatatableRowJava) => (isAutonomy ? r.nodeId || ownerId : ownerId);

  const listQuery = useQuery({
    queryKey: ['datatables-java', ownerId, page, search, status, typeFilter],
    queryFn: () =>
      listDatatablesJava({
        ownerId,
        pageNumber: page,
        pageSize: PAGE_SIZE,
        statusFilter: status,
        datatableNameFilter: search,
        types: typeFilter ? [typeFilter] : null,
        nodeNamesFilter: null,
      }),
    enabled: !!ownerId,
    // Poll while a push-to-TEE is running (legacy 2s timer).
    refetchInterval: (q) =>
      (q.state.data?.datatableNodeVOList || []).some((i) => i.datatableVO?.pushToTeeStatus === PushToTeeStatus.RUNNING) ? 2000 : false,
  });
  const rows = useMemo(
    () =>
      (listQuery.data?.datatableNodeVOList || []).map((i) => {
        const r = flattenDatatableNode(i);
        const k = rowKey(r);
        return k in statusOverride ? { ...r, status: statusOverride[k] } : r;
      }),
    [listQuery.data, statusOverride],
  );
  // 旧版 data-manager Tour：有数据表时首次提示「授权」（localStorage DatatableAuthTour）。
  const authTour = usePageTour(PAGE_TOUR_KEYS.datatableAuth, rows.length > 0 && scope.canWrite);
  const total = listQuery.data?.totalDatatableNums ?? 0;

  const invalidate = () => {
    setStatusOverride({});
    return queryClient.invalidateQueries({ queryKey: ['datatables-java', ownerId] });
  };

  const showTee = platform.supportsTee && !platform.isP2p && ownerId !== 'tee' && !scope.isTeeNode;
  const selectedRows = Object.values(selected);

  /* ------------------------------ mutations ------------------------------ */

  const refreshMutation = useMutation({
    mutationFn: (r: DatatableRowJava) =>
      getDatatableJava({ nodeId: nodeOf(r), datatableId: r.datatableId, type: r.type, datasourceType: r.datasourceType }),
    onSuccess: (vo, r) => {
      setStatusOverride((prev) => ({ ...prev, [rowKey(r)]: vo.datatableVO?.status }));
      toast.success(t('dtDetail.refreshSuccess'));
    },
    onError: () => toast.error(t('dtDetail.refreshFailed')),
  });

  const deleteMutation = useMutation({
    mutationFn: (r: DatatableRowJava) =>
      deleteDatatableJava({ nodeId: nodeOf(r), datatableId: r.datatableId, type: r.type, datasourceType: r.datasourceType }),
    onSuccess: (_, r) => {
      toast.success(t('dtList.deleteSuccess', { name: r.datatableName || '' }));
      setDeleteTarget(null);
      setPage(1);
      invalidate();
    },
    onError: (e) => {
      setDeleteTarget(null);
      toast.error(e instanceof Error ? e.message : String(e));
    },
  });

  const pushMutation = useMutation({
    mutationFn: (r: DatatableRowJava) =>
      pushDatatableToTeeJava({ nodeId: nodeOf(r), datatableId: r.datatableId, datasourceId: r.datasourceId, relativeUri: r.relativeUri }),
    onSuccess: () => {
      setPushTarget(null);
      toast.success(t('dataTables.pushTeeSuccess'));
      invalidate();
    },
    onError: (e) => {
      setPushTarget(null);
      toast.error(e instanceof Error ? e.message : String(e));
    },
  });

  const reportBatch = (res: ReturnType<typeof summarizeBatch>, okKey: string) => {
    res.failures.forEach((f) => toast.error(`「${f.name}」${f.message}`));
    if (res.success > 0) toast.success(t(okKey, { n: res.success }));
  };

  const batchDeleteMutation = useMutation({
    mutationFn: async (targets: DatatableRowJava[]) => {
      const results = await Promise.allSettled(
        targets.map((r) =>
          deleteDatatableJava({ nodeId: nodeOf(r), datatableId: r.datatableId, type: r.type, datasourceType: r.datasourceType }),
        ),
      );
      return summarizeBatch(targets, results, (r) => r.datatableName || r.datatableId || '');
    },
    onSuccess: (res) => {
      reportBatch(res, 'dtList.batchDeleteSuccess');
      setBatchDeleteOpen(false);
      setSelected({});
      setPage(1);
      invalidate();
    },
  });

  const batchAuthMutation = useMutation({
    mutationFn: async (projectId: string) => {
      const results = await Promise.allSettled(
        selectedRows.map((r) => authDatatableToProjectJava({ projectId, nodeId: nodeOf(r), datatableId: r.datatableId, configs: [], type: r.type })),
      );
      return summarizeBatch(selectedRows, results, (r) => r.datatableName || r.datatableId || '');
    },
    onSuccess: (res) => {
      reportBatch(res, 'dtList.batchAuthSuccess');
      setBatchAuthOpen(false);
      setBatchProjectId('');
      setSelected({});
      invalidate();
    },
  });

  const projectsQuery = useQuery({
    queryKey: ['authorizable-projects', platform.isP2p, 'batch'],
    queryFn: () => listAuthorizableProjectsJava({ p2p: platform.isP2p }),
    enabled: batchAuthOpen,
  });

  const deletableSelected = selectedRows.filter((r) => !datatableDeleteBlock(r));

  const toggleSelect = (r: DatatableRowJava, checked: boolean) =>
    setSelected((prev) => {
      const next = { ...prev };
      if (checked) next[rowKey(r)] = r;
      else delete next[rowKey(r)];
      return next;
    });

  /* ------------------------------ render ------------------------------ */

  const statusOptions = [
    { value: '', label: t('dataCommon.all') },
    { value: 'Available', label: t('dataTables.statusAvailable') },
    { value: 'UnAvailable', label: t('dataTables.statusUnavailable') },
  ];
  const typeOptions = [{ value: '', label: t('dataCommon.allTypes') }, ...ALL_DATATABLE_SOURCE_TYPES.map((v) => ({ value: v, label: v }))];

  const renderTee = (r: DatatableRowJava) => {
    if (!canPushToTee(r)) return '-';
    const s = r.pushToTeeStatus;
    const btn = (label: string) =>
      scope.canWrite ? (
        <Button size="sm" variant="link" onClick={() => setPushTarget(r)}>
          {label}
        </Button>
      ) : null;
    if (!s) return btn(t('dtList.teeUpload')) || '-';
    if (s === PushToTeeStatus.RUNNING) return <span className="text-blue-500">{t('dtList.teeRunning')}</span>;
    if (s === PushToTeeStatus.SUCCESS)
      return (
        <span className="inline-flex items-center gap-1">
          <Badge status="success">{t('dtList.teeSuccess')}</Badge>
          {btn(t('dtList.teeReupload'))}
        </span>
      );
    if (s === PushToTeeStatus.FAILED)
      return (
        <span className="inline-flex items-center gap-1" title={r.pushToTeeErrMsg}>
          <Badge status="error">{t('dtList.teeFailed')}</Badge>
          {btn(t('dtList.teeReupload'))}
        </span>
      );
    return '-';
  };

  const colCount = 5 + (showTee ? 1 : 0) + (isAutonomy ? 1 : 0) + (batchMode ? 1 : 0);

  return (
    <div className="space-y-4">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataTables.title')}</h2>
          <p className="text-xs text-gray-500">{t('dataTables.subtitle')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {scope.showPicker && (
            <Select
              aria-label={t('dataTables.nodeSelect')}
              value={ownerId}
              options={scope.nodeOptions}
              onChange={(v) => {
                scope.setOwnerId(v);
                setPage(1);
                setSelected({});
              }}
              className="w-40"
            />
          )}
          {scope.canWrite && (
            <>
              <Button
                variant={batchMode ? 'primary' : 'outline'}
                onClick={() => {
                  setBatchMode((v) => !v);
                  setSelected({});
                }}
              >
                {batchMode ? t('dtList.exitBatch') : t('dtList.batch')}
              </Button>
              {!scope.isTeeNode && (
                <Button variant="primary" onClick={() => setAddChoiceOpen(true)}>
                  ＋ {t('dtList.addData')}
                </Button>
              )}
            </>
          )}
        </div>
      </div>

      <FilterBar onReset={()=>{setSearchInput('');setSearch('');setStatus('');setTypeFilter('');setPage(1);}}>
        <Input
          aria-label={t('dtList.searchPlaceholder')}
          placeholder={t('dtList.searchPlaceholder')}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="!w-56"
        />
        <RadioGroup
          name={t('dataTables.status')}
          options={statusOptions}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
        />
        <Select
          aria-label={t('dtDetail.datasourceType')}
          value={typeFilter}
          options={typeOptions}
          onChange={(v) => {
            setTypeFilter(v);
            setPage(1);
          }}
          className="!w-36"
        />
        <span className="text-[11px] text-gray-400">{t('dtList.statusHint')}</span>
      </FilterBar>

      {batchMode && selectedRows.length > 0 && (
        <div className="flex items-center justify-between px-4 py-2 rounded-lg bg-blue-50 dark:bg-blue-950/30 text-xs">
          <span>{t('dtList.selectedCount', { n: selectedRows.length })}</span>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => (deletableSelected.length === 0 ? toast.warning(t('dtList.noneDeletable')) : setBatchDeleteOpen(true))}
            >
              {t('dtList.batchDelete')}
            </Button>
            <Button size="sm" variant="primary" onClick={() => setBatchAuthOpen(true)}>
              {t('dtList.batchAuth')}
            </Button>
          </div>
        </div>
      )}

      {listQuery.error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: listQuery.error.message })}
        </div>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <TableView id="data-tables"><table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                {batchMode && <th className="p-3 w-8" />}
                <th className="p-3">{t('dtList.name')}</th>
                <th className="p-3">{t('dtDetail.datasourceType')}</th>
                <th className="p-3">{t('dataTables.authProjects')}</th>
                {isAutonomy && <th className="p-3">{t('dataTables.nodeBelongs')}</th>}
                <th className="p-3">{t('dataTables.status')}</th>
                {showTee && <th className="p-3">{t('dtList.teeColumn')}</th>}
                <th className="p-3">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {listQuery.isLoading && (
                <tr>
                  <td colSpan={colCount} className="p-4 text-center text-gray-400">
                    <QueryFeedback loading/>
                  </td>
                </tr>
              )}
              {!listQuery.isLoading && !listQuery.error && rows.length === 0 && (
                <tr>
                  <td colSpan={colCount} className="p-6 text-center text-gray-400">
                    {t('dataTables.noData')}
                  </td>
                </tr>
              )}
              {rows.map((r, rowIndex) => {
                const auth = r.authProjects || [];
                const block = datatableDeleteBlock(r);
                const k = rowKey(r);
                return (
                  <tr key={k} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                    {batchMode && (
                      <td className="p-3">
                        <input
                          type="checkbox"
                          aria-label={r.datatableName}
                          disabled={EMBEDDED_SHEETS.includes(r.datatableName || '')}
                          checked={!!selected[k]}
                          onChange={(e) => toggleSelect(r, e.target.checked)}
                        />
                      </td>
                    )}
                    <td className="p-3">
                      <button
                        type="button"
                        className="font-semibold text-blue-600 hover:underline"
                        title={r.datatableName}
                        onClick={() => setDetail({ row: r, tab: 'schema' })}
                      >
                        {r.datatableName}
                      </button>
                    </td>
                    <td className="p-3 font-mono">{r.datasourceType || '-'}</td>
                    <td className="p-3" title={auth.map((a) => a.name || a.projectId).join('\n')}>
                      {auth.slice(0, 2).map((a) => a.name).filter(Boolean).join('、')}
                      {auth.length > 0 ? '，' : ''}
                      {t('dtList.authCount', { n: auth.length })}
                    </td>
                    {isAutonomy && <td className="p-3">{r.nodeName || r.nodeId || '-'}</td>}
                    <td className="p-3">
                      <span className="inline-flex items-center gap-1">
                        <Badge status={r.status === 'Available' ? 'success' : 'error'}>
                          {r.status === 'Available' ? t('dataTables.statusAvailable') : t('dataTables.statusUnavailable')}
                        </Badge>
                        <Button
                          size="sm"
                          variant="link"
                          loading={refreshMutation.isPending && refreshMutation.variables === r}
                          onClick={() => refreshMutation.mutate(r)}
                        >
                          {t('common.refresh')}
                        </Button>
                      </span>
                    </td>
                    {showTee && <td className="p-3">{renderTee(r)}</td>}
                    <td className="p-3">
                      <RowActions>
                        <Button size="sm" variant="link" data-tour={rowIndex === 0 ? 'datatable-auth' : undefined} onClick={() => setDetail({ row: r, tab: 'auth' })}>
                          {t('dtList.authManage')}
                        </Button>
                        {scope.canWrite && block !== 'embedded' && (
                          <span title={block === 'authorized' ? t('dtList.deleteBlockedAuth') : ''}>
                            <Button size="sm" variant="link" className="!text-red-600" disabled={!!block} onClick={() => setDeleteTarget(r)}>
                              {t('common.delete')}
                            </Button>
                          </span>
                        )}
                      </RowActions>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table></TableView>
        </div>
        <div className="p-3 border-t border-gray-100 dark:border-gray-800">
          <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
        </div>
      </Card>

      <DatatableDetailDrawer
        row={detail?.row ?? null}
        initialTab={detail?.tab}
        nodeId={detail ? nodeOf(detail.row) : ownerId}
        canWrite={scope.canWrite}
        showNode={isAutonomy}
        p2p={platform.isP2p}
        onClose={() => setDetail(null)}
        onChanged={invalidate}
      />

      {/* Add data: local upload or register from an existing datasource. */}
      <Modal isOpen={addChoiceOpen} onClose={() => setAddChoiceOpen(false)} title={t('dtList.addData')} width="max-w-md">
        <div className="grid grid-cols-1 gap-3 text-xs">
          <Button
            variant="outline"
            className="justify-start !p-4"
            onClick={() => {
              setAddChoiceOpen(false);
              setUploadOpen(true);
            }}
          >
            <div className="text-left">
              <div className="font-semibold">{t('dtList.addLocal')}</div>
              <div className="text-gray-500">{t('dtList.addLocalDesc')}</div>
            </div>
          </Button>
          <Button
            variant="outline"
            className="justify-start !p-4"
            onClick={() => {
              setAddChoiceOpen(false);
              setRegisterOpen(true);
            }}
          >
            <div className="text-left">
              <div className="font-semibold">{t('dtList.addFromSource')}</div>
              <div className="text-gray-500">{t('dtList.addFromSourceDesc')}</div>
            </div>
          </Button>
        </div>
      </Modal>

      <DataUploadModal
        isOpen={uploadOpen}
        onClose={() => setUploadOpen(false)}
        nodes={scope.nodes}
        defaultNodeId={ownerId}
        fixedNodeId={scope.showPicker ? undefined : ownerId}
        onCreated={() => invalidate()}
      />

      <RegisterDatatableDrawer
        isOpen={registerOpen}
        onClose={() => setRegisterOpen(false)}
        onCreated={() => invalidate()}
        ownerId={ownerId}
        multiNode={isAutonomy}
      />

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('common.delete')}
        message={t('dtList.deleteConfirm', { name: deleteTarget?.datatableName || '' })}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        onCancel={() => setDeleteTarget(null)}
      />

      <ConfirmDialog
        isOpen={batchDeleteOpen}
        title={t('dtList.batchDelete')}
        message={t('dtList.batchDeleteConfirm', { n: deletableSelected.length, skipped: selectedRows.length - deletableSelected.length })}
        danger
        loading={batchDeleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => batchDeleteMutation.mutate(deletableSelected)}
        onCancel={() => setBatchDeleteOpen(false)}
      />

      <Modal
        isOpen={batchAuthOpen}
        onClose={() => setBatchAuthOpen(false)}
        title={t('dtList.batchAuth')}
        width="max-w-md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setBatchAuthOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              disabled={!batchProjectId}
              loading={batchAuthMutation.isPending}
              onClick={() => batchAuthMutation.mutate(batchProjectId)}
            >
              {t('common.confirm')}
            </Button>
          </>
        }
      >
        <div className="space-y-3 text-xs">
          <div>{t('dtList.batchAuthTip', { n: selectedRows.length })}</div>
          <Select
            aria-label={t('dtAuth.project')}
            value={batchProjectId}
            placeholder={t('dsForm.selectPlaceholder')}
            options={(projectsQuery.data || []).map((p) => ({ value: p.projectId, label: p.projectName || p.projectId }))}
            onChange={setBatchProjectId}
          />
        </div>
      </Modal>

      <ConfirmDialog
        isOpen={!!pushTarget}
        title={t('dataTables.pushTee')}
        message={t('dataTables.pushTeeConfirm')}
        loading={pushMutation.isPending}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        onConfirm={() => pushTarget && pushMutation.mutate(pushTarget)}
        onCancel={() => setPushTarget(null)}
      />
      <Tour
        open={authTour.open}
        onClose={authTour.close}
        steps={[{ target: '[data-tour="datatable-auth"]', title: t('guideTour.datatableAuth.title'), content: t('guideTour.datatableAuth.desc') }]}
        labels={{ next: t('guideTour.next'), prev: t('guideTour.prev'), finish: t('guideTour.finish'), skip: t('guideTour.skip') }}
      />
    </div>
  );
};
