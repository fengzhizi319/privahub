import React, { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { DataSource, CreateDataSourceInput } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';

const DEFAULT_TYPES = ['local', 'odps', 'mysql', 'postgres'];

export const DataSourcesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [selectedNodeId, setSelectedNodeId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DataSource | null>(null);

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [dsName, setDsName] = useState('');
  const [dsType, setDsType] = useState('local');
  const [dsInfo, setDsInfo] = useState('{}');

  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: () => apiClient.getNodes(),
  });
  const nodes = nodesQuery.data ?? [];

  // Default the selected node to the first one once nodes load.
  useEffect(() => {
    if (!selectedNodeId && nodes.length > 0) {
      setSelectedNodeId(nodes[0].nodeId);
    }
  }, [nodes, selectedNodeId]);

  const sourcesQuery = useQuery({
    queryKey: ['datasources', selectedNodeId],
    queryFn: () => apiClient.getDataSources(selectedNodeId),
    enabled: !!selectedNodeId,
  });
  const sources = sourcesQuery.data ?? [];

  const invalidateSources = () =>
    queryClient.invalidateQueries({ queryKey: ['datasources', selectedNodeId] });

  const createMutation = useMutation({
    mutationFn: () => {
      let info: Record<string, any>;
      try {
        info = JSON.parse(dsInfo);
      } catch {
        return Promise.reject(new Error(t('dataSources.infoInvalid')));
      }
      const input: CreateDataSourceInput = {
        ownerId: selectedNodeId,
        nodeIds: [selectedNodeId],
        type: dsType,
        name: dsName,
        info,
      };
      return apiClient.createDataSource(input);
    },
    onSuccess: () => {
      setIsModalOpen(false);
      resetForm();
      invalidateSources();
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteMutation = useMutation({
    mutationFn: (ds: DataSource) => apiClient.deleteDataSource(selectedNodeId, ds.datasourceId, ds.type),
    onSuccess: () => {
      setDeleteTarget(null);
      invalidateSources();
      toast.success(t('dataSources.deleteSuccess'));
    },
    onError: (e) => {
      setDeleteTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const resetForm = () => {
    setDsName('');
    setDsType('local');
    setDsInfo('{}');
  };

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    createMutation.mutate();
  };

  const handleDelete = (ds: DataSource) => {
    setDeleteTarget(ds);
  };

  const handleDetail = (ds: DataSource) => {
    navigate({
      to: '/data-sources/detail',
      search: { ownerId: selectedNodeId, datasourceId: ds.datasourceId, type: ds.type },
    });
  };

  const loading = sourcesQuery.isLoading;
  const queryError = sourcesQuery.error?.message || nodesQuery.error?.message || null;

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataSources.title')}</h2>
          <p className="text-xs text-gray-500">{t('dataSources.subtitle')}</p>
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
            <Button variant="primary" icon={<span>＋</span>} onClick={() => setIsModalOpen(true)}>{t('dataSources.add')}</Button>
          </AccessGuard>
        </div>
      </div>

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {sources.map((ds) => (
          <Card key={ds.datasourceId}>
            <div className="flex items-center justify-between mb-2">
              <h3 className="font-bold text-sm text-gray-900 dark:text-gray-100">{ds.name}</h3>
              <div className="flex items-center gap-2">
                <Badge status="success">{ds.status || 'Available'}</Badge>
                <Button size="sm" variant="ghost" onClick={() => handleDetail(ds)}>{t('dataSources.detail')}</Button>
                <AccessGuard access={{ types: [Platform.CENTER] }}>
                  <Button size="sm" variant="danger" onClick={() => handleDelete(ds)}>{t('common.delete')}</Button>
                </AccessGuard>
              </div>
            </div>
            <div className="text-xs text-gray-500 space-y-1 font-mono">
              <div>{t('dataSources.id')}: {ds.datasourceId}</div>
              <div>{t('dataSources.type')}: {ds.type}</div>
              <div>{t('dataSources.nodes')}: {ds.nodes.map((n) => n.nodeName || n.nodeId).join(', ')}</div>
            </div>
          </Card>
        ))}
      </div>

      {sources.length === 0 && !loading && !queryError && (
        <div className="text-center text-xs text-gray-400 py-10">{t('dataSources.noData')}</div>
      )}

      <Modal
        isOpen={isModalOpen}
        onClose={() => { setIsModalOpen(false); resetForm(); }}
        title={t('dataSources.modalAddTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsModalOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleCreate} loading={createMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleCreate} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataSources.nameLabel')}</label>
            <input
              type="text"
              value={dsName}
              onChange={(e) => setDsName(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataSources.typeLabel')}</label>
            <select
              value={dsType}
              onChange={(e) => setDsType(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              {DEFAULT_TYPES.map((type) => (
                <option key={type} value={type}>{type.toUpperCase()}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dataSources.infoLabel')} (JSON)</label>
            <textarea
              value={dsInfo}
              onChange={(e) => setDsInfo(e.target.value)}
              rows={4}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 font-mono text-[10px] focus:outline-none focus:border-blue-500"
            />
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('dataSources.delete')}
        message={t('dataSources.deleteConfirm')}
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
