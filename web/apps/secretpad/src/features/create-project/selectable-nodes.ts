import type { NodeVOJava } from '@secretpad/api-client';

/** Ready (or status-less) non-TEE nodes are selectable participants. */
export function selectableNodes(nodes: NodeVOJava[]): NodeVOJava[] {
  return nodes.filter((n) => {
    if (!n.nodeId || n.nodeId === 'tee') return false;
    const status = (n.nodeStatus || 'Ready').toLowerCase();
    return status === 'ready' || status === 'succeeded';
  });
}
