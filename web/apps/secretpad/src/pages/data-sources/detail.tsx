/**
 * 数据源详情（旧前端 `DataSourceInfoDrawer`）：按类型展示连接信息，密钥类字段脱敏。
 */
import React from 'react';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Badge, Button, Card } from '@secretpad/design-system';
import { getDatasourceDetailJava, getDatasourceNodesJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { SECRET_INFO_KEYS } from '@/features/datasource-form';

interface DetailSearch {
  ownerId: string;
  datasourceId: string;
  type: string;
}

/** Info rows shown per type (legacy order); unknown types show every key. */
const INFO_FIELDS: Record<string, { key: string; label: string }[]> = {
  OSS: [
    { key: 'endpoint', label: 'endpoint' },
    { key: 'ak', label: 'AccessKeyID' },
    { key: 'sk', label: 'AccessKeySecret' },
    { key: 'virtualhost', label: 'virtualhost' },
    { key: 'bucket', label: 'bucket' },
    { key: 'prefix', label: 'prefix' },
  ],
  ODPS: [
    { key: 'project', label: 'ODPS Project' },
    { key: 'endpoint', label: 'endpoint' },
    { key: 'accessId', label: 'AccessKeyID' },
    { key: 'accessKey', label: 'AccessKeySecret' },
  ],
  MYSQL: [
    { key: 'endpoint', label: 'endpoint' },
    { key: 'user', label: 'user' },
    { key: 'password', label: 'password' },
    { key: 'database', label: 'database' },
  ],
  LOCAL: [{ key: 'path', label: 'path' }],
};

function show(key: string, value: unknown): string {
  if (SECRET_INFO_KEYS.includes(key)) return '******';
  if (value === undefined || value === null || value === '') return '--';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

export const DataSourceDetailPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { ownerId, datasourceId, type } = useSearch({ strict: false }) as DetailSearch;

  const detailQuery = useQuery({
    queryKey: ['datasource-detail-java', ownerId, datasourceId, type],
    queryFn: () => getDatasourceDetailJava({ ownerId, datasourceId, type }),
    enabled: !!ownerId && !!datasourceId,
  });
  const nodesQuery = useQuery({
    queryKey: ['datasource-nodes-java', ownerId, datasourceId],
    queryFn: () => getDatasourceNodesJava({ ownerId, datasourceId }),
    enabled: !!ownerId && !!datasourceId,
  });

  const detail = detailQuery.data;
  const dsType = detail?.type || type;
  const info = (detail?.info || {}) as Record<string, unknown>;
  const fields = INFO_FIELDS[dsType] || Object.keys(info).map((k) => ({ key: k, label: k }));
  const relatedNodes = nodesQuery.data?.length ? nodesQuery.data : detail?.nodes ?? [];
  const queryError = detailQuery.error?.message || nodesQuery.error?.message || null;

  const row = (label: string, value: React.ReactNode, key?: string) => (
    <div key={key ?? label}>
      <div className="text-gray-400 mb-1">{label}</div>
      <div className="font-semibold text-gray-800 dark:text-gray-200 font-mono break-all">{value}</div>
    </div>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dataSources.detailTitle')}</h2>
          <p className="text-xs text-gray-500 font-mono">{datasourceId}</p>
        </div>
        <Button variant="ghost" onClick={() => window.history.length > 1 ? window.history.back() : navigate({ to: '/data-sources' })}>
          ← {t('dataSources.back')}
        </Button>
      </div>

      {queryError && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: queryError })}
        </div>
      )}

      <Card>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 text-xs">
          {row(t('dataSources.nameLabel'), detail?.name || '--')}
          {row(t('dataSources.id'), detail?.datasourceId || datasourceId)}
          {row(t('dataSources.type'), dsType || '--')}
          {row(
            t('dataSources.status'),
            <Badge status={detail?.status === 'Available' ? 'success' : 'default'}>{detail?.status || '-'}</Badge>,
          )}
        </div>
      </Card>

      <Card>
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-3">{t('dataSources.info')}</div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
          {fields.length === 0 && <div className="text-gray-400">--</div>}
          {fields.map((f) => row(f.label, show(f.key, info[f.key]), f.key))}
        </div>
      </Card>

      <Card bodyClassName="p-0">
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 text-sm font-semibold text-gray-700 dark:text-gray-200">
          {t('dataSources.relatedNodes')}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('dataSources.nodeName')}</th>
                <th className="p-4">{t('dataSources.id')}</th>
                <th className="p-4">{t('dataSources.nodeStatus')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {relatedNodes.length === 0 && (
                <tr>
                  <td colSpan={3} className="p-4 text-center text-gray-400">
                    {t('dataSources.noNodes')}
                  </td>
                </tr>
              )}
              {relatedNodes.map((node) => (
                <tr key={node.nodeId}>
                  <td className="p-4 font-semibold text-blue-600 dark:text-blue-400">{node.nodeName || '-'}</td>
                  <td className="p-4 font-mono text-gray-500">{node.nodeId || '-'}</td>
                  <td className="p-4">
                    <Badge status={node.status === 'Available' ? 'success' : 'error'}>{node.status || '-'}</Badge>
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
