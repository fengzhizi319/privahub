/**
 * DAG 页面适配层：后端（Java 契约，兼容旧 Go 形态）↔ dag-next 画布模型。
 */
import type { GraphDetailJava, GraphNodeDetailJava, GraphStatusJava, ComponentSummaryJava, ProjectJobSummaryVOJava } from '@secretpad/api-client';
import type { DAGComponentDef, DAGEdge, DAGNode, DAGNodeStatus, ComponentIoLike, ExecutionRecord, JobStatus } from '@secretpad/dag-next';
import {
  PROGRESS_CODE_NAMES,
  CONTINUE_RUN_CODE_NAMES,
  buildAttributeDefs,
  buildOutputIds,
  buildPorts,
  buildResultOutputs,
  deriveInputs,
  edgeIdOf,
  isConfigFinished,
  toCanvasStatus,
  normalizeStatus,
  isActiveStatus,
  NODE_STATUS,
} from '@secretpad/dag-next';

export type ComponentDefLike = ComponentIoLike & {
  attrs?: Array<Record<string, unknown>>;
  inputs?: Array<Record<string, unknown> & { types?: string[] }>;
  outputs?: Array<Record<string, unknown> & { types?: string[] }>;
  desc?: string;
  version?: string;
  domain?: string;
  name?: string;
};

/** 数据表树拖入画布时创建的样本表组件（旧版 data-table-tree → read_data/datatable）。 */
export const READ_DATA_COMPONENT = { domain: 'read_data', name: 'datatable', version: '1.0.0', icon: '📥' };

export function splitCodeName(codeName?: string): { domain: string; name: string } {
  if (!codeName) return { domain: 'unknown', name: 'unknown' };
  const parts = codeName.split('/');
  if (parts.length >= 2) return { domain: parts.slice(0, -1).join('/'), name: parts[parts.length - 1] };
  return { domain: codeName, name: codeName };
}

export function iconFor(domain: string): string {
  if (domain === 'read_data' || domain.includes('data_prep')) return '📥';
  if (domain.startsWith('ml')) return '🤖';
  if (domain.startsWith('stats')) return '📊';
  if (domain.startsWith('privacy')) return '🔒';
  if (domain.startsWith('feature')) return '🧩';
  return '⚙️';
}

/** 为节点补充端口、结果输出与配置完成度（需要组件定义）。 */
export function decorateNode(node: DAGNode, def: ComponentDefLike | undefined): DAGNode {
  if (!def) return node;
  const defs = buildAttributeDefs(def);
  return {
    ...node,
    ports: buildPorts(node.id, def),
    resultOutputs: buildResultOutputs(node.id, def),
    outputs: node.outputs && node.outputs.length > 0 ? node.outputs : buildOutputIds(node.id, def),
    configFinished: isConfigFinished(node.nodeDef, defs),
  };
}

export function mapGraphNode(n: GraphNodeDetailJava, defs: Record<string, ComponentDefLike | undefined>, i18n: (s: string) => string): DAGNode {
  const codeName = n.codeName || '';
  const { domain } = splitCodeName(codeName);
  const base: DAGNode = {
    id: n.graphNodeId,
    name: n.label || i18n(splitCodeName(codeName).name) || codeName || 'Node',
    category: domain || 'Unknown',
    icon: iconFor(domain),
    status: toCanvasStatus(n.status),
    x: n.x ?? 100,
    y: n.y ?? 100,
    progress: PROGRESS_CODE_NAMES.includes(codeName) ? n.progress : undefined,
    codeName,
    nodeDef: n.nodeDef,
    inputs: n.inputs,
    outputs: n.outputs,
  };
  return decorateNode(base, defs[codeName]);
}

export function mapGraphToDAG(
  graph: GraphDetailJava | null | undefined,
  defs: Record<string, ComponentDefLike | undefined> = {},
  i18n: (s: string) => string = (s) => s,
): { nodes: DAGNode[]; edges: DAGEdge[] } {
  if (!graph) return { nodes: [], edges: [] };
  const nodes = graph.nodes.filter((n) => n.graphNodeId).map((n) => mapGraphNode(n, defs, i18n));
  const ids = new Set(nodes.map((n) => n.id));
  const edges: DAGEdge[] = graph.edges
    .filter((e) => e.source && e.target && ids.has(e.source) && ids.has(e.target))
    .map((e) => ({
      id: e.edgeId || (e.sourceAnchor && e.targetAnchor ? edgeIdOf(e.sourceAnchor, e.targetAnchor) : `${e.source}-${e.target}`),
      source: e.source!,
      target: e.target!,
      sourceAnchor: e.sourceAnchor,
      targetAnchor: e.targetAnchor,
    }));
  return { nodes, edges };
}

/**
 * 画布节点 → 后端 GraphNodeInfo（旧版 saveDag）：
 * - outputs 为 `${id}-output-${i}`；inputs 由连线推导；
 * - 训练组件在 nodeDef 中写入 checkpoint_uri = outputs[0]（断点续跑）。
 */
export function toGraphNodeInfo(node: DAGNode, edges: DAGEdge[]): Record<string, unknown> {
  const { domain, name } = splitCodeName(node.codeName);
  const nodeDef: Record<string, unknown> = { domain, name, ...(node.nodeDef || {}) };
  const outputs = node.outputs || [];
  if (CONTINUE_RUN_CODE_NAMES.includes(node.codeName || '') && outputs[0]) nodeDef.checkpoint_uri = outputs[0];
  return {
    graphNodeId: node.id,
    codeName: node.codeName,
    label: node.name,
    x: Math.round(node.x),
    y: Math.round(node.y),
    inputs: deriveInputs(node.id, edges),
    outputs,
    nodeDef,
  };
}

export function toGraphEdge(e: DAGEdge) {
  return { edgeId: e.id, source: e.source, sourceAnchor: e.sourceAnchor, target: e.target, targetAnchor: e.targetAnchor };
}

/** graph/node/status → 画布状态覆盖。 */
export function toStatusOverlay(status: GraphStatusJava | null | undefined, codeNameOf: (id: string) => string | undefined) {
  const out: Record<string, { status: DAGNodeStatus; progress?: number }> = {};
  (status?.nodes || []).forEach((n) => {
    if (!n.graphNodeId) return;
    const cn = codeNameOf(n.graphNodeId) || '';
    out[n.graphNodeId] = { status: toCanvasStatus(n.status), progress: PROGRESS_CODE_NAMES.includes(cn) ? n.progress : undefined };
  });
  return out;
}

/** 是否需要继续轮询：有节点处于 INITIALIZED / RUNNING，或刚发起运行且结果尚未 finished。 */
export function shouldPollStatus(status: GraphStatusJava | null | undefined, runRequested: boolean): boolean {
  if (!status) return runRequested;
  if (status.nodes.some((n) => normalizeStatus(n.status) === NODE_STATUS.RUNNING)) return true;
  // INITIALIZED 节点只在任务未结束时才代表“排队中”：任务失败/停止后 Kuscia 不会再启动它们
  // （旧数据中它们可能一直停留在 INITIALIZED），此时继续轮询会让画布永远处于“运行中”。
  if (status.nodes.some((n) => isActiveStatus(n.status))) return !status.finished;
  return runRequested && !status.finished;
}

/** 本轮任务是否全部成功（旧版 logDagSuccess 条件）。 */
export function allSucceeded(status: GraphStatusJava | null | undefined): boolean {
  const nodes = (status?.nodes || []).filter((n) => normalizeStatus(n.status) !== NODE_STATUS.STAGING);
  return nodes.length > 0 && nodes.every((n) => normalizeStatus(n.status) === NODE_STATUS.SUCCEED);
}

export function anyFailed(status: GraphStatusJava | null | undefined): boolean {
  return (status?.nodes || []).some((n) => normalizeStatus(n.status) === NODE_STATUS.FAILED);
}

/** 按计算模式过滤组件并分组：MPC → secretflow，TEE → trustedflow。 */
export function groupComponents(
  list: ComponentSummaryJava[],
  computeMode: string | undefined,
  label: (c: ComponentSummaryJava) => string = (c) => c.domain,
): Record<string, DAGComponentDef[]> {
  // MPC → secretflow；TEE → trustedflow（及 secretpad_tee）。后端未区分 app 时不过滤。
  const wanted = (computeMode || '').toUpperCase() === 'TEE' ? ['trustedflow', 'secretpad_tee'] : ['secretflow'];
  const apps = new Set(list.map((c) => c.app));
  const filtered = wanted.some((a) => apps.has(a)) ? list.filter((c) => wanted.includes(c.app)) : list;
  const groups: Record<string, DAGComponentDef[]> = {};
  filtered.forEach((c) => {
    const g = label(c);
    (groups[g] = groups[g] || []).push({ domain: c.domain, name: c.name, version: c.version, desc: c.desc, icon: iconFor(c.domain) });
  });
  return groups;
}

/* -------------------------------------------------------------------------- */
/* 模型提交链路（旧版 dag-submit/util.ts）                                        */
/* -------------------------------------------------------------------------- */

export interface ModelChain {
  modelNode: DAGNode[];
  preNodes: DAGNode[];
  predictNode: DAGNode[];
  postNodes: DAGNode[];
}

const domainOf = (n: DAGNode) => (n.nodeDef?.domain as string | undefined) || splitCodeName(n.codeName).domain;

function ancestors(id: string, edges: DAGEdge[]): Set<string> {
  const out = new Set<string>();
  const stack = [id];
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

function descendants(id: string, edges: DAGEdge[]): Set<string> {
  const out = new Set<string>();
  const stack = [id];
  while (stack.length) {
    const cur = stack.pop()!;
    edges.filter((e) => e.source === cur).forEach((e) => {
      if (!out.has(e.target)) {
        out.add(e.target);
        stack.push(e.target);
      }
    });
  }
  return out;
}

/** 是否存在一条从 from 经 table 类型输出锚点到达 to 的路径。 */
function tablePathExists(from: DAGNode, to: DAGNode, nodes: DAGNode[], edges: DAGEdge[], seen = new Set<string>()): boolean {
  if (seen.has(from.id)) return false;
  seen.add(from.id);
  const tableOutputs = new Set((from.resultOutputs || []).filter((o) => o.type === 'table').map((o) => o.id));
  for (const e of edges.filter((x) => x.source === from.id)) {
    const isTable = tableOutputs.size === 0 || (e.sourceAnchor ? tableOutputs.has(e.sourceAnchor) : true);
    if (!isTable) continue;
    if (e.target === to.id) return true;
    const next = nodes.find((n) => n.id === e.target);
    if (next && tablePathExists(next, to, nodes, edges, seen)) return true;
  }
  return false;
}

/**
 * 自动选择模型提交链路：
 * - 选中训练组件：模型节点 = 自身，前处理 = 成功的 preprocessing 祖先（经 table 端口连到训练），
 *   后处理 = 成功的 postprocessing 后代；
 * - 选中预测组件：预测节点 = 自身，模型节点 = 最近的 ml.train 祖先，前处理同上（以预测节点为终点）。
 */
export function computeModelChain(selectedId: string, nodes: DAGNode[], edges: DAGEdge[], statusOf: (n: DAGNode) => DAGNodeStatus = (n) => n.status): ModelChain {
  const chain: ModelChain = { modelNode: [], preNodes: [], predictNode: [], postNodes: [] };
  const selected = nodes.find((n) => n.id === selectedId);
  if (!selected) return chain;
  const ok = (n: DAGNode) => statusOf(n) === 'Success';
  const d = domainOf(selected);
  const anc = ancestors(selected.id, edges);
  if (d === 'ml.train') {
    chain.modelNode.push(selected);
  } else if (d === 'ml.predict') {
    chain.predictNode.push(selected);
    const train = nodes.filter((n) => anc.has(n.id) && domainOf(n) === 'ml.train');
    if (train[0]) chain.modelNode.push(train[train.length - 1]);
  } else {
    return chain;
  }
  chain.preNodes = nodes.filter((n) => anc.has(n.id) && ok(n) && domainOf(n) === 'preprocessing' && tablePathExists(n, selected, nodes, edges));
  const desc = descendants(selected.id, edges);
  chain.postNodes = nodes.filter((n) => desc.has(n.id) && ok(n) && domainOf(n) === 'postprocessing');
  // 按拓扑先后排序（祖先在前）。
  const order = (a: DAGNode, b: DAGNode) => (ancestors(b.id, edges).has(a.id) ? -1 : ancestors(a.id, edges).has(b.id) ? 1 : 0);
  chain.preNodes.sort(order);
  chain.postNodes.sort(order);
  return chain;
}

/** 模型提交时的 modelComponent：有预测组件时只提交预测链，否则提交训练链。 */
export function submitNodesOf(chain: ModelChain): DAGNode[] {
  return chain.predictNode.length > 0
    ? [...chain.preNodes, ...chain.predictNode, ...chain.postNodes]
    : [...chain.preNodes, ...chain.modelNode, ...chain.postNodes];
}

/** 节点能否作为模型提交入口（旧版 nodeCanOpaque）。 */
export function canSubmitModelFrom(node: DAGNode | null, nodes: DAGNode[], edges: DAGEdge[], statusOf: (n: DAGNode) => DAGNodeStatus): boolean {
  if (!node || statusOf(node) !== 'Success') return false;
  if (!edges.some((e) => e.target === node.id)) return false;
  const d = domainOf(node);
  if (d === 'ml.train') return true;
  if (d === 'ml.predict') {
    const anc = ancestors(node.id, edges);
    const hasModification = nodes.some((n) => anc.has(n.id) && (n.nodeDef?.name === 'model_param_modifications' || n.codeName === 'preprocessing/model_param_modifications') && statusOf(n) === 'Success');
    const desc = descendants(node.id, edges);
    const hasPost = nodes.some((n) => desc.has(n.id) && domainOf(n) === 'postprocessing' && statusOf(n) === 'Success');
    return hasModification || hasPost;
  }
  return false;
}

/** project/job/list 记录 → dag-next ExecutionTimeline 记录（运行记录抽屉）。 */
export function toExecutionRecord(j: ProjectJobSummaryVOJava): ExecutionRecord {
  const s = normalizeStatus(j.status);
  const status: JobStatus = s === NODE_STATUS.SUCCEED || s === NODE_STATUS.FAILED || s === NODE_STATUS.STOPPED || s === NODE_STATUS.RUNNING ? s : 'PENDING';
  return {
    jobId: j.jobId || '',
    status,
    gmtCreate: j.gmtCreate || '',
    gmtFinished: j.gmtFinished || undefined,
    taskCount: j.taskCount ?? 0,
    finishedTaskCount: j.finishedTaskCount ?? 0,
    errMsg: j.errMsg || undefined,
  };
}
