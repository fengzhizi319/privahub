import React, { useEffect, useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { DataTable, DatatableVO, AuthProjectVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';
import { DataUploadModal } from '../../features/data-upload';
import { LineageTree } from '../../features/lineage/lineage-tree';
import type { LineageNode } from '../../features/lineage/lineage-tree';

function parseSchemaText(text: string): { name: string; type: string }[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [name, type = 'string'] = line.split(':');
      return { name: name.trim(), type: type.trim() };
    });
}

/**
 * 数据资产管理页面。
 *
 * 设计要点：
 * 1. 左侧按节点过滤展示数据表卡片，支持导入、删除、推送到 TEE。
 * 2. 右侧展示选中数据表的 Schema / 分级。
 * 3. 点击“详情”打开详情抽屉，包含三个标签：
 *    - Schema 预览：展示数据表元数据（数据源、URI、空缺值、描述）与完整字段信息。
 *    - 授权项目：展示已授权的项目列表，支持新增授权（选择项目、关联键、标签列）与取消授权。
 *      授权请求调用 `project/datatable/add`，取消授权调用 `project/datatable/delete`。
 *    - 授权血缘：以树状形式展示“数据表 → 已授权项目”的血缘关系。
 * 4. 所有变更操作均通过 TanStack Query mutation + invalidateQueries 刷新相关缓存。
 */
export const DataTablesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [selectedNodeId, setSelectedNodeId] = useState<string>('');
  const [selectedTableId, setSelectedTableId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [tableName, setTableName] = useState('');
  const [datasourceId, setDatasourceId] = useState('');
  const [relativeUri, setRelativeUri] = useState('');
  const [schemaText, setSchemaText] = useState('id:string\nvalue:int');
  const [classification, setClassification] = useState('L1');
  const [deleteTarget, setDeleteTarget] = useState<DataTable | null>(null);
  const [pushTeeTarget, setPushTeeTarget] = useState<DataTable | null>(null);

  // Detail drawer state
  const [detailTable, setDetailTable] = useState<DataTable | null>(null);
  const [detailTab, setDetailTab] = useState<'preview' | 'auth' | 'lineage'>('preview');

  // Add authorization form state
  const [authProjectId, setAuthProjectId] = useState('');
  const [authKey, setAuthKey] = useState('');
  const [authLabel, setAuthLabel] = useState('');
  const [showAuthForm, setShowAuthForm] = useState(false);

  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });
  const nodes = nodesQuery.data ?? [];

  const memoizedNodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  // Default the selected node to the first one once nodes load.
  useEffect(() => {
    if (!selectedNodeId && memoizedNodes.length > 0) {
      setSelectedNodeId(memoizedNodes[0].nodeId);
    }
  }, [memoizedNodes, selectedNodeId]);

  const sourcesQuery = useQuery({
    queryKey: ['datasources', selectedNodeId],
    queryFn: () => apiClient.getDataSources(selectedNodeId),
    enabled: !!selectedNodeId,
  });
  const sources = sourcesQuery.data ?? [];

  const memoizedSources = useMemo(() => sourcesQuery.data ?? [], [sourcesQuery.data]);

  // Default the datasource selection once sources load.
  useEffect(() => {
    if (!datasourceId && memoizedSources.length > 0) {
      setDatasourceId(memoizedSources[0].datasourceId);
    }
  }, [memoizedSources, datasourceId]);

  const tablesQuery = useQuery({
    queryKey: ['datatables', selectedNodeId],
    queryFn: () => apiClient.getDataTables(selectedNodeId),
    enabled: !!selectedNodeId,
  });
  const tables = tablesQuery.data ?? [];
  const selectedTable: DataTable | null =
    tables.find((tbl) => tbl.tableId === selectedTableId) ?? tables[0] ?? null;

  const invalidateTables = () =>
    queryClient.invalidateQueries({ queryKey: ['datatables', selectedNodeId] });

  // Detail query: fetches full datatable VO including authProjects and raw schema.
  const detailQuery = useQuery({
    queryKey: ['datatable-detail', detailTable?.nodeId, detailTable?.tableId],
    queryFn: () =>
      apiClient.getDataTable({
        nodeId: detailTable!.nodeId,
        datatableId: detailTable!.tableId,
        type: 'CSV',
      }),
    enabled: !!detailTable,
  });
  const detailVO: DatatableVO | null = detailQuery.data?.datatableVO ?? null;

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
    enabled: showAuthForm,
  });

  // Projects that are not already authorized to this table (used for add-auth dropdown).
  const availableAuthProjects = useMemo(() => {
    const data = projectsQuery.data ?? [];
    const authorizedIds = new Set((detailVO?.authProjects ?? []).map((a) => a.projectId));
    return data.filter((p) => p.projectId && !authorizedIds.has(p.projectId));
  }, [projectsQuery.data, detailVO?.authProjects]);

  /**
   * 授权血缘树数据：数据源 → 数据表 → 已授权项目。
   *
   * - 根节点为数据源（名称 + 类型徽标）；
   * - 二级节点为数据表（名称 + 字段数 + URI / 状态明细）；
   * - 三级节点为已授权项目（项目名 + 计算模式徽标 + 关联键/标签列/授权时间明细）。
   * 供 `LineageTree` 组件以树形图可视化渲染。
   */
  const lineageRoot = useMemo<LineageNode | null>(() => {
    if (!detailTable) return null;
    const authProjects = detailVO?.authProjects ?? [];
    return {
      id: 'datasource',
      icon: '🗄️',
      title: detailVO?.datasourceName || t('dataTables.lineageDatasource'),
      subtitle: detailVO?.datasourceId,
      badge: detailVO?.datasourceType,
      badgeTone: 'purple',
      children: [
        {
          id: 'table',
          icon: '📋',
          title: detailVO?.datatableName || detailTable.tableName,
          subtitle: detailVO?.datatableId || detailTable.tableId,
          badge: `${detailVO?.schema?.length ?? 0} ${t('dataTables.lineageColumns')}`,
          badgeTone: 'blue',
          details: [
            ...(detailVO?.relativeUri ? [{ label: 'URI', value: detailVO.relativeUri }] : []),
            ...(detailVO?.status ? [{ label: t('dataTables.lineageStatus'), value: detailVO.status }] : []),
          ],
          children: authProjects.map((auth, idx) => ({
            id: `project-${auth.projectId}-${idx}`,
            icon: '📁',
            title: auth.name || auth.projectId || '-',
            subtitle: auth.projectId,
            badge: auth.computeMode,
            badgeTone: 'green' as const,
            details: [
              ...(auth.associateKeys?.length
                ? [{ label: t('dataTables.lineageAssocKeys'), value: auth.associateKeys.join(', ') }]
                : []),
              ...(auth.labelKeys?.length
                ? [{ label: t('dataTables.lineageLabelKeys'), value: auth.labelKeys.join(', ') }]
                : []),
              ...(auth.gmtCreate ? [{ label: t('dataTables.lineageAuthTime'), value: auth.gmtCreate }] : []),
            ],
          })),
        },
      ],
    };
  }, [detailTable, detailVO, t]);

  const createMutation = useMutation({
    mutationFn: () => {
      const selectedSource = sources.find((s) => s.datasourceId === datasourceId) || sources[0];
      const columns = parseSchemaText(schemaText).map((c) => ({ ...c, comment: '', classification }));
      return apiClient.createDataTable({
        ownerId: selectedNodeId,
        nodeIds: [selectedNodeId],
        datatableName: tableName,
        datasourceId: selectedSource.datasourceId,
        datasourceName: selectedSource.name,
        datasourceType: selectedSource.type,
        relativeUri,
        columns,
      });
    },
    onSuccess: () => {
      setIsModalOpen(false);
      resetForm();
      invalidateTables();
      toast.success(t('dataTables.createSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const pushTeeMutation = useMutation({
    mutationFn: (table: DataTable) =>
      apiClient.pushDatatableToTee({
        nodeId: table.nodeId || selectedNodeId,
        datatableId: table.tableId,
        datasourceId: table.datasourceId,
        relativeUri: table.relativeUri,
      }),
    onSuccess: () => {
      setPushTeeTarget(null);
      toast.success(t('dataTables.pushTeeSuccess'));
    },
    onError: (e) => {
      setPushTeeTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (table: DataTable) =>
      apiClient.deleteDataTable({
        nodeId: table.nodeId || selectedNodeId,
        datatableId: table.tableId,
        datasourceId: table.datasourceId,
        datasourceType: table.datasourceType,
        relativeUri: table.relativeUri,
      }),
    onSuccess: () => {
      setDeleteTarget(null);
      invalidateTables();
    },
    onError: (e) => {
      setDeleteTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const addAuthMutation = useMutation({
    mutationFn: () => {
      if (!detailTable || !authProjectId || !authKey) {
        throw new Error('Project and associate key are required');
      }
      if (authKey === authLabel) {
        throw new Error('Associate key and label column cannot be the same');
      }
      const configs: Array<{
        colName: string;
        isAssociateKey?: boolean;
        isLabelKey?: boolean;
        isProtection?: boolean;
      }> = [
        {
          colName: authKey,
          isAssociateKey: true,
          isProtection: true,
        },
      ];
      if (authLabel) {
        configs.push({
          colName: authLabel,
          isLabelKey: true,
        });
      }
      return apiClient.addProjectDatatable({
        projectId: authProjectId,
        nodeId: detailTable.nodeId,
        datatableId: detailTable.tableId,
        configs,
      });
    },
    onSuccess: () => {
      resetAuthForm();
      queryClient.invalidateQueries({
        queryKey: ['datatable-detail', detailTable?.nodeId, detailTable?.tableId],
      });
      toast.success(t('dataTables.authSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const removeAuthMutation = useMutation({
    mutationFn: (auth: AuthProjectVO) => {
      if (!detailTable || !auth.projectId) {
        throw new Error('Missing table or project info');
      }
      return apiClient.deleteProjectDatatable({
        projectId: auth.projectId,
        nodeId: detailTable.nodeId,
        datatableId: detailTable.tableId,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['datatable-detail', detailTable?.nodeId, detailTable?.tableId],
      });
      toast.success(t('dataTables.authCancel'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const resetForm = () => {
    setTableName('');
    setRelativeUri('');
    setSchemaText('id:string\nvalue:int');
    setClassification('L1');
    if (sources.length > 0) {
      setDatasourceId(sources[0].datasourceId);
    }
  };

  const resetAuthForm = () => {
    setShowAuthForm(false);
    setAuthProjectId('');
    setAuthKey('');
    setAuthLabel('');
  };

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    createMutation.mutate();
  };

  const handleDelete = (table: DataTable) => {
    setDeleteTarget(table);
  };

  const openDetail = (table: DataTable) => {
    setDetailTable(table);
    setDetailTab('preview');
    resetAuthForm();
  };

  const closeDetail = () => {
    setDetailTable(null);
    resetAuthForm();
  };

  const schemaColumns = detailVO?.schema ?? [];

  const loading = tablesQuery.isLoading;
  const queryError = tablesQuery.error?.message || nodesQuery.error?.message || null;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataTables.title')}</h2>
          <p className="text-xs text-gray-500">{t('dataTables.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={selectedNodeId}
            onChange={(e) => setSelectedNodeId(e.target.value)}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {nodes.map((n) => (
              <option key={n.nodeId} value={n.nodeId}>{n.nodeName}</option>
            ))}
          </select>
          <AccessGuard access={{ types: [Platform.CENTER] }}>
            <Button variant="primary" icon={<span>＋</span>} onClick={() => setIsModalOpen(true)}>{t('dataTables.import')}</Button>
            <Button variant="outline" onClick={() => setIsUploadModalOpen(true)}>{t('dataUpload.title')}</Button>
          </AccessGuard>
        </div>
      </div>

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left: Table List */}
        <div className="lg:col-span-1 space-y-3">
          {tables.length === 0 && !loading && !queryError && (
            <div className="text-xs text-gray-400 text-center py-6">{t('dataTables.noData')}</div>
          )}
          {tables.map((tbl) => (
            <Card
              key={tbl.tableId}
              onClick={() => setSelectedTableId(tbl.tableId)}
              className={`cursor-pointer transition-all ${
                selectedTable?.tableId === tbl.tableId ? 'border-blue-500 ring-2 ring-blue-500/20' : 'hover:border-gray-300'
              }`}
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="font-semibold text-sm text-gray-900 dark:text-gray-100">{tbl.tableName}</span>
                <div className="flex items-center gap-2">
                  <Badge status={tbl.status === 'Available' ? 'success' : 'error'}>
                    {tbl.status === 'Available' ? t('dataTables.statusAvailable') : t('dataTables.statusUnavailable')}
                  </Badge>
                  <AccessGuard access={{ types: [Platform.CENTER] }}>
                    <Button size="sm" variant="danger" onClick={(e) => { e.stopPropagation(); handleDelete(tbl); }}>{t('common.delete')}</Button>
                  </AccessGuard>
                </div>
              </div>
              <div className="text-xs text-gray-500 font-mono">ID: {tbl.tableId} • Node: {tbl.nodeName || tbl.nodeId}</div>
              <div className="mt-2 text-xs text-gray-400">{t('dataTables.rows')}: {(tbl.rowCount || 0).toLocaleString()} | {t('dataTables.columns')}: {tbl.columns.length}</div>
              <div className="mt-2">
                <Button size="sm" variant="outline" onClick={(e) => { e.stopPropagation(); openDetail(tbl); }}>
                  {t('dataTables.detail')}
                </Button>
              </div>
            </Card>
          ))}
        </div>

        {/* Right: Selected Table Schema & Classification Details */}
        {selectedTable && (
          <div className="lg:col-span-2">
            <Card title={`Schema: ${selectedTable.tableName}`}>
              <div className="mb-4 flex items-center justify-between text-xs text-gray-500 pb-3 border-b border-gray-100 dark:border-gray-800">
                <div>{t('dataTables.nodeBelongs')}: <span className="font-semibold text-gray-800 dark:text-gray-200">{selectedTable.nodeName || selectedTable.nodeId}</span></div>
                <div className="flex items-center gap-3">
                  <span>{t('dataTables.rows')}: <span className="font-semibold text-gray-800 dark:text-gray-200">{(selectedTable.rowCount || 0).toLocaleString()}</span></span>
                  <AccessGuard access={{ types: [Platform.CENTER] }}>
                    <Button size="sm" variant="outline" loading={pushTeeMutation.isPending} onClick={() => setPushTeeTarget(selectedTable)}>
                      {t('dataTables.pushTee')}
                    </Button>
                  </AccessGuard>
                </div>
              </div>

              <table className="w-full text-left text-xs">
                <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold uppercase">
                  <tr>
                    <th className="p-3">{t('dataTables.columnName')}</th>
                    <th className="p-3">{t('dataTables.dataType')}</th>
                    <th className="p-3">{t('dataTables.sensitivity')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:border-gray-800">
                  {selectedTable.columns.map((col, idx) => (
                    <tr key={idx}>
                      <td className="p-3 font-mono font-medium text-gray-800 dark:text-gray-200">{col.name}</td>
                      <td className="p-3 font-mono text-gray-500">{col.type}</td>
                      <td className="p-3">
                        <span className={`px-2 py-0.5 rounded text-[10px] font-semibold font-mono ${
                          col.classification === 'L3' ? 'bg-amber-100 dark:bg-amber-950 text-amber-600' :
                          col.classification === 'L2' ? 'bg-blue-100 dark:bg-blue-950 text-blue-600' :
                          'bg-gray-100 dark:bg-gray-800 text-gray-600'
                        }`}>
                          {col.classification || '-'} {col.classification ? t('dataTables.standard') : ''}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          </div>
        )}
      </div>

      <Modal
        isOpen={isModalOpen}
        onClose={() => { setIsModalOpen(false); resetForm(); }}
        title={t('dataTables.modalImportTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsModalOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleCreate} loading={createMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.nameLabel')}</label>
            <input
              type="text"
              value={tableName}
              onChange={(e) => setTableName(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.datasourceLabel')}</label>
            <select
              value={datasourceId}
              onChange={(e) => setDatasourceId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              {sources.map((s) => (
                <option key={s.datasourceId} value={s.datasourceId}>{s.name} ({s.type})</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.uriLabel')}</label>
            <input
              type="text"
              value={relativeUri}
              onChange={(e) => setRelativeUri(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.schemaLabel')}</label>
            <textarea
              value={schemaText}
              onChange={(e) => setSchemaText(e.target.value)}
              rows={5}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.classificationLabel')}</label>
            <select
              value={classification}
              onChange={(e) => setClassification(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              {['L1', 'L2', 'L3', 'L4', 'L5'].map((level) => (
                <option key={level} value={level}>{level}</option>
              ))}
            </select>
          </div>
        </form>
      </Modal>

      {/* Detail Drawer */}
      {detailTable && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-black/40" onClick={closeDetail} />
          <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 h-full shadow-2xl overflow-y-auto p-6 space-y-6">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataTables.datatableInfo')}</h3>
                <p className="text-xs text-gray-500 font-mono mt-1">{detailTable.tableId}</p>
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={() => detailQuery.refetch()} loading={detailQuery.isFetching}>
                  {t('dataTables.refreshStatus')}
                </Button>
                <Button size="sm" variant="ghost" onClick={closeDetail}>✕</Button>
              </div>
            </div>

            <div className="text-xs space-y-2">
              <div className="flex items-center gap-2">
                <span className="text-gray-500">{t('dataTables.datasourceLabel')}:</span>
                <span className="font-semibold">{detailVO?.datasourceName || detailTable.datasourceId} ({detailVO?.datasourceType || detailTable.datasourceType})</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-gray-500">{t('dataTables.nodeBelongs')}:</span>
                <span className="font-semibold">{detailTable.nodeName || detailTable.nodeId}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-gray-500">URI:</span>
                <span className="font-mono break-all">{detailVO?.relativeUri || detailTable.relativeUri || '-'}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-gray-500">{t('dataTables.status')}:</span>
                <Badge status={detailVO?.status === 'Available' || detailTable.status === 'Available' ? 'success' : 'error'}>
                  {detailVO?.status || detailTable.status}
                </Badge>
              </div>
            </div>

            {/* Tabs */}
            <div className="flex items-center gap-4 border-b border-gray-200 dark:border-gray-800 pb-2">
              {(['preview', 'auth', 'lineage'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setDetailTab(tab)}
                  className={`text-sm font-semibold pb-2 ${
                    detailTab === tab ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'
                  }`}
                >
                  {t(`dataTables.${tab}` as const)}
                </button>
              ))}
            </div>

            {detailTab === 'preview' && (
              <div className="space-y-4">
                <h4 className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('dataTables.preview')}</h4>
                <table className="w-full text-left text-xs">
                  <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold uppercase">
                    <tr>
                      <th className="p-3">{t('dataTables.columnName')}</th>
                      <th className="p-3">{t('dataTables.dataType')}</th>
                      <th className="p-3">{t('dataTables.columnDescription')}</th>
                      <th className="p-3">{t('dataTables.sensitivity')}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 dark:border-gray-800">
                    {schemaColumns.map((col, idx) => (
                      <tr key={idx}>
                        <td className="p-3 font-mono font-medium text-gray-800 dark:text-gray-200">{col.colName}</td>
                        <td className="p-3 font-mono text-gray-500">{col.colType}</td>
                        <td className="p-3 text-gray-500">{col.colComment || '-'}</td>
                        <td className="p-3">
                          <span className={`px-2 py-0.5 rounded text-[10px] font-semibold font-mono ${
                            col.classification === 'L3' ? 'bg-amber-100 dark:bg-amber-950 text-amber-600' :
                            col.classification === 'L2' ? 'bg-blue-100 dark:bg-blue-950 text-blue-600' :
                            'bg-gray-100 dark:bg-gray-800 text-gray-600'
                          }`}>
                            {col.classification || '-'}
                          </span>
                        </td>
                      </tr>
                    ))}
                    {schemaColumns.length === 0 && (
                      <tr>
                        <td colSpan={4} className="p-3 text-center text-gray-400">{t('dataTables.noData')}</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}

            {detailTab === 'auth' && (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('dataTables.authProjects')}</h4>
                  <AccessGuard access={{ types: [Platform.CENTER] }}>
                    <Button size="sm" variant="primary" onClick={() => setShowAuthForm(true)} disabled={showAuthForm}>
                      ＋ {t('dataTables.addAuth')}
                    </Button>
                  </AccessGuard>
                </div>

                {showAuthForm && (
                  <div className="p-4 rounded-lg border border-gray-200 dark:border-gray-800 bg-gray-50 dark:bg-gray-850 space-y-3 text-xs">
                    <div>
                      <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('projects.selectProject')}</label>
                      <select
                        value={authProjectId}
                        onChange={(e) => setAuthProjectId(e.target.value)}
                        className="w-full p-2 rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                      >
                        <option value="">-</option>
                        {availableAuthProjects.map((p) => (
                          <option key={p.projectId} value={p.projectId}>{p.projectName} ({p.projectId})</option>
                        ))}
                      </select>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.associateKey')} *</label>
                        <select
                          value={authKey}
                          onChange={(e) => setAuthKey(e.target.value)}
                          className="w-full p-2 rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                        >
                          <option value="">-</option>
                          {schemaColumns.map((col) => (
                            <option key={col.colName} value={col.colName}>{col.colName}</option>
                          ))}
                        </select>
                      </div>
                      <div>
                        <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataTables.labelKey')}</label>
                        <select
                          value={authLabel}
                          onChange={(e) => setAuthLabel(e.target.value)}
                          className="w-full p-2 rounded-lg bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                        >
                          <option value="">{t('dataTables.labelKeyNone')}</option>
                          {schemaColumns.map((col) => (
                            <option key={col.colName} value={col.colName}>{col.colName}</option>
                          ))}
                        </select>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 pt-1">
                      <Button size="sm" variant="ghost" onClick={resetAuthForm}>{t('common.cancel')}</Button>
                      <Button
                        size="sm"
                        variant="primary"
                        loading={addAuthMutation.isPending}
                        disabled={!authProjectId || !authKey}
                        onClick={() => addAuthMutation.mutate()}
                      >
                        {t('common.save')}
                      </Button>
                    </div>
                  </div>
                )}

                <div className="space-y-2">
                  {(detailVO?.authProjects || []).length === 0 && !showAuthForm && (
                    <div className="text-xs text-gray-400 text-center py-6">{t('dataTables.noAuth')}</div>
                  )}
                  {(detailVO?.authProjects || []).map((auth, idx) => (
                    <div key={`${auth.projectId}-${idx}`} className="flex items-center justify-between p-3 rounded-lg border border-gray-200 dark:border-gray-800 text-xs">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-gray-800 dark:text-gray-200 truncate">{auth.name || auth.projectId}</span>
                          {auth.computeMode && (
                            <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-blue-50 dark:bg-blue-950 text-blue-600 dark:text-blue-400">
                              {auth.computeMode}
                            </span>
                          )}
                        </div>
                        <div className="text-gray-400 mt-0.5 font-mono">{auth.projectId}</div>
                      </div>
                      <AccessGuard access={{ types: [Platform.CENTER] }}>
                        <Button
                          size="sm"
                          variant="danger"
                          loading={removeAuthMutation.isPending && removeAuthMutation.variables?.projectId === auth.projectId}
                          onClick={() => removeAuthMutation.mutate(auth)}
                        >
                          {t('dataTables.removeAuth')}
                        </Button>
                      </AccessGuard>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {detailTab === 'lineage' && (
              <div className="space-y-4">
                <h4 className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('dataTables.lineageTitle')}</h4>
                {/* 树形血缘图：数据源 → 数据表 → 已授权项目；未加载完成时展示空态。 */}
                {lineageRoot ? (
                  <LineageTree root={lineageRoot} />
                ) : (
                  <div className="text-xs text-gray-400 text-center py-6">{t('dataTables.noLineage')}</div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('common.delete')}
        message={t('dataTables.deleteConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        onCancel={() => setDeleteTarget(null)}
      />

      {/* Push to TEE Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!pushTeeTarget}
        title={t('dataTables.pushTee')}
        message={t('dataTables.pushTeeConfirm')}
        loading={pushTeeMutation.isPending}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        onConfirm={() => pushTeeTarget && pushTeeMutation.mutate(pushTeeTarget)}
        onCancel={() => setPushTeeTarget(null)}
      />

      {/* Data Upload Modal */}
      <DataUploadModal
        isOpen={isUploadModalOpen}
        onClose={() => setIsUploadModalOpen(false)}
        nodes={nodes}
        defaultNodeId={selectedNodeId}
      />

    </div>
  );
};
