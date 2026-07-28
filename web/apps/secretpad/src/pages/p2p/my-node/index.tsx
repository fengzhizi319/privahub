import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { NodeRouterVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../../shared/lib/i18n';

export const P2pMyNodePage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [error, setError] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<NodeRouterVO | null>(null);

  const [name, setName] = useState('');
  const [srcNodeId, setSrcNodeId] = useState('');
  const [dstNodeId, setDstNodeId] = useState('');
  const [dstInstId, setDstInstId] = useState('');
  const [dstInstName, setDstInstName] = useState('');
  const [srcNetAddress, setSrcNetAddress] = useState('');
  const [dstNetAddress, setDstNetAddress] = useState('');
  const [certText, setCertText] = useState('');

  const routesQuery = useQuery({
    queryKey: ['p2p-nodes'],
    queryFn: () => apiClient.listNodeRoutes({ pageNumber: 1, pageSize: 200 }),
  });
  const routes = routesQuery.data?.data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['p2p-nodes'] });

  const createMutation = useMutation({
    mutationFn: () =>
      apiClient.createP2pNode({
        name: name || undefined,
        mode: 0,
        srcNodeId,
        dstNodeId,
        dstInstId: dstInstId || undefined,
        dstInstName: dstInstName || undefined,
        srcNetAddress: srcNetAddress || undefined,
        dstNetAddress,
        certText,
      }),
    onSuccess: () => {
      setIsModalOpen(false);
      resetForm();
      invalidate();
      toast.success(t('common.save'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (routerId: string) => apiClient.deleteP2pNode(routerId),
    onSuccess: () => {
      setDeleteTarget(null);
      invalidate();
      toast.success(t('common.delete'));
    },
    onError: (e) => {
      setDeleteTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const resetForm = () => {
    setName('');
    setSrcNodeId('');
    setDstNodeId('');
    setDstInstId('');
    setDstInstName('');
    setSrcNetAddress('');
    setDstNetAddress('');
    setCertText('');
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    createMutation.mutate();
  };

  const nodeName = (node?: Record<string, unknown>, id?: string) =>
    (node?.nodeName as string) || (node?.name as string) || id || '-';

  const field = (label: string, value: string, setter: (v: string) => void, required = false, textarea = false) => (
    <div>
      <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{label}</label>
      {textarea ? (
        <textarea
          value={value}
          onChange={(e) => setter(e.target.value)}
          rows={4}
          required={required}
          className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none focus:border-blue-500"
        />
      ) : (
        <input
          type="text"
          value={value}
          onChange={(e) => setter(e.target.value)}
          required={required}
          className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
        />
      )}
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('p2p.myNodeTitle')}</h2>
          <p className="text-xs text-gray-500">{t('p2p.myNodeSubtitle')}</p>
        </div>
        <Button variant="primary" icon={<span>＋</span>} onClick={() => setIsModalOpen(true)}>{t('p2p.createNode')}</Button>
      </div>

      {(error || routesQuery.error) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || routesQuery.error?.message || '' })}
        </div>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('nodeRoutes.srcNode')}</th>
                <th className="p-4">{t('nodeRoutes.dstNode')}</th>
                <th className="p-4">{t('nodeRoutes.dstNetAddress')}</th>
                <th className="p-4">{t('nodeRoutes.status')}</th>
                <th className="p-4">{t('common.action') || 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {routes.length === 0 && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">{t('p2p.noNodes')}</td>
                </tr>
              )}
              {routes.map((route) => (
                <tr key={route.routeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{nodeName(route.srcNode, route.srcNodeId)}</td>
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{nodeName(route.dstNode, route.dstNodeId)}</td>
                  <td className="p-4 font-mono text-gray-500">{route.dstNetAddress || '-'}</td>
                  <td className="p-4">
                    <Badge status={route.status === 'Succeeded' || route.status === 'Ready' ? 'success' : 'default'}>
                      {route.status || '-'}
                    </Badge>
                  </td>
                  <td className="p-4">
                    <Button size="sm" variant="danger" onClick={() => setDeleteTarget(route)}>{t('p2p.deleteNode')}</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Create Node Modal */}
      <Modal
        isOpen={isModalOpen}
        onClose={() => { setIsModalOpen(false); resetForm(); }}
        title={t('p2p.createNodeTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsModalOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleSubmit} loading={createMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          {field(t('p2p.nodeName'), name, setName)}
          {field(t('p2p.srcNodeId'), srcNodeId, setSrcNodeId, true)}
          {field(t('p2p.dstNodeId'), dstNodeId, setDstNodeId, true)}
          {field(t('p2p.dstInstId'), dstInstId, setDstInstId)}
          {field(t('p2p.dstInstName'), dstInstName, setDstInstName)}
          {field(t('p2p.srcNetAddress'), srcNetAddress, setSrcNetAddress)}
          {field(t('p2p.dstNetAddress'), dstNetAddress, setDstNetAddress, true)}
          {field(t('p2p.certText'), certText, setCertText, true, true)}
        </form>
      </Modal>

      {/* Delete Confirm */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('p2p.deleteNode')}
        message={t('p2p.deleteNodeConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget?.routeId && deleteMutation.mutate(deleteTarget.routeId)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
