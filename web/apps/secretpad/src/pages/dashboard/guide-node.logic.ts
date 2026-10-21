/**
 * Pure helpers for the dashboard guide-node card (legacy `modules/guide-node`).
 */
import type { NodeRouteVOJava, NodeVOJava } from '@secretpad/api-client';

/** Distinct destination nodes the given node has a route to (legacy `getAuthenticatedNodes`). */
export function authorizedNodeIds(routes: NodeRouteVOJava[] | undefined, nodeId: string): string[] {
  const set = new Set<string>();
  (routes || []).forEach((r) => {
    if (r.srcNodeId === nodeId && r.dstNodeId) set.add(r.dstNodeId);
  });
  return [...set];
}

/** Embedded (built-in) nodes except `tee`; falls back to all non-tee nodes when none are embedded. */
export function guideNodes(nodes: NodeVOJava[]): NodeVOJava[] {
  const nonTee = nodes.filter((n) => n.nodeId && n.nodeId !== 'tee');
  const embedded = nonTee.filter((n) => n.type === 'embedded');
  return (embedded.length > 0 ? embedded : nonTee).slice(0, 4);
}
