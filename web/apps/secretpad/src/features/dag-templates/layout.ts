import type { TemplateBuildResult } from './types';

/** Lay out one newly created template only. Never append another template's
 * nodes, reuse its mutable objects, or apply coordinates to a different graph.
 * Topological layers leave room for cards, ports and branching connections.
 */
export function layoutTemplate(result: TemplateBuildResult): TemplateBuildResult {
  const graph = structuredClone(result);
  const ids = graph.nodes.map(n => n.graphNodeId!);
  const remaining = new Set(ids);
  const layers: string[][] = [];
  while (remaining.size) {
    const ready = ids.filter(id => remaining.has(id) && !graph.edges.some(e => e.target === id && remaining.has(e.source!)));
    // Defensive fallback for a malformed cycle: still place every card apart.
    const layer = ready.length ? ready : [...remaining];
    layers.push(layer);
    layer.forEach(id => remaining.delete(id));
  }
  const maxColumns = Math.max(1, ...layers.map(layer => layer.length));
  const positions = new Map<string, { x: number; y: number }>();
  layers.forEach((layer, row) => layer.forEach((id, column) => {
    positions.set(id, { x: 80 + ((maxColumns - layer.length) / 2 + column) * 260, y: 60 + row * 200 });
  }));
  return { ...graph, nodes: graph.nodes.map(node => ({ ...node, ...positions.get(node.graphNodeId!) })) };
}
