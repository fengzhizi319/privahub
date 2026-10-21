import type { PreviewEdge } from './dag-preview';

/** 某节点的所有上游（含自身）。 */
export function lineageIds(nodeId: string, edges: PreviewEdge[]): Set<string> {
  const out = new Set<string>([nodeId]);
  const stack = [nodeId];
  while (stack.length) {
    const cur = stack.pop()!;
    edges.filter((e) => e.target === cur).forEach((e) => {
      if (!out.has(e.source)) {
        out.add(e.source);
        stack.push(e.source);
      }
    });
  }
  return out;
}

/** 找到产出该结果的节点：outputs 中某个锚点是 domainDataId 的后缀。 */
export function producingNodeId(nodes: Array<{ graphNodeId: string; outputs?: string[] }>, domainDataId?: string): string | undefined {
  if (!domainDataId) return undefined;
  return nodes.find((n) => (n.outputs || []).some((o) => o && (domainDataId === o || domainDataId.endsWith(`-${o}`) || domainDataId.endsWith(o))))?.graphNodeId;
}
