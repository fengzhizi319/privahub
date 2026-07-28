import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { NodeRouterVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

export const NodeRoutesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<NodeRouterVO | null>(null);
  const [srcNetAddress, setSrcNetAddress] = useState('');
  const [dstNetAddress, setDstNetAddress] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<NodeRouterVO | null>(null);

  const routesQuery = useQuery({
    queryKey: ['node-routes'],
    queryFn: () => apiClient.listNodeRoutes({ pageNumber: 1, pageSize: 200 }),
  });
  const routes = routesQuery.data?.data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['node-routes'] });

  const updateMutation = useMutation({
    mutationFn: (input: { routerId: string; srcNetAddress?: string; dstNetAddress?: string }) =>
      apiClient.updateNodeRoute(input),
    onSuccess: () => {
      setEditing(null);
      invalidate();
      toast.success(t('common.save'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (routerId: string) => apiClient.deleteNodeRoute(routerId),
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

  const refreshMutation = useMutation({
    mutationFn: (routerId: string) => apiClient.refreshNodeRoute(routerId),
    onSuccess: () => {
      invalidate();
      toast.success(t('nodeRoutes.refreshSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const openEdit = (route: NodeRouterVO) => {
    setEditing(route);
    setSrcNetAddress(route.srcNetAddress || '');
    setDstNetAddress(route.dstNetAddress || '');
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editing?.routeId) return;
    setError(null);
    updateMutation.mutate({ routerId: editing.routeId, srcNetAddress, dstNetAddress });
  };

  const nodeName = (node?: Record<string, unknown>, id?: string) =>
    (node?.nodeName as string) || (node?.name as string) || id || '-';

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('nodeRoutes.title')}</h2>
          <p className="text-xs text-gray-500">{t('nodeRoutes.subtitle')}</p>
        </div>
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
                <th className="p-4">{t('nodeRoutes.srcNetAddress')}</th>
                <th className="p-4">{t('nodeRoutes.dstNetAddress')}</th>
                <th className="p-4">{t('nodeRoutes.routeType')}</th>
                <th className="p-4">{t('nodeRoutes.status')}</th>
                <th className="p-4">{t('common.action') || 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {routes.length === 0 && (
                <tr>
                  <td colSpan={7} className="p-4 text-center text-gray-400">{t('nodeRoutes.noRoutes')}</td>
                </tr>
              )}
              {routes.map((route) => (
                <tr key={route.routeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{nodeName(route.srcNode, route.srcNodeId)}</td>
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{nodeName(route.dstNode, route.dstNodeId)}</td>
                  <td className="p-4 font-mono text-gray-500">{route.srcNetAddress || '-'}</td>
                  <td className="p-4 font-mono text-gray-500">{route.dstNetAddress || '-'}</td>
                  <td className="p-4">
                    <span className="px-2 py-0.5 rounded font-mono bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300">
                      {route.routeType || '-'}
                    </span>
                  </td>
                  <td className="p-4">
                    <Badge status={route.status === 'Succeeded' || route.status === 'Ready' ? 'success' : 'default'}>
                      {route.status || '-'}
                    </Badge>
                  </td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button size="sm" variant="ghost" onClick={() => route.routeId && refreshMutation.mutate(route.routeId)}>
                        {t('nodeRoutes.refresh')}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => openEdit(route)}>{t('nodeRoutes.edit')}</Button>
                      <Button size="sm" variant="danger" onClick={() => setDeleteTarget(route)}>{t('nodeRoutes.delete')}</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <Modal
        isOpen={!!editing}
        onClose={() => setEditing(null)}
        title={t('nodeRoutes.editTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditing(null)}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleSubmit} loading={updateMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('nodeRoutes.srcNetAddress')}</label>
            <input
              type="text"
              value={srcNetAddress}
              onChange={(e) => setSrcNetAddress(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('nodeRoutes.dstNetAddress')}</label>
            <input
              type="text"
              value={dstNetAddress}
              onChange={(e) => setDstNetAddress(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('nodeRoutes.delete')}
        message={t('nodeRoutes.deleteConfirm')}
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
