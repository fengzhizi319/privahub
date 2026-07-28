import React from 'react';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Card, Button, Badge } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

interface DetailSearch {
  ownerId: string;
  datasourceId: string;
  type: string;
}

export const DataSourceDetailPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { ownerId, datasourceId, type } = useSearch({ strict: false }) as DetailSearch;

  const detailQuery = useQuery({
    queryKey: ['datasource-detail', ownerId, datasourceId, type],
    queryFn: () => apiClient.getDataSourceDetail(ownerId, datasourceId, type),
    enabled: !!ownerId && !!datasourceId,
  });

  const nodesQuery = useQuery({
    queryKey: ['datasource-nodes', ownerId, datasourceId],
    queryFn: () => apiClient.getDataSourceNodes(ownerId, datasourceId),
    enabled: !!ownerId && !!datasourceId,
  });

  const detail = detailQuery.data;
  const relatedNodes = nodesQuery.data?.nodes ?? detail?.nodes ?? [];
  const queryError = detailQuery.error?.message || nodesQuery.error?.message || null;

  const infoRow = (label: string, value?: string) => (
    <div>
      <div className="text-gray-400 mb-1">{label}</div>
      <div className="font-semibold text-gray-800 dark:text-gray-200 font-mono break-all">{value || '-'}</div>
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataSources.detailTitle')}</h2>
          <p className="text-xs text-gray-500 font-mono">{datasourceId}</p>
        </div>
        <Button variant="ghost" onClick={() => navigate({ to: '/data-sources' })}>← {t('dataSources.back')}</Button>
      </div>

      {queryError && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: queryError })}
        </div>
      )}

      {/* Basic Info */}
      <Card>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 text-xs">
          {infoRow(t('dataSources.nameLabel'), detail?.name)}
          {infoRow(t('dataSources.id'), detail?.datasourceId || datasourceId)}
          {infoRow(t('dataSources.type'), detail?.type || type)}
          <div>
            <div className="text-gray-400 mb-1">{t('dataSources.status')}</div>
            <Badge status={detail?.status === 'Available' || detail?.status === 'Ready' ? 'success' : 'default'}>
              {detail?.status || '-'}
            </Badge>
          </div>
        </div>
      </Card>

      {/* Connection Info */}
      <Card>
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-3">{t('dataSources.info')}</div>
        <pre className="text-[11px] font-mono bg-gray-50 dark:bg-gray-800 rounded-lg p-4 overflow-x-auto text-gray-700 dark:text-gray-300">
          {detail?.info ? JSON.stringify(detail.info, null, 2) : '-'}
        </pre>
      </Card>

      {/* Related Nodes */}
      <Card bodyClassName="p-0">
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 text-sm font-semibold text-gray-700 dark:text-gray-200">
          {t('dataSources.relatedNodes')}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('dataSources.nodeName')}</th>
                <th className="p-4">{t('dataSources.id')}</th>
                <th className="p-4">{t('dataSources.nodeStatus')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {relatedNodes.length === 0 && (
                <tr>
                  <td colSpan={3} className="p-4 text-center text-gray-400">{t('dataSources.noNodes')}</td>
                </tr>
              )}
              {relatedNodes.map((node) => (
                <tr key={node.nodeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{node.nodeName || '-'}</td>
                  <td className="p-4 font-mono text-gray-500">{node.nodeId || '-'}</td>
                  <td className="p-4">
                    <Badge status={node.status === 'Ready' || node.status === 'Available' ? 'success' : 'default'}>
                      {node.status || '-'}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
};
