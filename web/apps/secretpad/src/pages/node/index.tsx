import React from 'react';
import { Link, Outlet, useParams, useRouterState } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '@secretpad/api-client';
import { Badge } from '@secretpad/design-system';
import { useTranslation } from '@/shared/lib/i18n';
import { NodeContextProvider } from '@/shared/lib/node-context';
import { useCanAccessEmbeddedNode, usePlatform } from '@/shared/lib/platform';
import { visibleNodeTabs, type NodeTab } from './tabs';

/**
 * Node-context layout (`/node/$nodeId/*`), the equivalent of the legacy
 * `/node?ownerId=` view: data sources, data tables, cooperative nodes and
 * results, all scoped to one node through {@link NodeContextProvider}.
 */
export const NodeLayoutPage: React.FC = () => {
  const { t } = useTranslation();
  const { nodeId } = useParams({ strict: false }) as { nodeId: string };
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { ownerId, isCenterAdmin } = usePlatform();
  // my-node shows the own node, or a built-in node for CENTER admins (legacy edge-auth).
  const embeddedAccessible = useCanAccessEmbeddedNode(nodeId);
  const canOpenMyNode = nodeId === ownerId || embeddedAccessible;

  const nodeQuery = useQuery({
    queryKey: ['node', nodeId],
    queryFn: () => apiClient.getNode(nodeId),
    enabled: !!nodeId,
    retry: false,
  });
  const node = nodeQuery.data as (Record<string, unknown> & { nodeName?: string; type?: string; nodeStatus?: string }) | undefined;
  const nodeType = typeof node?.type === 'string' ? node.type : undefined;
  const tabs = visibleNodeTabs(nodeId, nodeType);
  const status = String(node?.nodeStatus || '');

  const labels: Record<NodeTab, string> = {
    'data-sources': t('nodeView.tabs.dataSources'),
    'data-tables': t('nodeView.tabs.dataTables'),
    'cooperative-nodes': t('nodeView.tabs.cooperativeNodes'),
    results: t('nodeView.tabs.results'),
  };

  return (
    <NodeContextProvider value={{ nodeId, nodeName: node?.nodeName, nodeType }}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-lg bg-blue-600/10 text-blue-600 flex items-center justify-center">🖥️</div>
            <div>
              <div className="text-base font-semibold text-gray-900 dark:text-gray-100">
                {node?.nodeName || nodeId}
                {nodeId === ownerId && (
                  <span className="ml-2 text-[11px] text-blue-600 font-normal">{t('nodeView.mine')}</span>
                )}
              </div>
              <div className="text-xs text-gray-500 font-mono">{nodeId}</div>
            </div>
            {status && (
              <Badge status={/ready|succeed|online/i.test(status) ? 'success' : 'warning'}>{status}</Badge>
            )}
          </div>
          <div className="flex items-center gap-3 text-xs">
            {canOpenMyNode && (
              <Link to="/p2p/my-node" search={nodeId === ownerId ? {} : { ownerId: nodeId }} className="text-blue-600 hover:underline">
                {t('nodeView.nodeInfo')}
              </Link>
            )}
            {isCenterAdmin && (
              <Link to="/nodes" className="text-gray-500 hover:underline">
                ← {t('nodeView.backToNodes')}
              </Link>
            )}
          </div>
        </div>

        <nav role="tablist" className="flex gap-1 border-b border-gray-200 dark:border-gray-800">
          {tabs.map((tab) => {
            const active = pathname.endsWith(`/${tab}`);
            return (
              <Link
                key={tab}
                role="tab"
                aria-selected={active}
                to={`/node/$nodeId/${tab}` as '/node/$nodeId/data-sources'}
                params={{ nodeId }}
                className={`px-3 py-2 text-sm -mb-px border-b-2 transition-colors ${
                  active
                    ? 'border-blue-600 text-blue-600 dark:text-blue-400 font-medium'
                    : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
                }`}
              >
                {labels[tab]}
              </Link>
            );
          })}
        </nav>

        <Outlet />
      </div>
    </NodeContextProvider>
  );
};
