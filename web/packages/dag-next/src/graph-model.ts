/**
 * 画布图模型：端口（Port）、锚点命名、节点 ID、连线校验与图遍历工具。
 *
 * 与旧版 `@secretflow/dag` + `main-dag/graph-hook-service.ts` 的约定保持一致：
 * - 节点 ID：`${graphId}-node-${index}`，index 来自 `graph/node/max_index`；
 * - 输入锚点：`${nodeId}-input-${i}`，输出锚点：`${nodeId}-output-${i}`；
 * - 边 ID：`${sourceAnchor}__${targetAnchor}`；
 * - 端口由组件 IoDef 生成：输出类型为 `sf.report` / `sf.read_data` 的不出输出端口；
 * - 下游节点 `inputs[i]` 保存连接到第 i 个输入端口的上游输出锚点。
 */

export const DISTDATA_TYPE = {
  REPORT: 'sf.report',
  READ_DATA: 'sf.read_data',
} as const;

/** 组件输入 / 输出定义（兼容 Java camelCase 与 proto snake_case）。 */
export interface IoDefLike {
  name?: string;
  desc?: string;
  types?: string[];
  type?: string;
  attrs?: Array<Record<string, unknown>>;
}

export interface ComponentIoLike {
  inputs?: IoDefLike[];
  outputs?: IoDefLike[];
}

export interface PortDef {
  /** 锚点 ID，如 `g1-node-3-input-0`。 */
  id: string;
  group: 'input' | 'output';
  index: number;
  name?: string;
  types: string[];
}

export interface GraphNodeLike {
  id: string;
  codeName?: string;
  inputs?: string[];
  outputs?: string[];
}

export interface GraphEdgeLike {
  id: string;
  source: string;
  target: string;
  sourceAnchor?: string;
  targetAnchor?: string;
}

export function ioTypes(io: IoDefLike | undefined): string[] {
  if (!io) return [];
  if (Array.isArray(io.types) && io.types.length > 0) return io.types.map(String);
  if (io.type) return [String(io.type)];
  return [];
}

export const outputAnchor = (nodeId: string, index: number) => `${nodeId}-output-${index}`;
export const inputAnchor = (nodeId: string, index: number) => `${nodeId}-input-${index}`;
export const edgeIdOf = (sourceAnchor: string, targetAnchor: string) => `${sourceAnchor}__${targetAnchor}`;
export const nodeIdOf = (graphId: string, index: number) => `${graphId}-node-${index}`;

/** 解析锚点：返回节点 ID、方向与下标。 */
export function parseAnchor(anchor?: string): { nodeId: string; kind: 'input' | 'output'; index: number } | null {
  if (!anchor) return null;
  const m = /^(.*)-(input|output)-(\d+)$/.exec(anchor);
  if (!m) return null;
  return { nodeId: m[1], kind: m[2] as 'input' | 'output', index: Number(m[3]) };
}

/** 从节点 ID 中提取序号（`xxx-node-12` → 12），无法解析时返回 0。 */
export function nodeIndexOf(nodeId: string): number {
  const m = /-node-(\d+)$/.exec(nodeId);
  return m ? Number(m[1]) : 0;
}

/** 当前节点集合中的最大序号（作为 max_index 的 currentIndex 上报）。 */
export function maxNodeIndex(nodeIds: string[]): number {
  return nodeIds.reduce((max, id) => Math.max(max, nodeIndexOf(id)), 0);
}

/**
 * 按 IoDef 生成端口：所有输入都有端口；输出中 `sf.report` / `sf.read_data`
 * 不生成端口（报告只能查看、不能被下游消费）。
 */
export function buildPorts(nodeId: string, def: ComponentIoLike | null | undefined): PortDef[] {
  if (!def) return [];
  const ports: PortDef[] = [];
  (def.inputs || []).forEach((io, index) => {
    ports.push({ id: inputAnchor(nodeId, index), group: 'input', index, name: io?.name, types: ioTypes(io) });
  });
  (def.outputs || []).forEach((io, index) => {
    const types = ioTypes(io);
    if (types[0] === DISTDATA_TYPE.REPORT || types[0] === DISTDATA_TYPE.READ_DATA) return;
    ports.push({ id: outputAnchor(nodeId, index), group: 'output', index, name: io?.name, types });
  });
  return ports;
}

/** 节点保存到后端的 outputs 列表：每个输出一个锚点（含报告）。 */
export function buildOutputIds(nodeId: string, def: ComponentIoLike | null | undefined): string[] {
  return (def?.outputs || []).map((_, index) => outputAnchor(nodeId, index));
}

/** 可查看结果的输出（排除 read_data）。 */
export function buildResultOutputs(
  nodeId: string,
  def: ComponentIoLike | null | undefined,
): Array<{ id: string; name?: string; type: string }> {
  const out: Array<{ id: string; name?: string; type: string }> = [];
  (def?.outputs || []).forEach((io, index) => {
    const types = ioTypes(io);
    if (types[0] === DISTDATA_TYPE.READ_DATA) return;
    out.push({ id: outputAnchor(nodeId, index), name: io?.name, type: (types[0] || '').split('.')[1] || 'table' });
  });
  return out;
}

/** 根据连线重新推导节点 inputs（inputs[i] = 连到第 i 个输入端口的上游锚点）。 */
/** JSON 导入 / 跨图复制时需要改写 ID 的节点最小形态。 */
export interface RemappableNode {
  id: string;
  inputs?: string[];
  outputs?: string[];
  ports?: Array<{ id: string }>;
  resultOutputs?: Array<{ id: string }>;
}

/**
 * 导入的图改写为当前图的节点 ID（旧版复制 / 导入都按 `${graphId}-node-${max_index+i}`）：
 * `ids[i]` 对应 `nodes[i]` 的新 ID；锚点（`<nodeId>-output-0` 等）、端口、结果输出、inputs
 * 与连线的 source/target/anchor 一并改写，连线 ID 按新锚点重算。引用未知节点的连线丢弃。
 */
export function remapImportedGraph<N extends RemappableNode, E extends GraphEdgeLike>(
  nodes: N[],
  edges: E[],
  ids: string[],
): { nodes: N[]; edges: E[]; idMap: Map<string, string> } {
  const idMap = new Map<string, string>();
  nodes.forEach((n, i) => idMap.set(n.id, ids[i] ?? n.id));
  const anchor = (a?: string): string | undefined => {
    const parsed = parseAnchor(a);
    const next = parsed ? idMap.get(parsed.nodeId) : undefined;
    return parsed && next ? `${next}-${parsed.kind}-${parsed.index}` : a;
  };
  const outNodes = nodes.map((n) => ({
    ...n,
    id: idMap.get(n.id)!,
    inputs: n.inputs?.map((a) => anchor(a) ?? a),
    outputs: n.outputs?.map((a) => anchor(a) ?? a),
    ports: n.ports?.map((p) => ({ ...p, id: anchor(p.id) ?? p.id })),
    resultOutputs: n.resultOutputs?.map((o) => ({ ...o, id: anchor(o.id) ?? o.id })),
  }));
  const outEdges = edges
    .filter((e) => idMap.has(e.source) && idMap.has(e.target))
    .map((e) => {
      const source = idMap.get(e.source)!;
      const target = idMap.get(e.target)!;
      const sourceAnchor = anchor(e.sourceAnchor);
      const targetAnchor = anchor(e.targetAnchor);
      return { ...e, id: sourceAnchor && targetAnchor ? edgeIdOf(sourceAnchor, targetAnchor) : `${source}__${target}`, source, target, sourceAnchor, targetAnchor };
    });
  return { nodes: outNodes, edges: outEdges, idMap };
}

export function deriveInputs(nodeId: string, edges: GraphEdgeLike[]): string[] {
  const slots: string[] = [];
  let max = -1;
  edges.forEach((e) => {
    if (e.target !== nodeId) return;
    const parsed = parseAnchor(e.targetAnchor);
    if (!parsed || !e.sourceAnchor) return;
    slots[parsed.index] = e.sourceAnchor;
    max = Math.max(max, parsed.index);
  });
  const out: string[] = [];
  for (let i = 0; i <= max; i++) out.push(slots[i] ?? '');
  return out;
}

export type ConnectionError =
  | 'SELF'
  | 'NO_PORT'
  | 'DIRECTION'
  | 'TYPE'
  | 'DUPLICATE'
  | 'OCCUPIED'
  | 'CYCLE';

export interface ConnectionCandidate {
  source: string;
  target: string;
  sourceAnchor: string;
  targetAnchor: string;
}

export type ConnectionResult = { ok: true } | { ok: false; reason: ConnectionError };

/** 判断 from 是否能沿边到达 to（用于成环检测）。 */
export function isReachable(from: string, to: string, edges: GraphEdgeLike[]): boolean {
  if (from === to) return true;
  const adj = new Map<string, string[]>();
  edges.forEach((e) => {
    if (!adj.has(e.source)) adj.set(e.source, []);
    adj.get(e.source)!.push(e.target);
  });
  const seen = new Set<string>([from]);
  const stack = [from];
  while (stack.length) {
    const cur = stack.pop()!;
    for (const nxt of adj.get(cur) || []) {
      if (nxt === to) return true;
      if (!seen.has(nxt)) {
        seen.add(nxt);
        stack.push(nxt);
      }
    }
  }
  return false;
}

/**
 * 连线校验（对应旧版 `main-dag/util.ts#validateConnection`，并补充成环检测）：
 * 1. 不能自连；
 * 2. 只能从输出端口连到输入端口；
 * 3. 端口类型需有交集（任一端未声明类型时放行）；
 * 4. 不能与已有边重复，且一个输入端口只能被连一次；
 * 5. 不能形成环。
 */
export function validateConnection(
  candidate: ConnectionCandidate,
  ctx: { edges: GraphEdgeLike[]; portsOf: (nodeId: string) => PortDef[] | undefined },
): ConnectionResult {
  const { source, target, sourceAnchor, targetAnchor } = candidate;
  if (source === target) return { ok: false, reason: 'SELF' };
  const srcPorts = ctx.portsOf(source);
  const tgtPorts = ctx.portsOf(target);
  const srcParsed = parseAnchor(sourceAnchor);
  const tgtParsed = parseAnchor(targetAnchor);
  if (!srcParsed || !tgtParsed) return { ok: false, reason: 'NO_PORT' };
  if (srcParsed.kind !== 'output' || tgtParsed.kind !== 'input') return { ok: false, reason: 'DIRECTION' };
  if (srcParsed.nodeId !== source || tgtParsed.nodeId !== target) return { ok: false, reason: 'DIRECTION' };
  const srcPort = srcPorts?.find((p) => p.id === sourceAnchor);
  const tgtPort = tgtPorts?.find((p) => p.id === targetAnchor);
  // 端口定义已知时必须存在（例如 report 输出不能连出）。
  if (srcPorts && srcPorts.length > 0 && !srcPort) return { ok: false, reason: 'NO_PORT' };
  if (tgtPorts && tgtPorts.length > 0 && !tgtPort) return { ok: false, reason: 'NO_PORT' };
  if (srcPort && tgtPort && srcPort.types.length > 0 && tgtPort.types.length > 0) {
    const compatible = srcPort.types.some((t) => tgtPort.types.includes(t));
    if (!compatible) return { ok: false, reason: 'TYPE' };
  }
  if (ctx.edges.some((e) => e.sourceAnchor === sourceAnchor && e.targetAnchor === targetAnchor)) {
    return { ok: false, reason: 'DUPLICATE' };
  }
  if (ctx.edges.some((e) => e.targetAnchor === targetAnchor)) return { ok: false, reason: 'OCCUPIED' };
  if (isReachable(target, source, ctx.edges)) return { ok: false, reason: 'CYCLE' };
  return { ok: true };
}

/**
 * 点击式连线（不指定具体端口）时自动挑选一对兼容端口：
 * 依次尝试源节点输出端口 × 目标节点空闲输入端口，返回第一个校验通过的组合。
 * 若都失败，返回最具代表性的失败原因。
 */
export function pickConnection(
  source: string,
  target: string,
  ctx: { edges: GraphEdgeLike[]; portsOf: (nodeId: string) => PortDef[] | undefined },
): { ok: true; candidate: ConnectionCandidate } | { ok: false; reason: ConnectionError } {
  if (source === target) return { ok: false, reason: 'SELF' };
  const outs = (ctx.portsOf(source) || []).filter((p) => p.group === 'output');
  const ins = (ctx.portsOf(target) || []).filter((p) => p.group === 'input');
  if (outs.length === 0 || ins.length === 0) return { ok: false, reason: 'NO_PORT' };
  const reasons = new Set<ConnectionError>();
  for (const input of ins) {
    for (const output of outs) {
      const candidate = { source, target, sourceAnchor: output.id, targetAnchor: input.id };
      const res = validateConnection(candidate, ctx);
      if (res.ok) return { ok: true, candidate };
      // 成环与端口无关，直接返回。
      if (res.reason === 'CYCLE') return { ok: false, reason: 'CYCLE' };
      reasons.add(res.reason);
    }
  }
  const priority: ConnectionError[] = ['DUPLICATE', 'OCCUPIED', 'TYPE', 'NO_PORT', 'DIRECTION'];
  return { ok: false, reason: priority.find((r) => reasons.has(r)) ?? 'TYPE' };
}

/** 获取下游（含传递）节点 ID。 */
export function downstreamIds(nodeId: string, edges: GraphEdgeLike[]): string[] {
  const visited = new Set<string>();
  const queue = [nodeId];
  while (queue.length) {
    const cur = queue.shift()!;
    for (const e of edges) {
      if (e.source === cur && !visited.has(e.target)) {
        visited.add(e.target);
        queue.push(e.target);
      }
    }
  }
  return [...visited];
}

/** 获取上游（含传递）节点 ID。 */
export function upstreamIds(nodeId: string, edges: GraphEdgeLike[]): string[] {
  const visited = new Set<string>();
  const queue = [nodeId];
  while (queue.length) {
    const cur = queue.shift()!;
    for (const e of edges) {
      if (e.target === cur && !visited.has(e.source)) {
        visited.add(e.source);
        queue.push(e.source);
      }
    }
  }
  return [...visited];
}

/** 直接上游节点（按输入端口顺序）。 */
export function directUpstream(nodeId: string, edges: GraphEdgeLike[]): Array<{ nodeId: string; sourceAnchor?: string; inputIndex: number }> {
  return edges
    .filter((e) => e.target === nodeId)
    .map((e) => ({ nodeId: e.source, sourceAnchor: e.sourceAnchor, inputIndex: parseAnchor(e.targetAnchor)?.index ?? 0 }))
    .sort((a, b) => a.inputIndex - b.inputIndex);
}

/** 需要在重连 / 粘贴时清空 nodeDef.attrs 的“参数修改”类组件。 */
export const MODIFICATION_CODE_NAMES = [
  'feature/binning_modifications',
  'preprocessing/model_param_modifications',
];

export function isModificationNode(codeName?: string): boolean {
  return !!codeName && MODIFICATION_CODE_NAMES.includes(codeName);
}

/** 清空 nodeDef 中的 attrPaths / attrs（保留 domain/name/version）。 */
export function clearNodeDefAttrs<T extends Record<string, unknown> | undefined>(nodeDef: T): T {
  if (!nodeDef) return nodeDef;
  const rest = { ...(nodeDef as Record<string, unknown>) };
  delete rest.attrs;
  delete rest.attrPaths;
  return rest as T;
}

/** 支持断点续跑（继续执行）的训练组件。 */
export const CONTINUE_RUN_CODE_NAMES = ['ml.train/sgb_train', 'ml.train/ss_glm_train', 'ml.train/ss_xgb_train'];
/** 展示训练进度的组件。 */
export const PROGRESS_CODE_NAMES = [...CONTINUE_RUN_CODE_NAMES, 'ml.train/ss_sgd_train'];
