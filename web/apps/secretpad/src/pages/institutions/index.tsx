import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { Node, InstTokenVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';

export const InstitutionsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { ownerId } = usePlatform();

  const [error, setError] = useState<string | null>(null);
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [nodeName, setNodeName] = useState('');
  const [netAddress, setNetAddress] = useState('');
  const [description, setDescription] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Node | null>(null);
  const [tokenNode, setTokenNode] = useState<Node | null>(null);
  const [tokenInfo, setTokenInfo] = useState<InstTokenVO | null>(null);

  const instQuery = useQuery({
    queryKey: ['inst', ownerId],
    queryFn: () => apiClient.getInst(ownerId || ''),
    enabled: !!ownerId,
  });

  const nodesQuery = useQuery({
    queryKey: ['inst-nodes'],
    queryFn: () => apiClient.listInstNodes(),
  });
  const nodes = nodesQuery.data ?? [];

  const invalidateNodes = () => queryClient.invalidateQueries({ queryKey: ['inst-nodes'] });

  const addMutation = useMutation({
    mutationFn: (input: { name: string; netAddress?: string; description?: string }) =>
      apiClient.addInstNode(input),
    onSuccess: () => {
      setIsAddOpen(false);
      resetForm();
      invalidateNodes();
      toast.success(t('common.save'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (nodeId: string) => apiClient.deleteInstNode(nodeId),
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

  const resetForm = () => {
    setNodeName('');
    setNetAddress('');
    setDescription('');
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    addMutation.mutate({ name: nodeName, netAddress, description });
  };

  const handleViewToken = async (node: Node) => {
    setTokenNode(node);
    setTokenInfo(null);
    try {
      const res = await apiClient.getInstNodeToken(node.nodeId);
      setTokenInfo(res);
    } catch (e) {
      setTokenInfo({ instToken: e instanceof Error ? e.message : String(e), instTokenState: 'ERROR' });
    }
  };

  const handleCopyToken = async () => {
    if (!tokenInfo?.instToken) return;
    try {
      await navigator.clipboard.writeText(tokenInfo.instToken);
      toast.success(t('institutions.tokenCopied'));
    } catch {
      toast.error(t('institutions.tokenCopied'));
    }
  };

  const inst = instQuery.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('institutions.title')}</h2>
          <p className="text-xs text-gray-500">{t('institutions.subtitle')}</p>
        </div>
        <Button variant="primary" icon={<span>＋</span>} onClick={() => setIsAddOpen(true)}>{t('institutions.addNode')}</Button>
      </div>

      {(error || instQuery.error) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || instQuery.error?.message || '' })}
        </div>
      )}

      {/* Institution Info */}
      <Card>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.instName')}</div>
            <div className="font-semibold text-gray-800 dark:text-gray-200">{inst?.instName || '-'}</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.instId')}</div>
            <div className="font-mono text-gray-600 dark:text-gray-300">{inst?.instId || '-'}</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.localNode')}</div>
            <div className="font-mono text-gray-600 dark:text-gray-300">{inst?.localNodeId || '-'}</div>
          </div>
        </div>
      </Card>

      {/* Institution Nodes */}
      <Card bodyClassName="p-0">
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 text-sm font-semibold text-gray-700 dark:text-gray-200">
          {t('institutions.nodeList')}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('institutions.nodeName')}</th>
                <th className="p-4">{t('institutions.nodeId')}</th>
                <th className="p-4">{t('institutions.status')}</th>
                <th className="p-4">{t('institutions.createTime')}</th>
                <th className="p-4">{t('common.action') || 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {nodes.length === 0 && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">{t('institutions.noNodes')}</td>
                </tr>
              )}
              {nodes.map((node) => (
                <tr key={node.nodeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{node.nodeName}</td>
                  <td className="p-4 font-mono text-gray-500">{node.nodeId}</td>
                  <td className="p-4">
                    <Badge status={node.nodeStatus === 'Ready' ? 'success' : 'default'}>{node.nodeStatus || '-'}</Badge>
                  </td>
                  <td className="p-4 text-gray-400">{node.gmtCreate || '-'}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="ghost" onClick={() => handleViewToken(node)}>{t('institutions.token')}</Button>
                      <Button size="sm" variant="danger" onClick={() => setDeleteTarget(node)}>{t('institutions.deleteNode')}</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Add Node Modal */}
      <Modal
        isOpen={isAddOpen}
        onClose={() => { setIsAddOpen(false); resetForm(); }}
        title={t('institutions.addNodeTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsAddOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleSubmit} loading={addMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('institutions.nodeName')}</label>
            <input
              type="text"
              value={nodeName}
              onChange={(e) => setNodeName(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('institutions.netAddress')}</label>
            <input
              type="text"
              value={netAddress}
              onChange={(e) => setNetAddress(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('institutions.description')}</label>
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
        </form>
      </Modal>

      {/* Token Modal */}
      <Modal
        isOpen={!!tokenNode}
        onClose={() => { setTokenNode(null); setTokenInfo(null); }}
        title={tokenNode ? `${t('institutions.tokenTitle')} - ${tokenNode.nodeName}` : t('institutions.tokenTitle')}
        footer={
          <>
            <Button variant="outline" onClick={handleCopyToken} disabled={!tokenInfo?.instToken}>📋 {t('institutions.copyToken')}</Button>
            <Button variant="primary" onClick={() => { setTokenNode(null); setTokenInfo(null); }}>{t('common.close')}</Button>
          </>
        }
      >
        <div className="text-xs space-y-2">
          <div className="font-semibold text-gray-700 dark:text-gray-300">Status: <span className="font-normal">{tokenInfo?.instTokenState || '...'}</span></div>
          <textarea
            readOnly
            value={tokenInfo?.instToken || ''}
            rows={6}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none"
          />
        </div>
      </Modal>

      {/* Delete Confirm */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('institutions.deleteNode')}
        message={t('institutions.deleteNodeConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.nodeId)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
