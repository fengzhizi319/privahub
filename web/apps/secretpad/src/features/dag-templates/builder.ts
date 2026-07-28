/**
 * DAG 模板构建通用工具函数。
 *
 * 这些工具封装了旧前端中反复出现的模式：
 * - 生成基于 graphId 的节点/边 ID
 * - 构造 `read_data/datatable` 节点的 `datatable_selected` 属性
 * - 构造 PSI、VIF、WOE 等常用算子的 nodeDef
 * 通过统一封装，模板实现只关注拓扑与参数，避免重复硬编码。
 */
import type { GraphEdge, GraphNodeInfo } from '@secretpad/api-client';

/** 生成节点 ID。 */
export function nodeId(graphId: string, idx: number): string {
  return `${graphId}-node-${idx}`;
}

/** 生成节点输出锚点 ID。 */
export function outputAnchor(graphId: string, nodeIdx: number, outputIdx: number): string {
  return `${nodeId(graphId, nodeIdx)}-output-${outputIdx}`;
}

/** 生成节点输入锚点 ID。 */
export function inputAnchor(graphId: string, nodeIdx: number, inputIdx: number): string {
  return `${nodeId(graphId, nodeIdx)}-input-${inputIdx}`;
}

/** 生成边 ID。 */
export function edgeId(
  graphId: string,
  sourceIdx: number,
  sourceOutputIdx: number,
  targetIdx: number,
  targetInputIdx: number
): string {
  const sourceAnchor = outputAnchor(graphId, sourceIdx, sourceOutputIdx);
  const targetAnchor = inputAnchor(graphId, targetIdx, targetInputIdx);
  return `${sourceAnchor}__${targetAnchor}`;
}

/** 构造连接两条节点的边。 */
export function connect(
  graphId: string,
  sourceIdx: number,
  sourceOutputIdx: number,
  targetIdx: number,
  targetInputIdx: number
): GraphEdge {
  return {
    edgeId: edgeId(graphId, sourceIdx, sourceOutputIdx, targetIdx, targetInputIdx),
    source: nodeId(graphId, sourceIdx),
    target: nodeId(graphId, targetIdx),
    sourceAnchor: outputAnchor(graphId, sourceIdx, sourceOutputIdx),
    targetAnchor: inputAnchor(graphId, targetIdx, targetInputIdx),
  };
}

/** 构造 `read_data/datatable` 节点的 `datatable_selected` 定义。 */
export function datatableDef(
  tableId?: string,
  partition?: string
): { attrPaths: string[]; attrs: Record<string, unknown>[] } {
  if (!tableId) return { attrPaths: [], attrs: [] };
  const selected: Record<string, unknown> = { s: tableId, is_na: false };
  if (partition) {
    return {
      attrPaths: ['datatable_selected', 'datatable_partition'],
      attrs: [selected, { s: partition, is_na: false }],
    };
  }
  return {
    attrPaths: ['datatable_selected'],
    attrs: [selected],
  };
}

/** 构造一个通用节点。 */
export function createNode(
  graphId: string,
  idx: number,
  codeName: string,
  label: string,
  options: {
    x: number;
    y: number;
    inputs?: string[];
    outputs?: string[];
    nodeDef?: Record<string, unknown>;
  }
): GraphNodeInfo {
  return {
    graphNodeId: nodeId(graphId, idx),
    codeName,
    label,
    x: options.x,
    y: options.y,
    inputs: options.inputs ?? [],
    outputs: options.outputs ?? [],
    nodeDef: options.nodeDef ?? { domain: codeName.split('/')[0], name: codeName.split('/')[1], version: '0.0.1' },
  };
}

/** 构造 `read_data/datatable` 节点。 */
export function createReadDataNode(
  graphId: string,
  idx: number,
  tableId: string | undefined,
  options: { x: number; y: number; partition?: string; label?: string }
): GraphNodeInfo {
  const def = datatableDef(tableId, options.partition);
  return createNode(graphId, idx, 'read_data/datatable', options.label ?? '样本表', {
    x: options.x,
    y: options.y,
    outputs: [outputAnchor(graphId, idx, 0)],
    nodeDef: {
      ...def,
      domain: 'read_data',
      name: 'datatable',
      version: '0.0.1',
    },
  });
}

/** 构造 PSI 节点。 */
export function createPsiNode(
  graphId: string,
  idx: number,
  inputs: string[],
  options: {
    receiverKey: string;
    senderKey: string;
    receiverNodeId: string;
    senderNodeId: string;
    x: number;
    y: number;
  }
): GraphNodeInfo {
  return createNode(graphId, idx, 'data_prep/psi', '隐私求交', {
    x: options.x,
    y: options.y,
    inputs,
    outputs: [outputAnchor(graphId, idx, 0), outputAnchor(graphId, idx, 1)],
    nodeDef: {
      domain: 'data_prep',
      name: 'psi',
      version: '1.0.0',
      attrPaths: [
        'input/input_ds1/keys',
        'input/input_ds2/keys',
        'protocol',
        'sort_result',
        'receiver_parties',
        'allow_empty_result',
        'join_type',
        'input_ds1_keys_duplicated',
        'input_ds2_keys_duplicated',
      ],
      attrs: [
        { ss: [options.receiverKey], is_na: false },
        { ss: [options.senderKey], is_na: false },
        { s: 'PROTOCOL_RR22', is_na: false },
        { b: true, is_na: false },
        { ss: [options.receiverNodeId, options.senderNodeId], is_na: false },
        { is_na: true },
        { s: 'inner_join', is_na: false },
        { b: true, is_na: false },
        { b: true, is_na: false },
      ],
    },
  });
}

/** 构造一个只有单一处理节点的隐私组件模板（数据分类分级、K-匿名、L-多样性等）。 */
export function buildSingleTablePrivacyTemplate(
  graphId: string,
  tableId: string | undefined,
  processor: {
    codeName: string;
    label: string;
    nodeDef: Record<string, unknown>;
  }
): { nodes: GraphNodeInfo[]; edges: GraphEdge[] } {
  const readNode = createReadDataNode(graphId, 1, tableId, { x: -260, y: -210 });
  const output0 = outputAnchor(graphId, 1, 0);
  const processNode = createNode(graphId, 2, processor.codeName, processor.label, {
    x: -260,
    y: -80,
    inputs: [output0],
    outputs: [outputAnchor(graphId, 2, 0), outputAnchor(graphId, 2, 1)],
    nodeDef: processor.nodeDef,
  });
  return {
    nodes: [readNode, processNode],
    edges: [connect(graphId, 1, 0, 2, 0)],
  };
}

/** 把字符串数组打包成 `ss` 类型属性。 */
export function ssAttr(values: string[]): { ss: string[]; is_na: boolean } {
  return { ss: values, is_na: false };
}

/** 把单个字符串打包成 `s` 类型属性。 */
export function sAttr(value: string): { s: string; is_na: boolean } {
  return { s: value, is_na: false };
}

/** 把 JSON 对象序列化为字符串属性。 */
export function jsonAttr(value: unknown): { s: string; is_na: boolean } {
  return { s: JSON.stringify(value), is_na: false };
}

/** 计算属性值。 */
export function i64Attr(value: number): { i64: number; is_na: boolean } {
  return { i64: value, is_na: false };
}

/** 浮点属性值。 */
export function fAttr(value: number): { f: number; is_na: boolean } {
  return { f: value, is_na: false };
}

/** 布尔属性值。 */
export function bAttr(value: boolean): { b: boolean; is_na: boolean } {
  return { b: value, is_na: false };
}

/** 缺失属性占位。 */
export function naAttr(): { is_na: boolean } {
  return { is_na: true };
}
