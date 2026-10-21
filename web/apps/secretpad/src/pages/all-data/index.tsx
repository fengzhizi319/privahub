/**
 * CENTER 聚合视图（旧前端 `all-data-sources` / `all-data-tables`）。
 *
 * 遍历所有节点分别调用 `datasource/list` / `datatable/list`，按 “id + 节点” 去重聚合，
 * 支持名称搜索、按节点筛选，并可跳转到对应节点视图（/node/$nodeId）。
 */
import React, { useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Badge, Button, Card, Empty, Input, Pagination, Select } from '@secretpad/design-system';
import { apiClient, flattenDatatableNode, listDatasourcesJava, listDatatablesJava } from '@secretpad/api-client';
import type { DatatableAuthProjectJava, Node } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { aggregateDatasources, aggregateDatatables, filterAggregated, uniqueNodes } from './aggregate';
import type { AggregatedDatasource, AggregatedDatatable } from './aggregate';

const PAGE_SIZE = 10;

function useGoNode() {
  const navigate = useNavigate();
  return (nodeId?: string) => {
    if (!nodeId) return;
    // `/node/$nodeId` is registered by the node-layout stream.
    (navigate as unknown as (o: { to: string; params: Record<string, string> }) => void)({ to: '/node/$nodeId', params: { nodeId } });
  };
}

function useAllNodes() {
  const q = useQuery({ queryKey: ['nodes'], queryFn: () => apiClient.getNodes() });
  const nodes = useMemo(() => uniqueNodes(q.data ?? []), [q.data]);
  return { ...q, nodes };
}

const Toolbar: React.FC<{
  title: string;
  search: string;
  onSearch: (v: string) => void;
  nodeFilter: string;
  onNodeFilter: (v: string) => void;
  nodes: Node[];
  summary?: string;
  failed: string[];
  onReload: () => void;
  loading: boolean;
}> = ({ title, search, onSearch, nodeFilter, onNodeFilter, nodes, summary, failed, onReload, loading }) => {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{title}</h2>
          <p className="text-xs text-gray-500">{t('allData.subtitle')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Input aria-label={t('common.search')} placeholder={t('allData.searchPlaceholder')} value={search} onChange={(e) => onSearch(e.target.value)} className="!w-56" />
          <Select
            aria-label={t('dataTables.nodeBelongs')}
            value={nodeFilter}
            options={[{ value: '', label: t('allData.allNodes') }, ...nodes.map((n) => ({ value: n.nodeId, label: n.nodeName || n.nodeId }))]}
            onChange={onNodeFilter}
            className="!w-40"
          />
          <Button variant="outline" onClick={onReload} loading={loading}>
            {t('common.refresh')}
          </Button>
        </div>
      </div>
      {failed.length > 0 && (
        <div className="text-xs text-amber-700 bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 rounded-lg px-4 py-2">
          {t('allData.failedNodes', { nodes: failed.join('、') })}
        </div>
      )}
      {summary && <div className="text-xs text-gray-500">{t('allData.distribution', { summary })}</div>}
    </>
  );
};

function nodeSummary(items: { nodeId?: string; nodeName?: string }[]): string {
  const m = new Map<string, number>();
  items.forEach((i) => {
    const k = i.nodeName || i.nodeId || '-';
    m.set(k, (m.get(k) || 0) + 1);
  });
  return Array.from(m.entries())
    .map(([k, v]) => `${k}: ${v}`)
    .join('，');
}

const NotAllowed: React.FC = () => {
  const { t } = useTranslation();
  return <Empty className="py-16">{t('allData.centerOnly')}</Empty>;
};

export const AllDataSourcesPage: React.FC = () => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const goNode = useGoNode();
  const nodesQ = useAllNodes();
  const [search, setSearch] = useState('');
  const [nodeFilter, setNodeFilter] = useState('');
  const [page, setPage] = useState(1);

  const dataQ = useQuery({
    queryKey: ['all-datasources', nodesQ.nodes.map((n) => n.nodeId)],
    enabled: platform.isCenterAdmin && nodesQ.isSuccess,
    queryFn: async () => {
      const results = await Promise.allSettled(nodesQ.nodes.map((n) => listDatasourcesJava({ ownerId: n.nodeId, page: 1, size: 1000 })));
      return aggregateDatasources(nodesQ.nodes, results);
    },
  });

  const list = useMemo(
    () => filterAggregated<AggregatedDatasource>(dataQ.data?.items ?? [], search, nodeFilter, (i) => i.name),
    [dataQ.data, search, nodeFilter],
  );
  const pageItems = list.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  if (!platform.isCenterAdmin) return <NotAllowed />;

  return (
    <div className="space-y-4">
      <Toolbar
        title={t('allData.sourcesTitle')}
        search={search}
        onSearch={(v) => {
          setSearch(v);
          setPage(1);
        }}
        nodeFilter={nodeFilter}
        onNodeFilter={(v) => {
          setNodeFilter(v);
          setPage(1);
        }}
        nodes={nodesQ.nodes}
        summary={nodeSummary(list)}
        failed={dataQ.data?.failed ?? []}
        onReload={() => dataQ.refetch()}
        loading={dataQ.isFetching}
      />
      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-3">{t('dataSources.nameLabel')}</th>
                <th className="p-3">{t('dataSources.type')}</th>
                <th className="p-3">{t('dataTables.nodeBelongs')}</th>
                <th className="p-3">{t('dataSources.status')}</th>
                <th className="p-3">{t('allData.boundTables')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {(dataQ.isLoading || nodesQ.isLoading) && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">
                    {t('common.loading')}
                  </td>
                </tr>
              )}
              {!dataQ.isLoading && pageItems.length === 0 && (
                <tr>
                  <td colSpan={5} className="p-6 text-center text-gray-400">
                    {t('common.empty')}
                  </td>
                </tr>
              )}
              {pageItems.map((ds) => (
                <tr key={`${ds.datasourceId}-${ds.nodeId}`}>
                  <td className="p-3 font-semibold" title={ds.name}>
                    {ds.name}
                  </td>
                  <td className="p-3 font-mono">{ds.type}</td>
                  <td className="p-3">
                    <Button size="sm" variant="link" onClick={() => goNode(ds.nodeId)}>
                      {ds.nodeName || ds.nodeId}
                    </Button>
                  </td>
                  <td className="p-3">
                    <Badge status={ds.status === 'Available' ? 'success' : 'default'}>{ds.status || 'Unknown'}</Badge>
                  </td>
                  <td className="p-3" title={(ds.relatedDatas || []).join(', ')}>
                    {ds.relatedDatas?.length ? t('allData.tableCount', { n: ds.relatedDatas.length }) : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="p-3 border-t border-gray-100 dark:border-gray-800">
          <Pagination page={page} pageSize={PAGE_SIZE} total={list.length} onChange={setPage} />
        </div>
      </Card>
    </div>
  );
};

export const AllDataTablesPage: React.FC = () => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const goNode = useGoNode();
  const nodesQ = useAllNodes();
  const [search, setSearch] = useState('');
  const [nodeFilter, setNodeFilter] = useState('');
  const [page, setPage] = useState(1);

  const dataQ = useQuery({
    queryKey: ['all-datatables', nodesQ.nodes.map((n) => n.nodeId)],
    enabled: platform.isCenterAdmin && nodesQ.isSuccess,
    queryFn: async () => {
      const results = await Promise.allSettled(
        nodesQ.nodes.map((n) =>
          listDatatablesJava({ ownerId: n.nodeId, pageNumber: 1, pageSize: 1000 }).then((r) => r.datatableNodeVOList.map(flattenDatatableNode)),
        ),
      );
      return aggregateDatatables(nodesQ.nodes, results);
    },
  });

  const list = useMemo(
    () => filterAggregated<AggregatedDatatable>(dataQ.data?.items ?? [], search, nodeFilter, (i) => i.datatableName),
    [dataQ.data, search, nodeFilter],
  );
  const pageItems = list.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  if (!platform.isCenterAdmin) return <NotAllowed />;

  const authText = (a?: DatatableAuthProjectJava[]) => (a?.length ? t('allData.projectCount', { n: a.length }) : '-');

  return (
    <div className="space-y-4">
      <Toolbar
        title={t('allData.tablesTitle')}
        search={search}
        onSearch={(v) => {
          setSearch(v);
          setPage(1);
        }}
        nodeFilter={nodeFilter}
        onNodeFilter={(v) => {
          setNodeFilter(v);
          setPage(1);
        }}
        nodes={nodesQ.nodes}
        summary={nodeSummary(list)}
        failed={dataQ.data?.failed ?? []}
        onReload={() => dataQ.refetch()}
        loading={dataQ.isFetching}
      />
      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-3">{t('dtList.name')}</th>
                <th className="p-3">{t('dataTables.nodeBelongs')}</th>
                <th className="p-3">{t('dtDetail.datasource')}</th>
                <th className="p-3">{t('dataTables.status')}</th>
                <th className="p-3">{t('allData.teePush')}</th>
                <th className="p-3">{t('dataTables.authProjects')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {(dataQ.isLoading || nodesQ.isLoading) && (
                <tr>
                  <td colSpan={6} className="p-4 text-center text-gray-400">
                    {t('common.loading')}
                  </td>
                </tr>
              )}
              {!dataQ.isLoading && pageItems.length === 0 && (
                <tr>
                  <td colSpan={6} className="p-6 text-center text-gray-400">
                    {t('common.empty')}
                  </td>
                </tr>
              )}
              {pageItems.map((dt) => (
                <tr key={`${dt.datatableId}-${dt.nodeId}-${dt.datasourceType || 'LOCAL'}`}>
                  <td className="p-3 font-semibold" title={dt.datatableName}>
                    {dt.datatableName}
                  </td>
                  <td className="p-3">
                    <Button size="sm" variant="link" onClick={() => goNode(dt.nodeId)}>
                      {dt.nodeName || dt.nodeId}
                    </Button>
                  </td>
                  <td className="p-3">
                    {dt.datasourceName || '-'}
                    {dt.datasourceType && <span className="ml-1 px-1.5 py-0.5 rounded bg-gray-100 dark:bg-gray-800 font-mono text-[10px]">{dt.datasourceType}</span>}
                  </td>
                  <td className="p-3">
                    <Badge status={dt.status === 'Available' ? 'success' : 'default'}>{dt.status || 'Unknown'}</Badge>
                  </td>
                  <td className="p-3">
                    {dt.pushToTeeStatus ? (
                      <Badge status={dt.pushToTeeStatus === 'SUCCESS' ? 'success' : dt.pushToTeeStatus === 'FAILED' ? 'error' : 'processing'}>
                        {dt.pushToTeeStatus}
                      </Badge>
                    ) : (
                      '-'
                    )}
                  </td>
                  <td className="p-3" title={(dt.authProjects || []).map((p) => p.name || p.projectId).join(', ')}>
                    {authText(dt.authProjects)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="p-3 border-t border-gray-100 dark:border-gray-800">
          <Pagination page={page} pageSize={PAGE_SIZE} total={list.length} onChange={setPage} />
        </div>
      </Card>
    </div>
  );
};
