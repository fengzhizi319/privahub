export const NODE_TABS = ['data-sources', 'data-tables', 'cooperative-nodes', 'results'] as const;
export type NodeTab = (typeof NODE_TABS)[number];

/** TEE nodes have no datasource management (legacy node view hides the tab). */
export function isTeeNode(nodeId?: string, nodeType?: string): boolean {
  return nodeId === 'tee' || (nodeType || '').toLowerCase().includes('tee');
}

export function visibleNodeTabs(nodeId?: string, nodeType?: string): NodeTab[] {
  return NODE_TABS.filter((tab) => !(tab === 'data-sources' && isTeeNode(nodeId, nodeType)));
}
