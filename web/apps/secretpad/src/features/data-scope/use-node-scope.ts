/**
 * Node scope for the data pages (datasources / datatables).
 *
 * - Inside the node layout (`/node/$nodeId/*`) the node comes from
 *   `useNodeContext()` and the node picker is hidden.
 * - Otherwise a node picker is shown, defaulting to the account's own node
 *   (`ownerId`) when present, else the first node.
 */
import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '@secretpad/api-client';
import type { Node } from '@secretpad/api-client';
import { useNodeContext } from '@/shared/lib/use-node-context';
import { useInstNodeIds, usePlatform } from '@/shared/lib/platform';

export function useNodeScope() {
  const ctx = useNodeContext();
  const platform = usePlatform();
  const [picked, setPicked] = useState('');
  const instNodeIds = useInstNodeIds();

  const nodesQuery = useQuery({ queryKey: ['nodes'], queryFn: () => apiClient.getNodes() });
  const nodes: Node[] = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);

  useEffect(() => {
    if (ctx.nodeId || picked || nodes.length === 0) return;
    // AUTONOMY: ownerId is the institution id, so default to the first node of the institution.
    const own =
      nodes.find((n) => n.nodeId === platform.ownerId) ||
      nodes.find((n) => (n.instId && n.instId === platform.ownerId) || instNodeIds.includes(n.nodeId));
    setPicked(own ? own.nodeId : nodes[0].nodeId);
  }, [ctx.nodeId, picked, nodes, platform.ownerId, instNodeIds]);

  const ownerId = ctx.nodeId || picked;
  const node = nodes.find((n) => n.nodeId === ownerId);
  const nodeType = (ctx.nodeType || node?.type || '').toLowerCase();

  return {
    ownerId,
    setOwnerId: setPicked,
    nodes,
    nodesQuery,
    showPicker: !ctx.nodeId,
    nodeOptions: nodes.map((n) => ({ value: n.nodeId, label: n.nodeName || n.nodeId })),
    /** TEE nodes have no datasources (legacy hid them). */
    isTeeNode: nodeType === 'tee' || ownerId === 'tee',
    platform,
    canWrite: platform.canWriteNode(ownerId, { instId: node?.instId, instNodeIds }),
  };
}
