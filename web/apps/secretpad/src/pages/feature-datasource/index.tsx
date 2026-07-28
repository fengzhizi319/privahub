/**
 * 特征数据源 / 投票同步页面（Feature Datasource & Vote Sync）。
 *
 * 对应旧前端尚未迁移的两个后端控制器：
 * - `FeatureDatasourceController`：特征数据源的创建（`feature_datasource/create`）
 *   与授权列表查询（`feature_datasource/auth/list`）；
 * - `VoteSyncController`：跨节点数据同步投票（`vote_sync/create`）。
 *
 * 页面分为两个 Tab：
 * 1. 特征数据源：选择项目 + 节点后查询该节点在该项目下被授权的特征表及其字段，
 *    并支持创建新的特征数据源（动态编辑字段、多选授权节点）；
 * 2. 投票同步：选择项目、同步数据类型（对应后端 `VoteSyncTypeEnum`）与参与节点，
 *    发起一次跨节点数据同步投票。
 */
import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, toast } from '@secretpad/design-system';
import type { TableColumnVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';

/**
 * 同步数据类型选项，对应后端 `VoteSyncTypeEnum`。
 *
 * 这些值决定了本次投票同步要同步哪一类数据（项目、节点路由、数据表管理等）。
 */
const SYNC_DATA_TYPES = [
  'VOTE_REQUEST',
  'VOTE_INVITE',
  'NODE_ROUTE',
  'TEE_NODE_DATATABLE_MANAGEMENT',
  'PROJECT_APPROVAL_CONFIG',
  'PROJECT',
  'PROJECT_NODE',
  'PROJECT_INST',
];

/** 特征数据源类型选项（与后端数据源类型保持一致）。 */
const FEATURE_DS_TYPES = ['mysql', 'postgres', 'csv', 'localfs'];

/** 创建表单中的单个字段行（可编辑的 TableColumnVO）。 */
interface ColumnRow {
  colName: string;
  colType: string;
  colComment: string;
}

export const FeatureDatasourcePage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { user } = useAuthStore();

  // 当前用户所属节点（ownerId），创建特征数据源时作为属主。
  const ownerId = user?.ownerId || '';

  // 当前激活的 Tab：特征数据源 / 投票同步。
  const [activeTab, setActiveTab] = useState<'feature' | 'voteSync'>('feature');

  /* ------------------------- 特征数据源查询状态 ------------------------- */
  const [queryProjectId, setQueryProjectId] = useState('');
  const [queryNodeId, setQueryNodeId] = useState('');
  // 是否已触发查询（区分“未查询”与“查询为空”）。
  const [hasQueried, setHasQueried] = useState(false);

  /* ------------------------- 创建表单状态 ------------------------- */
  const [createOpen, setCreateOpen] = useState(false);
  const [featureTableName, setFeatureTableName] = useState('');
  const [dsType, setDsType] = useState('mysql');
  const [dsUrl, setDsUrl] = useState('');
  const [dsDesc, setDsDesc] = useState('');
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [columnRows, setColumnRows] = useState<ColumnRow[]>([{ colName: '', colType: 'string', colComment: '' }]);

  /* ------------------------- 投票同步状态 ------------------------- */
  const [syncProjectId, setSyncProjectId] = useState('');
  const [syncType, setSyncType] = useState('');
  const [syncNodeIds, setSyncNodeIds] = useState<string[]>([]);

  /* ------------------------------ 数据查询 ------------------------------ */

  // 项目列表（供查询与创建表单选择）。
  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const projects = projectsQuery.data ?? [];

  // 节点列表（供查询、创建授权与投票同步选择）。
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });
  const nodes = nodesQuery.data ?? [];

  // 特征数据源授权列表：需要项目 + 节点两个条件。
  const featureListQuery = useQuery({
    queryKey: ['feature-datasource', queryProjectId, queryNodeId],
    queryFn: () => apiClient.listFeatureDatasourceAuth(queryProjectId, queryNodeId),
    enabled: hasQueried && !!queryProjectId && !!queryNodeId,
  });
  const featureList = featureListQuery.data ?? [];

  /* ------------------------------ 变更操作 ------------------------------ */

  // 失效特征数据源列表缓存（创建成功后刷新）。
  const invalidateFeature = () =>
    queryClient.invalidateQueries({ queryKey: ['feature-datasource'] });

  // 创建特征数据源。
  const createMutation = useMutation({
    mutationFn: () => {
      // 将编辑行转换为后端需要的 TableColumnVO 结构（过滤掉未填写字段名的行）。
      const columns: TableColumnVO[] = columnRows
        .filter((r) => r.colName.trim())
        .map((r) => ({ colName: r.colName.trim(), colType: r.colType || 'string', colComment: r.colComment || undefined }));
      return apiClient.createFeatureDatasource({
        featureTableName: featureTableName.trim(),
        type: dsType,
        url: dsUrl.trim(),
        ownerId,
        nodeIds: selectedNodeIds,
        columns,
        desc: dsDesc.trim() || undefined,
      });
    },
    onSuccess: () => {
      setCreateOpen(false);
      resetCreateForm();
      invalidateFeature();
      toast.success(t('featureDs.createSuccess'));
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  // 发起投票同步。
  const syncMutation = useMutation({
    mutationFn: () =>
      apiClient.createVoteSync([
        {
          projectNodesInfo: { projectId: syncProjectId, nodeIds: syncNodeIds },
          syncDataType: syncType,
        },
      ]),
    onSuccess: () => {
      toast.success(t('featureDs.syncSuccess'));
      setSyncType('');
      setSyncNodeIds([]);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  /* ------------------------------ 表单辅助 ------------------------------ */

  /** 重置创建表单。 */
  const resetCreateForm = () => {
    setFeatureTableName('');
    setDsType('mysql');
    setDsUrl('');
    setDsDesc('');
    setSelectedNodeIds([]);
    setColumnRows([{ colName: '', colType: 'string', colComment: '' }]);
  };

  /** 更新某一字段行的某一列。 */
  const updateColumnRow = (idx: number, field: keyof ColumnRow, value: string) => {
    setColumnRows((prev) => prev.map((r, i) => (i === idx ? { ...r, [field]: value } : r)));
  };

  /** 切换某节点在授权列表中的选中状态。 */
  const toggleNodeId = (nodeId: string) => {
    setSelectedNodeIds((prev) => (prev.includes(nodeId) ? prev.filter((id) => id !== nodeId) : [...prev, nodeId]));
  };

  /** 切换某节点在投票同步中的选中状态。 */
  const toggleSyncNode = (nodeId: string) => {
    setSyncNodeIds((prev) => (prev.includes(nodeId) ? prev.filter((id) => id !== nodeId) : [...prev, nodeId]));
  };

  const queryError = featureListQuery.error?.message || null;

  /* ------------------------------ 渲染 ------------------------------ */

  return (
    <div className="space-y-6">
      {/* 页头 + Tab 切换 */}
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('featureDs.title')}</h2>
          <p className="text-xs text-gray-500">{t('featureDs.subtitle')}</p>
        </div>
        <div className="flex items-center rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
          <button
            onClick={() => setActiveTab('feature')}
            className={`px-4 py-2 text-xs font-medium transition-colors ${
              activeTab === 'feature' ? 'bg-blue-600 text-white' : 'bg-gray-50 dark:bg-gray-800 text-gray-500 hover:text-gray-700'
            }`}
          >
            {t('featureDs.tabFeature')}
          </button>
          <button
            onClick={() => setActiveTab('voteSync')}
            className={`px-4 py-2 text-xs font-medium transition-colors ${
              activeTab === 'voteSync' ? 'bg-blue-600 text-white' : 'bg-gray-50 dark:bg-gray-800 text-gray-500 hover:text-gray-700'
            }`}
          >
            {t('featureDs.tabVoteSync')}
          </button>
        </div>
      </div>

      {/* ============================ 特征数据源 Tab ============================ */}
      {activeTab === 'feature' && (
        <>
          {/* 查询条件 */}
          <Card>
            <div className="flex flex-wrap items-end gap-4 text-xs">
              <div>
                <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('featureDs.project')}</label>
                <select
                  value={queryProjectId}
                  onChange={(e) => setQueryProjectId(e.target.value)}
                  className="p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500 min-w-[180px]"
                >
                  <option value="">{t('featureDs.selectProject')}</option>
                  {projects.map((p) => (
                    <option key={p.projectId} value={p.projectId}>{p.projectName}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('featureDs.node')}</label>
                <select
                  value={queryNodeId}
                  onChange={(e) => setQueryNodeId(e.target.value)}
                  className="p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500 min-w-[160px]"
                >
                  <option value="">{t('featureDs.selectNode')}</option>
                  {nodes.map((n) => (
                    <option key={n.nodeId} value={n.nodeId}>{n.nodeName || n.nodeId}</option>
                  ))}
                </select>
              </div>
              <Button variant="primary" onClick={() => setHasQueried(true)} disabled={!queryProjectId || !queryNodeId} loading={featureListQuery.isLoading}>
                {t('featureDs.query')}
              </Button>
              <Button variant="outline" onClick={() => { resetCreateForm(); setCreateOpen(true); }}>
                ＋ {t('featureDs.create')}
              </Button>
            </div>
          </Card>

          {/* 错误提示 */}
          {queryError && (
            <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
              {t('common.error', { message: queryError })}
            </div>
          )}

          {/* 特征表列表 */}
          {hasQueried && !queryError && (
            <div className="space-y-4">
              {featureList.length === 0 && !featureListQuery.isLoading && (
                <div className="text-center text-xs text-gray-400 py-10">{t('featureDs.noData')}</div>
              )}
              {featureList.map((ds) => (
                <Card key={ds.featureTableId || ds.featureTableName}>
                  <div className="flex items-center justify-between mb-3">
                    <div>
                      <h3 className="font-bold text-sm text-gray-900 dark:text-gray-100">{ds.featureTableName || '-'}</h3>
                      <div className="text-[10px] text-gray-400 font-mono mt-0.5">
                        {t('featureDs.featureTableId')}: {ds.featureTableId || '-'} • {ds.nodeId || '-'}
                      </div>
                    </div>
                    <Badge status="default">{(ds.columns?.length ?? 0)} {t('featureDs.columns')}</Badge>
                  </div>
                  {/* 字段表格 */}
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-xs">
                      <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
                        <tr>
                          <th className="p-2.5">{t('featureDs.colName')}</th>
                          <th className="p-2.5">{t('featureDs.colType')}</th>
                          <th className="p-2.5">{t('featureDs.colComment')}</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
                        {(ds.columns ?? []).length === 0 && (
                          <tr>
                            <td colSpan={3} className="p-2.5 text-center text-gray-400">-</td>
                          </tr>
                        )}
                        {(ds.columns ?? []).map((col, idx) => (
                          <tr key={idx} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                            <td className="p-2.5 font-mono text-blue-600 dark:text-blue-400">{col.colName || '-'}</td>
                            <td className="p-2.5 font-mono text-gray-500">{col.colType || '-'}</td>
                            <td className="p-2.5 text-gray-500">{col.colComment || '-'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </Card>
              ))}
            </div>
          )}
        </>
      )}

      {/* ============================ 投票同步 Tab ============================ */}
      {activeTab === 'voteSync' && (
        <Card>
          <div className="mb-4">
            <h3 className="font-bold text-sm text-gray-900 dark:text-gray-100">{t('featureDs.voteSyncTitle')}</h3>
            <p className="text-xs text-gray-500 mt-0.5">{t('featureDs.voteSyncSubtitle')}</p>
          </div>

          <div className="space-y-4 text-xs">
            {/* 项目选择 */}
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
                {t('featureDs.project')} <span className="text-red-500">*</span>
              </label>
              <select
                value={syncProjectId}
                onChange={(e) => setSyncProjectId(e.target.value)}
                className="w-full md:w-1/2 p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              >
                <option value="">{t('featureDs.selectProject')}</option>
                {projects.map((p) => (
                  <option key={p.projectId} value={p.projectId}>{p.projectName}</option>
                ))}
              </select>
            </div>

            {/* 同步数据类型 */}
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
                {t('featureDs.syncDataType')} <span className="text-red-500">*</span>
              </label>
              <select
                value={syncType}
                onChange={(e) => setSyncType(e.target.value)}
                className="w-full md:w-1/2 p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
              >
                <option value="">{t('featureDs.selectSyncType')}</option>
                {SYNC_DATA_TYPES.map((type) => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </div>

            {/* 参与节点多选 */}
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
                {t('featureDs.syncNodes')} <span className="text-red-500">*</span>
              </label>
              <div className="flex flex-wrap gap-2">
                {nodes.map((n) => {
                  const checked = syncNodeIds.includes(n.nodeId);
                  return (
                    <label
                      key={n.nodeId}
                      className={`flex items-center gap-2 px-3 py-2 rounded-lg border cursor-pointer select-none transition-colors ${
                        checked
                          ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300'
                          : 'border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 hover:border-blue-400'
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggleSyncNode(n.nodeId)}
                        className="w-3.5 h-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                      />
                      <span className="font-medium">{n.nodeName || n.nodeId}</span>
                    </label>
                  );
                })}
              </div>
              {syncNodeIds.length === 0 && <div className="text-[11px] text-gray-400 mt-1">{t('featureDs.selectNodes')}</div>}
            </div>

            {/* 提交按钮 */}
            <div className="pt-2">
              <Button
                variant="primary"
                onClick={() => syncMutation.mutate()}
                disabled={!syncProjectId || !syncType || syncNodeIds.length === 0}
                loading={syncMutation.isPending}
              >
                {t('featureDs.submitSync')}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* ============================ 创建特征数据源 Modal ============================ */}
      <Modal
        isOpen={createOpen}
        onClose={() => { setCreateOpen(false); resetCreateForm(); }}
        title={t('featureDs.createTitle')}
        width="max-w-2xl"
        footer={
          <>
            <Button variant="ghost" onClick={() => { setCreateOpen(false); resetCreateForm(); }}>{t('common.cancel')}</Button>
            <Button
              variant="primary"
              onClick={() => createMutation.mutate()}
              loading={createMutation.isPending}
              disabled={!featureTableName.trim() || !dsUrl.trim() || selectedNodeIds.length === 0 || columnRows.every((r) => !r.colName.trim())}
            >
              {t('common.confirm')}
            </Button>
          </>
        }
      >
        <div className="text-xs space-y-4 max-h-[65vh] overflow-y-auto pr-1">
          {/* 基本信息 */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
                {t('featureDs.featureTableName')} <span className="text-red-500">*</span>
              </label>
              <input
                type="text"
                value={featureTableName}
                onChange={(e) => setFeatureTableName(e.target.value)}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                required
              />
            </div>
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('featureDs.type')}</label>
              <select
                value={dsType}
                onChange={(e) => setDsType(e.target.value)}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
              >
                {FEATURE_DS_TYPES.map((type) => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </div>
          </div>

          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
              {t('featureDs.url')} <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={dsUrl}
              onChange={(e) => setDsUrl(e.target.value)}
              placeholder="jdbc:mysql://host:3306/db 或 /path/to/file.csv"
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('featureDs.desc')}</label>
            <input
              type="text"
              value={dsDesc}
              onChange={(e) => setDsDesc(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 授权节点多选 */}
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
              {t('featureDs.nodeIds')} <span className="text-red-500">*</span>
            </label>
            <div className="flex flex-wrap gap-2">
              {nodes.map((n) => {
                const checked = selectedNodeIds.includes(n.nodeId);
                return (
                  <label
                    key={n.nodeId}
                    className={`flex items-center gap-2 px-3 py-2 rounded-lg border cursor-pointer select-none transition-colors ${
                      checked
                        ? 'border-blue-500 bg-blue-50 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300'
                        : 'border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 hover:border-blue-400'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleNodeId(n.nodeId)}
                      className="w-3.5 h-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                    />
                    <span className="font-medium">{n.nodeName || n.nodeId}</span>
                  </label>
                );
              })}
            </div>
          </div>

          {/* 字段动态编辑 */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="font-semibold text-gray-700 dark:text-gray-300">{t('featureDs.columns')}</label>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setColumnRows((prev) => [...prev, { colName: '', colType: 'string', colComment: '' }])}
              >
                ＋ {t('featureDs.addColumn')}
              </Button>
            </div>
            <div className="space-y-2">
              {columnRows.map((row, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <input
                    type="text"
                    value={row.colName}
                    onChange={(e) => updateColumnRow(idx, 'colName', e.target.value)}
                    placeholder={t('featureDs.colName')}
                    className="flex-1 p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
                  />
                  <input
                    type="text"
                    value={row.colType}
                    onChange={(e) => updateColumnRow(idx, 'colType', e.target.value)}
                    placeholder={t('featureDs.colType')}
                    className="w-24 p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono focus:outline-none focus:border-blue-500"
                  />
                  <input
                    type="text"
                    value={row.colComment}
                    onChange={(e) => updateColumnRow(idx, 'colComment', e.target.value)}
                    placeholder={t('featureDs.colComment')}
                    className="flex-1 p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                  />
                  <button
                    onClick={() => setColumnRows((prev) => prev.filter((_, i) => i !== idx))}
                    className="text-gray-400 hover:text-red-500 text-sm flex-shrink-0"
                    aria-label={t('featureDs.removeColumn')}
                  >
                    ✕
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
};
