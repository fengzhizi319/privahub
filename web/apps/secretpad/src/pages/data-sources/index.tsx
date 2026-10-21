/**
 * 数据源管理页（旧前端 `data-source-list` 的迁移版）。
 *
 * - 列表：`datasource/list`（Java `DatasourceListRequest{page,size,ownerId,name,status,types}`），服务端分页；
 * - 名称搜索（300ms 防抖）、类型筛选、状态筛选（AUTONOMY 下隐藏）；
 * - 注册：结构化表单（OSS / ODPS / MYSQL / HTTP / LOCAL），AUTONOMY 支持多节点（最多 5 个）；
 * - 删除：已绑定数据表（relatedDatas）时禁止并列出绑定的数据表，HTTP 数据源不可删除；
 * - 节点视图（/node/$nodeId）下作用于该节点并隐藏节点选择；TEE 节点无数据源功能。
 */
import React, { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Button, Card, ConfirmDialog, Empty, Input, Pagination, RadioGroup, Select, toast } from '@secretpad/design-system';
import { REMOTE_DATASOURCE_TYPES, apiClient, deleteDatasourceJava, listDatasourcesJava } from '@secretpad/api-client';
import type { DatasourceListInfoJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { useNodeScope } from '@/features/data-scope';
import { DatasourceFormDrawer, datasourceDeleteBlock } from '@/features/datasource-form';

const PAGE_SIZE = 10;

export const DataSourcesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const scope = useNodeScope();
  const { ownerId, platform } = scope;
  const isAutonomy = platform.isAutonomy;

  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DatasourceListInfoJava | null>(null);

  useEffect(() => {
    const id = setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 300);
    return () => clearTimeout(id);
  }, [searchInput]);

  const listKey = ['datasources-java', ownerId, page, search, status, typeFilter];
  const listQuery = useQuery({
    queryKey: listKey,
    queryFn: () =>
      listDatasourcesJava({
        ownerId,
        page,
        size: PAGE_SIZE,
        name: search,
        status,
        types: typeFilter ? [typeFilter] : REMOTE_DATASOURCE_TYPES,
      }),
    enabled: !!ownerId && !scope.isTeeNode,
  });
  const infos = listQuery.data?.infos ?? [];
  const total = listQuery.data?.total ?? 0;

  // AUTONOMY: nodes of the institution that are Ready (legacy `inst/node/list`).
  const instNodesQuery = useQuery({
    queryKey: ['inst-nodes'],
    queryFn: () => apiClient.listInstNodes(),
    enabled: isAutonomy && createOpen,
  });
  const formNodeOptions = isAutonomy
    ? (instNodesQuery.data ?? [])
        .filter((n) => (n.nodeStatus || n.status || '').toLowerCase() === 'ready')
        .map((n) => ({ value: n.nodeId, label: n.nodeName || n.nodeId }))
    : scope.nodeOptions.filter((o) => o.value === ownerId);

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['datasources-java', ownerId] });

  const deleteMutation = useMutation({
    mutationFn: (ds: DatasourceListInfoJava) =>
      deleteDatasourceJava({ ownerId, datasourceId: ds.datasourceId, type: ds.type }),
    onSuccess: () => {
      toast.success(t('dataSources.deleteSuccess'));
      setDeleteTarget(null);
      invalidate();
    },
    onError: (e) => {
      setDeleteTarget(null);
      toast.error(e instanceof Error ? e.message : String(e));
    },
  });

  const openDetail = (ds: DatasourceListInfoJava) =>
    navigate({ to: '/data-sources/detail', search: { ownerId: ownerId || '', datasourceId: ds.datasourceId || '', type: ds.type || '' } });

  if (scope.isTeeNode) {
    return <Empty className="py-16">{t('dataSources.teeNoDatasource')}</Empty>;
  }

  const typeOptions = [{ value: '', label: t('dataCommon.allTypes') }, ...REMOTE_DATASOURCE_TYPES.map((v) => ({ value: v, label: v }))];
  const statusOptions = [
    { value: '', label: t('dataCommon.all') },
    { value: 'Available', label: t('dataTables.statusAvailable') },
    { value: 'UnAvailable', label: t('dataTables.statusUnavailable') },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataSources.title')}</h2>
          <p className="text-xs text-gray-500">{t('dataSources.subtitle')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {scope.showPicker && (
            <Select
              aria-label={t('dataSources.nodeSelect')}
              value={ownerId}
              options={scope.nodeOptions}
              onChange={(v) => {
                scope.setOwnerId(v);
                setPage(1);
              }}
              className="w-40"
            />
          )}
          {scope.canWrite && (
            <Button variant="primary" onClick={() => setCreateOpen(true)}>
              ＋ {t('dataSources.register')}
            </Button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Input
          aria-label={t('dataSources.searchPlaceholder')}
          placeholder={t('dataSources.searchPlaceholder')}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          className="!w-56"
        />
        {!isAutonomy && (
          <RadioGroup
            name={t('dataSources.status')}
            options={statusOptions}
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(1);
            }}
          />
        )}
        <Select
          aria-label={t('dataSources.type')}
          value={typeFilter}
          options={typeOptions}
          onChange={(v) => {
            setTypeFilter(v);
            setPage(1);
          }}
          className="!w-36"
        />
      </div>

      {listQuery.error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: listQuery.error.message })}
        </div>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-3">{t('dataSources.nameLabel')}</th>
                <th className="p-3">{t('dataSources.type')}</th>
                <th className="p-3">{t('dataSources.status')}</th>
                <th className="p-3">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {listQuery.isLoading && (
                <tr>
                  <td colSpan={4} className="p-4 text-center text-gray-400">
                    {t('common.loading')}
                  </td>
                </tr>
              )}
              {!listQuery.isLoading && infos.length === 0 && (
                <tr>
                  <td colSpan={4} className="p-6 text-center text-gray-400">
                    {t('dataSources.noData')}
                  </td>
                </tr>
              )}
              {infos.map((ds) => {
                const block = datasourceDeleteBlock(ds);
                const blockTip =
                  block === 'bound'
                    ? `${t('dataSources.deleteBlockedBound')}\n${(ds.relatedDatas || []).join('\n')}`
                    : block === 'http'
                      ? t('dataSources.deleteBlockedHttp')
                      : '';
                return (
                  <tr key={ds.datasourceId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                    <td className="p-3">
                      <button type="button" className="font-semibold text-blue-600 hover:underline" title={ds.name} onClick={() => openDetail(ds)}>
                        {ds.name}
                      </button>
                    </td>
                    <td className="p-3 font-mono">{ds.type}</td>
                    <td className="p-3">
                      <div className="flex flex-wrap gap-1.5">
                        {(ds.nodes || []).map((n) => (
                          <span key={n.nodeId} className="inline-flex items-center gap-1">
                            <span className="text-gray-500">{n.nodeName || n.nodeId}</span>
                            <Badge status={n.status === 'Available' ? 'success' : 'error'}>
                              {n.status === 'Available' ? t('dataTables.statusAvailable') : t('dataTables.statusUnavailable')}
                            </Badge>
                          </span>
                        ))}
                        {(ds.nodes || []).length === 0 && '-'}
                      </div>
                    </td>
                    <td className="p-3">
                      <div className="flex items-center gap-2">
                        <Button size="sm" variant="link" onClick={() => openDetail(ds)}>
                          {t('dataSources.detail')}
                        </Button>
                        {scope.canWrite && (
                          <span title={blockTip}>
                            <Button size="sm" variant="link" className="!text-red-600" disabled={!!block} onClick={() => setDeleteTarget(ds)}>
                              {t('common.delete')}
                            </Button>
                          </span>
                        )}
                        {block === 'bound' && (
                          <span className="text-[11px] text-gray-400" title={blockTip}>
                            {t('dataSources.boundCount', { n: (ds.relatedDatas || []).length })}
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <div className="p-3 border-t border-gray-100 dark:border-gray-800">
          <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
        </div>
      </Card>

      <DatasourceFormDrawer
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={invalidate}
        ownerId={ownerId}
        nodeOptions={formNodeOptions}
        defaultNodeId={ownerId}
        multiNode={isAutonomy}
      />

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('dataSources.delete')}
        message={t('dataSources.deleteConfirmNamed', { name: deleteTarget?.name || '' })}
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
