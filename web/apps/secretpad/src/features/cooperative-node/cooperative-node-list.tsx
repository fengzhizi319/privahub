import React, { useEffect, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, Input, Pagination, Select, toast } from '@secretpad/design-system';
import {
  apiClient,
  getNodeJava,
  listMyInstNodesJava,
  pageCooperativeRoutesJava,
  refreshNodeRouteJava,
  type NodeRouterJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { AddCooperativeNodeDrawer } from './add-cooperative-node-drawer';
import { CooperativeNodeDetailDrawer, DeleteCooperativeNodeDialog, EditCooperativeNodeModal } from './cooperative-node-detail';
import { errorText, formatTime } from './format';
import { isEmbeddedRoute } from './route-helpers';
import { NodeStatusBadge } from './ui-common';
import { useDebounced } from '@/shared/lib/use-debounced';

type SortField = 'gmtCreate' | 'gmtModified';

export interface CooperativeNodeListProps {
  /** Own node whose routes are listed (node context / EDGE). */
  ownerNodeId?: string;
  /** Hide the page title (embedded in another page). */
  compact?: boolean;
}

/** Legacy `modules/cooperative-node-list/index.tsx`, shared by /node-routes, node context and P2P my-node. */
export const CooperativeNodeList: React.FC<CooperativeNodeListProps> = ({ ownerNodeId, compact }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const platform = usePlatform();
  const p2p = platform.isP2p;

  const [pickedNode, setPickedNode] = useState('');
  const [search, setSearch] = useState('');
  const debounced = useDebounced(search);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  const [sort, setSort] = useState<{ field: SortField; order: 'ASC' | 'DESC' } | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [editing, setEditing] = useState<NodeRouterJava | null>(null);
  const [deleting, setDeleting] = useState<NodeRouterJava | null>(null);

  // CENTER admin on the standalone page chooses the node to inspect.
  const needsPicker = !ownerNodeId && !p2p && platform.isCenterAdmin;
  const pickerQuery = useQuery({ queryKey: ['coop-picker-nodes'], queryFn: () => apiClient.getNodes(), enabled: needsPicker });
  useEffect(() => {
    if (needsPicker && !pickedNode && pickerQuery.data?.length) setPickedNode(pickerQuery.data[0].nodeId);
  }, [needsPicker, pickedNode, pickerQuery.data]);

  const ownerId = ownerNodeId || (needsPicker ? pickedNode : platform.ownerId);
  const selfNodeId = p2p ? undefined : ownerId;
  const canWrite = platform.canWriteNode(selfNodeId);

  useEffect(() => setPage(1), [debounced, ownerId, sort]);

  const listQuery = useQuery({
    queryKey: ['coop-routes', ownerId, page, size, debounced, sort],
    queryFn: () =>
      pageCooperativeRoutesJava({
        page,
        size,
        search: debounced,
        sort: sort ? { [sort.field]: sort.order } : {},
        ownerId,
      }),
    enabled: !!ownerId,
    placeholderData: keepPreviousData,
  });
  const routes = listQuery.data?.list ?? [];
  const total = listQuery.data?.total ?? 0;
  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['coop-routes'] });

  // Add button availability (legacy: own node must be Ready / any inst node Ready).
  const selfNodeQuery = useQuery({
    queryKey: ['coop-self-status', selfNodeId],
    queryFn: () => getNodeJava(selfNodeId!),
    enabled: !p2p && !!selfNodeId,
  });
  const instNodesQuery = useQuery({ queryKey: ['coop-inst-nodes'], queryFn: listMyInstNodesJava, enabled: p2p });
  const addDisabled = p2p
    ? !(instNodesQuery.data || []).some((n) => n.nodeStatus === 'Ready')
    : selfNodeQuery.data?.nodeStatus !== 'Ready';

  const refresh = useMutation({
    mutationFn: (routeId: string) => refreshNodeRouteJava(routeId),
    onSuccess: () => {
      toast.success(t('coop.refreshSuccess'));
      void invalidate();
    },
    onError: () => toast.error(t('coop.refreshFailed')),
  });

  const toggleSort = (field: SortField) =>
    setSort((s) => (s?.field !== field ? { field, order: 'DESC' } : s.order === 'DESC' ? { field, order: 'ASC' } : null));
  const sortMark = (field: SortField) => (sort?.field === field ? (sort.order === 'ASC' ? ' ↑' : ' ↓') : ' ↕');

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div className="flex flex-wrap items-center gap-2 sm:gap-3 min-w-0 w-full sm:w-auto">
          {!compact && (
            <div className="mr-2">
              <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('coop.title')}</h2>
              <p className="text-xs text-gray-500">{t('coop.subtitle')}</p>
            </div>
          )}
          {needsPicker && (
            <Select
              className="!w-full sm:!w-48"
              value={pickedNode}
              onChange={setPickedNode}
              options={(pickerQuery.data || []).map((n) => ({ value: n.nodeId, label: n.nodeName || n.nodeId }))}
            />
          )}
          <Input
            className="!w-full sm:!w-60"
            value={search}
            placeholder={t('coop.searchPlaceholder')}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        {canWrite && (
          <span className="w-full sm:w-auto" title={addDisabled ? t('coop.nodeUnavailable') : undefined}>
            <Button className="w-full sm:w-auto" variant="primary" disabled={addDisabled || !ownerId} onClick={() => setAddOpen(true)}>
              {t('coop.add')}
            </Button>
          </span>
        )}
      </div>

      {listQuery.error && <div className="text-xs text-rose-500">{errorText(listQuery.error)}</div>}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-3">{t('coop.cooperativeNode')}</th>
                <th className="p-3">{t('coop.coopAddress')}</th>
                <th className="p-3">{t('coop.selfNode')}</th>
                <th className="p-3" title={t('coop.selfAddressTip')}>
                  {t('coop.selfAddress')}
                </th>
                <th className="p-3">{t('coop.initiator')}</th>
                <th className="p-3">{t('coop.commStatus')}</th>
                <th className="p-3 cursor-pointer select-none" onClick={() => toggleSort('gmtCreate')}>
                  {t('coop.cooperateTime')}
                  {sortMark('gmtCreate')}
                </th>
                <th className="p-3 cursor-pointer select-none" onClick={() => toggleSort('gmtModified')}>
                  {t('coop.editTime')}
                  {sortMark('gmtModified')}
                </th>
                <th className="p-3">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {listQuery.isLoading && (
                <tr>
                  <td colSpan={9} className="p-4 text-center text-gray-400">
                    {t('common.loading')}
                  </td>
                </tr>
              )}
              {!listQuery.isLoading && routes.length === 0 && (
                <tr>
                  <td colSpan={9} className="p-4 text-center text-gray-400">
                    {t('nodeRoutes.noRoutes')}
                  </td>
                </tr>
              )}
              {routes.map((r) => {
                const embedded = isEmbeddedRoute(r);
                const deleteBlocked = p2p && !!r.isProjectJobRunning;
                return (
                  <tr key={r.routeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-800/50">
                    <td className="p-3">
                      <button
                        type="button"
                        className="font-semibold text-blue-600 dark:text-blue-400 hover:underline text-left"
                        onClick={() => setDetailId(r.routeId || null)}
                      >
                        {embedded && (
                          <span className="mr-1 px-1.5 py-0.5 rounded text-[10px] bg-green-100 dark:bg-green-950 text-green-600">
                            {t('nodes.embedded')}
                          </span>
                        )}
                        {r.srcNode?.nodeName || r.srcNodeId || '-'}
                      </button>
                      <div className="font-mono text-[10px] text-gray-400">{r.srcNodeId}</div>
                      {p2p && <div className="text-[10px] text-gray-400">{r.srcNode?.instName || '- -'}</div>}
                    </td>
                    <td className="p-3 font-mono text-gray-500">{r.srcNetAddress || '- -'}</td>
                    <td className="p-3">{r.dstNode?.nodeName || r.dstNodeId || '- -'}</td>
                    <td className="p-3 font-mono text-gray-500">{r.dstNetAddress || '- -'}</td>
                    <td className="p-3">{r.srcNode?.nodeName || '- -'}</td>
                    <td className="p-3">
                      <span className="inline-flex items-center gap-1">
                        <NodeStatusBadge status={r.status} />
                        <Button
                          size="sm"
                          variant="link"
                          onClick={() => r.routeId && refresh.mutate(r.routeId)}
                          loading={refresh.isPending && refresh.variables === r.routeId}
                        >
                          {t('common.refresh')}
                        </Button>
                      </span>
                    </td>
                    <td className="p-3 text-gray-500">{formatTime(r.gmtCreate)}</td>
                    <td className="p-3 text-gray-500">{formatTime(r.gmtModified)}</td>
                    <td className="p-3">
                      {embedded || !canWrite ? (
                        '- -'
                      ) : (
                        <div className="flex items-center gap-2">
                          <Button size="sm" variant="ghost" onClick={() => setEditing(r)}>
                            {t('common.edit')}
                          </Button>
                          <span title={deleteBlocked ? t('coop.deleteBlocked') : undefined}>
                            <Button size="sm" variant="ghost" disabled={deleteBlocked} onClick={() => setDeleting(r)}>
                              {t('common.delete')}
                            </Button>
                          </span>
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-end gap-3 p-3">
          <Select
            className="!w-24"
            value={String(size)}
            onChange={(v) => {
              setSize(Number(v));
              setPage(1);
            }}
            options={[10, 20, 50].map((n) => ({ value: String(n), label: `${n}/page` }))}
          />
          <Pagination page={page} pageSize={size} total={total} onChange={setPage} />
        </div>
      </Card>

      <AddCooperativeNodeDrawer
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onOk={() => void invalidate()}
        p2p={p2p}
        ownerNodeId={selfNodeId}
      />
      <CooperativeNodeDetailDrawer
        routeId={detailId}
        p2p={p2p}
        canWrite={canWrite}
        onClose={() => setDetailId(null)}
        onChanged={() => void invalidate()}
      />
      <EditCooperativeNodeModal route={editing} p2p={p2p} onClose={() => setEditing(null)} onOk={() => void invalidate()} />
      <DeleteCooperativeNodeDialog route={deleting} p2p={p2p} onClose={() => setDeleting(null)} onOk={() => void invalidate()} />
    </div>
  );
};
