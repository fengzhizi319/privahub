import React, { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { Node, CreateNodeInput, UpdateNodeInput, NodeAllResultsVO, NodeResultDetailVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';

const MODE_MAP: Record<string, number> = { MPC: 0, TEE: 1, BOTH: 2 };

/**
 * Kuscia 节点管理页面。
 *
 * 设计要点：
 * 1. 节点列表支持按名称 / ID / 通讯地址搜索，支持注册、编辑、刷新、删除、查看 Token。
 * 2. 点击节点名称打开详情抽屉，包含三个标签：
 *    - 基本信息：节点名、ID、类型、状态、地址、注册时间、描述。
 *    - 部署令牌：展示 Token 状态与内容，支持复制到剪贴板和重新生成。
 *    - 节点产物：调用 `node/result/list` 获取该节点产出的模型/规则等数据，支持查看产物详情。
 * 3. 托管/内置节点在列表中显示标签，并在详情中区分展示。
 * 4. 所有变更操作均通过 TanStack Query mutation + invalidateQueries 刷新相关缓存。
 */
export const NodesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingNode, setEditingNode] = useState<Node | null>(null);
  const [nodeName, setNodeName] = useState('');
  const [nodeMode, setNodeMode] = useState<'MPC' | 'TEE' | 'BOTH'>('MPC');
  const [netAddress, setNetAddress] = useState('');
  const [tokenModalNode, setTokenModalNode] = useState<Node | null>(null);
  const [token, setToken] = useState<{ token?: string; tokenStatus?: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Node | null>(null);
  const [resetTokenTarget, setResetTokenTarget] = useState<Node | null>(null);

  // Detail drawer state
  const [detailNode, setDetailNode] = useState<Node | null>(null);
  const [detailTab, setDetailTab] = useState<'basic' | 'token' | 'results'>('basic');

  // Node result detail modal state
  const [resultDetail, setResultDetail] = useState<NodeResultDetailVO | null>(null);

  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });

  const filteredNodes = useMemo(() => {
    const data = nodesQuery.data ?? [];
    const s = search.trim().toLowerCase();
    if (!s) return data;
    return data.filter(
      (n) =>
        n.nodeName.toLowerCase().includes(s) ||
        n.nodeId.toLowerCase().includes(s) ||
        (n.netAddress && n.netAddress.toLowerCase().includes(s))
    );
  }, [nodesQuery.data, search]);

  const invalidateNodes = () => queryClient.invalidateQueries({ queryKey: ['nodes'] });

  const detailQuery = useQuery({
    queryKey: ['node-detail', detailNode?.nodeId],
    queryFn: () => apiClient.getNode(detailNode!.nodeId),
    enabled: !!detailNode,
  });
  const nodeInfo = detailQuery.data ?? detailNode;

  const resultsQuery = useQuery({
    queryKey: ['node-results', detailNode?.nodeId],
    queryFn: () =>
      apiClient.listNodeResults({
        ownerId: detailNode!.nodeId,
        pageSize: 50,
        pageNumber: 1,
      }),
    enabled: !!detailNode && detailTab === 'results',
  });
  const resultsList = resultsQuery.data?.nodeAllResultsVOList ?? [];

  const resultDetailQuery = useQuery({
    queryKey: ['node-result-detail', detailNode?.nodeId, resultDetail?.nodeResultsVO?.domainDataId],
    queryFn: () =>
      apiClient.getNodeResultDetail({
        nodeId: detailNode!.nodeId,
        domainDataId: resultDetail!.nodeResultsVO!.domainDataId!,
        dataType: resultDetail?.nodeResultsVO?.datatableType,
      }),
    enabled: !!detailNode && !!resultDetail?.nodeResultsVO?.domainDataId,
  });

  const saveMutation = useMutation({
    mutationFn: (input: { editing: Node | null }) => {
      if (input.editing) {
        const update: UpdateNodeInput = { nodeId: input.editing.nodeId, netAddress };
        return apiClient.updateNode(update);
      }
      const create: CreateNodeInput = { name: nodeName, mode: MODE_MAP[nodeMode] };
      return apiClient.createNode(create);
    },
    onSuccess: () => {
      setIsModalOpen(false);
      resetForm();
      invalidateNodes();
      toast.success(t('common.save'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (nodeId: string) => apiClient.deleteNode(nodeId),
    onSuccess: () => {
      setDeleteTarget(null);
      invalidateNodes();
      toast.success(t('common.delete'));
    },
    onError: (e) => {
      setDeleteTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const refreshMutation = useMutation({
    mutationFn: (nodeId: string) => apiClient.refreshNode(nodeId),
    onSuccess: invalidateNodes,
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const resetTokenMutation = useMutation({
    mutationFn: (nodeId: string) => apiClient.newNodeToken(nodeId),
    onSuccess: (res) => {
      setResetTokenTarget(null);
      setToken(res);
      toast.success(t('nodes.resetTokenSuccess'));
    },
    onError: (e) => {
      setResetTokenTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const resetForm = () => {
    setNodeName('');
    setNodeMode('MPC');
    setNetAddress('');
    setEditingNode(null);
  };

  const openRegister = () => {
    resetForm();
    setIsModalOpen(true);
  };

  const openEdit = (node: Node) => {
    setEditingNode(node);
    setNodeName(node.nodeName);
    setNodeMode('MPC');
    setNetAddress(node.netAddress || '');
    setIsModalOpen(true);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    saveMutation.mutate({ editing: editingNode });
  };

  const handleRefresh = (node: Node) => {
    refreshMutation.mutate(node.nodeId);
  };

  const handleViewToken = async (node: Node) => {
    setTokenModalNode(node);
    setToken(null);
    try {
      const res = await apiClient.getNodeToken(node.nodeId);
      setToken(res);
    } catch (e) {
      setToken({ token: e instanceof Error ? e.message : String(e), tokenStatus: 'ERROR' });
    }
  };

  const handleCopyToken = async () => {
    if (!token?.token) return;
    try {
      await navigator.clipboard.writeText(token.token);
      toast.success(t('nodes.tokenCopied'));
    } catch {
      toast.error(t('nodes.tokenCopied'));
    }
  };

  const openDetail = (node: Node) => {
    setDetailNode(node);
    setDetailTab('basic');
    setResultDetail(null);
  };

  const closeDetail = () => {
    setDetailNode(null);
    setResultDetail(null);
  };

  const viewResultDetail = (result: NodeAllResultsVO) => {
    if (result.nodeResultsVO) {
      setResultDetail({ nodeResultsVO: result.nodeResultsVO } as NodeResultDetailVO);
    }
  };

  const closeResultDetail = () => {
    setResultDetail(null);
  };

  return (
    <div className="space-y-6">
      {/* Header Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('nodes.title')}</h2>
          <p className="text-xs text-gray-500">{t('nodes.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <input
            type="text"
            placeholder={t('nodes.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          />
          <AccessGuard access={{ types: [Platform.CENTER] }}>
            <Button variant="primary" icon={<span>＋</span>} onClick={openRegister}>{t('nodes.register')}</Button>
          </AccessGuard>
        </div>
      </div>

      {(error || nodesQuery.error) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || nodesQuery.error?.message || '' })}
        </div>
      )}

      {/* Nodes Table */}
      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('nodes.name')}</th>
                <th className="p-4">{t('nodes.id')}</th>
                <th className="p-4">{t('nodes.type')}</th>
                <th className="p-4">{t('nodes.status')}</th>
                <th className="p-4">{t('nodes.netAddress')}</th>
                <th className="p-4">{t('nodes.registerTime')}</th>
                <th className="p-4">{t('common.action') || 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {filteredNodes.length === 0 && (
                <tr>
                  <td colSpan={7} className="p-4 text-center text-gray-400">{t('nodes.noNodes')}</td>
                </tr>
              )}
              {filteredNodes.map((node) => (
                <tr key={node.nodeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4">
                    <button
                      onClick={() => openDetail(node)}
                      className="font-semibold text-blue-600 dark:text-blue-400 hover:underline text-left"
                    >
                      {node.type === 'embedded' && (
                        <span className="mr-1.5 px-1.5 py-0.5 rounded text-[10px] font-semibold bg-green-100 dark:bg-green-950 text-green-600 dark:text-green-400">
                          {t('nodes.embedded')}
                        </span>
                      )}
                      {node.nodeName}
                    </button>
                  </td>
                  <td className="p-4 font-mono text-gray-500">{node.nodeId}</td>
                  <td className="p-4">
                    <span className="px-2 py-0.5 rounded font-mono bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300">
                      {node.type}
                    </span>
                  </td>
                  <td className="p-4">
                    <Badge status={node.nodeStatus === 'Ready' ? 'success' : 'default'}>
                      {node.nodeStatus}
                    </Badge>
                  </td>
                  <td className="p-4 font-mono text-gray-500">{node.netAddress || '-'}</td>
                  <td className="p-4 text-gray-400">{node.gmtCreate}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="ghost" onClick={() => handleRefresh(node)}>{t('nodes.refresh')}</Button>
                      <AccessGuard access={{ types: [Platform.CENTER] }}>
                        <Button size="sm" variant="ghost" onClick={() => openEdit(node)}>{t('nodes.edit')}</Button>
                        <Button size="sm" variant="ghost" onClick={() => handleViewToken(node)}>{t('nodes.token')}</Button>
                        <Button size="sm" variant="danger" onClick={() => setDeleteTarget(node)}>{t('nodes.delete')}</Button>
                      </AccessGuard>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Register / Edit Modal */}
      <Modal
        isOpen={isModalOpen}
        onClose={() => { setIsModalOpen(false); resetForm(); }}
        title={editingNode ? t('nodes.modalEditTitle') : t('nodes.modalRegisterTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsModalOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleSubmit} loading={saveMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          {!editingNode && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('nodes.nodeNameLabel')}</label>
              <input
                type="text"
                value={nodeName}
                onChange={(e) => setNodeName(e.target.value)}
                placeholder={t('nodes.nodeNamePlaceholder')}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                required
              />
            </div>
          )}

          {!editingNode && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('nodes.modeLabel')}</label>
              <select
                value={nodeMode}
                onChange={(e) => setNodeMode(e.target.value as 'MPC' | 'TEE' | 'BOTH')}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              >
                <option value="MPC">{t('nodes.modeMPC')}</option>
                <option value="TEE">{t('nodes.modeTEE')}</option>
                <option value="BOTH">{t('nodes.modeBoth')}</option>
              </select>
            </div>
          )}

          {editingNode && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('nodes.netAddressLabel')}</label>
              <input
                type="text"
                value={netAddress}
                onChange={(e) => setNetAddress(e.target.value)}
                placeholder={t('nodes.netAddressPlaceholder')}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                required
              />
            </div>
          )}
        </form>
      </Modal>

      {/* Token Modal */}
      <Modal
        isOpen={!!tokenModalNode}
        onClose={() => { setTokenModalNode(null); setToken(null); }}
        title={tokenModalNode ? `${t('nodes.token')} - ${tokenModalNode.nodeName}` : t('nodes.token')}
        footer={
          <>
            <Button variant="outline" onClick={handleCopyToken} disabled={!token?.token}>📋 {t('nodes.copyToken')}</Button>
            <AccessGuard access={{ types: [Platform.CENTER] }}>
              <Button
                variant="ghost"
                onClick={() => tokenModalNode && setResetTokenTarget(tokenModalNode)}
              >
                {t('nodes.resetToken')}
              </Button>
            </AccessGuard>
            <Button variant="primary" onClick={() => { setTokenModalNode(null); setToken(null); }}>{t('common.close')}</Button>
          </>
        }
      >
        <div className="text-xs space-y-2">
          <div className="font-semibold text-gray-700 dark:text-gray-300">Status: <span className="font-normal">{token?.tokenStatus || '...'}</span></div>
          <textarea
            readOnly
            value={token?.token || ''}
            rows={6}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none"
          />
        </div>
      </Modal>

      {/* Detail Drawer */}
      {detailNode && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-black/40" onClick={closeDetail} />
          <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 h-full shadow-2xl overflow-y-auto p-6 space-y-6">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="text-lg font-bold text-gray-900 dark:text-gray-100">
                  {nodeInfo?.nodeName || detailNode.nodeName}
                </h3>
                <p className="text-xs text-gray-500 font-mono mt-1">{nodeInfo?.nodeId || detailNode.nodeId}</p>
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={() => detailQuery.refetch()} loading={detailQuery.isFetching}>
                  {t('nodes.refresh')}
                </Button>
                <Button size="sm" variant="ghost" onClick={closeDetail}>✕</Button>
              </div>
            </div>

            <div className="flex items-center gap-2 text-xs">
              {nodeInfo?.type === 'embedded' && (
                <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-green-100 dark:bg-green-950 text-green-600 dark:text-green-400">
                  {t('nodes.embedded')}
                </span>
              )}
              <Badge status={nodeInfo?.nodeStatus === 'Ready' ? 'success' : 'default'}>{nodeInfo?.nodeStatus || detailNode.nodeStatus}</Badge>
            </div>

            {/* Tabs */}
            <div className="flex items-center gap-4 border-b border-gray-200 dark:border-gray-800 pb-2">
              {(['basic', 'token', 'results'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setDetailTab(tab)}
                  className={`text-sm font-semibold pb-2 ${
                    detailTab === tab ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'
                  }`}
                >
                  {t(`nodes.${tab === 'basic' ? 'basicInfo' : tab === 'token' ? 'deployToken' : 'nodeResults'}` as const)}
                </button>
              ))}
            </div>

            {detailTab === 'basic' && (
              <div className="text-xs space-y-3">
                <div className="grid grid-cols-2 gap-4">
                  <div className="p-3 rounded-lg bg-gray-50 dark:bg-gray-850">
                    <div className="text-gray-500">{t('nodes.type')}</div>
                    <div className="font-semibold text-gray-800 dark:text-gray-200 mt-1">{nodeInfo?.type || detailNode.type}</div>
                  </div>
                  <div className="p-3 rounded-lg bg-gray-50 dark:bg-gray-850">
                    <div className="text-gray-500">{t('nodes.status')}</div>
                    <div className="font-semibold text-gray-800 dark:text-gray-200 mt-1">{nodeInfo?.nodeStatus || detailNode.nodeStatus}</div>
                  </div>
                </div>
                <div className="p-3 rounded-lg bg-gray-50 dark:bg-gray-850">
                  <div className="text-gray-500">{t('nodes.netAddress')}</div>
                  <div className="font-mono text-gray-800 dark:text-gray-200 mt-1">{nodeInfo?.netAddress || detailNode.netAddress || '-'}</div>
                </div>
                <div className="p-3 rounded-lg bg-gray-50 dark:bg-gray-850">
                  <div className="text-gray-500">{t('nodes.registerTime')}</div>
                  <div className="font-mono text-gray-800 dark:text-gray-200 mt-1">{nodeInfo?.gmtCreate || detailNode.gmtCreate}</div>
                </div>
                {nodeInfo?.description && (
                  <div className="p-3 rounded-lg bg-gray-50 dark:bg-gray-850">
                    <div className="text-gray-500">{t('dataSources.info')}</div>
                    <div className="text-gray-800 dark:text-gray-200 mt-1">{nodeInfo.description}</div>
                  </div>
                )}
              </div>
            )}

            {detailTab === 'token' && (
              <div className="space-y-4">
                <div className="text-xs">
                  <span className="text-gray-500">{t('nodes.status')}:</span>{' '}
                  <span className="font-semibold">{token?.tokenStatus || detailQuery.data?.tokenStatus || '-'}</span>
                </div>
                <textarea
                  readOnly
                  value={token?.token || detailQuery.data?.token || ''}
                  rows={8}
                  className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none"
                />
                <div className="flex items-center gap-3">
                  <Button variant="outline" onClick={handleCopyToken} disabled={!(token?.token || detailQuery.data?.token)}>
                    📋 {t('nodes.copyToken')}
                  </Button>
                  <AccessGuard access={{ types: [Platform.CENTER] }}>
                    <Button
                      variant="ghost"
                      onClick={() => detailNode && setResetTokenTarget(detailNode)}
                    >
                      {t('nodes.resetToken')}
                    </Button>
                  </AccessGuard>
                </div>
              </div>
            )}

            {detailTab === 'results' && (
              <div className="space-y-3">
                {resultsQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
                {resultsList.length === 0 && !resultsQuery.isLoading && (
                  <div className="text-xs text-gray-400 text-center py-6">{t('nodes.noResults')}</div>
                )}
                {resultsList.map((item, idx) => {
                  const result = item.nodeResultsVO;
                  return (
                    <div key={idx} className="p-3 rounded-lg border border-gray-200 dark:border-gray-800 text-xs">
                      <div className="flex items-center justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-gray-800 dark:text-gray-200">{result?.productName || result?.domainDataId || '-'}</span>
                          {result?.datatableType && (
                            <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-gray-100 dark:bg-gray-800 text-gray-600">
                              {result.datatableType}
                            </span>
                          )}
                        </div>
                        <Button size="sm" variant="outline" onClick={() => viewResultDetail(item)}>
                          {t('nodes.viewResult')}
                        </Button>
                      </div>
                      <div className="text-gray-400 font-mono">{result?.domainDataId}</div>
                      {result?.sourceProjectName && (
                        <div className="text-gray-500 mt-1">{t('nodes.sourceProject')}: {result.sourceProjectName}</div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Result Detail Modal */}
      <Modal
        isOpen={!!resultDetail}
        onClose={closeResultDetail}
        title={t('nodes.resultDetail')}
        footer={<Button variant="primary" onClick={closeResultDetail}>{t('nodes.close')}</Button>}
      >
        <div className="text-xs space-y-3">
          {resultDetailQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
          {resultDetailQuery.error && <div className="text-red-500">{t('common.error', { message: resultDetailQuery.error.message })}</div>}
          {resultDetailQuery.data && (
            <>
              <div className="p-2 rounded bg-gray-50 dark:bg-gray-800 font-mono text-[10px] overflow-auto">
                {JSON.stringify(resultDetailQuery.data.nodeResultsVO, null, 2)}
              </div>
              {resultDetailQuery.data.output && (
                <div className="p-2 rounded bg-gray-50 dark:bg-gray-800 font-mono text-[10px] overflow-auto">
                  {JSON.stringify(resultDetailQuery.data.output, null, 2)}
                </div>
              )}
            </>
          )}
        </div>
      </Modal>

      {/* Delete Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('nodes.delete')}
        message={t('nodes.deleteConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.nodeId)}
        onCancel={() => setDeleteTarget(null)}
      />

      {/* Reset Token Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!resetTokenTarget}
        title={t('nodes.resetToken')}
        message={t('nodes.resetTokenConfirm')}
        danger
        loading={resetTokenMutation.isPending}
        confirmText={t('nodes.resetToken')}
        cancelText={t('common.cancel')}
        onConfirm={() => resetTokenTarget && resetTokenMutation.mutate(resetTokenTarget.nodeId)}
        onCancel={() => setResetTokenTarget(null)}
      />
    </div>
  );
};
