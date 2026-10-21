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
import { toList } from './types';

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
    receiverKey: string | string[];
    senderKey: string | string[];
    receiverNodeId: string;
    senderNodeId: string;
    /** PSI 结果接收方（旧版 receiverPSI）；缺省为两方。 */
    receiverParties?: string[];
    x: number;
    y: number;
  }
): GraphNodeInfo {
  const rk = toList(options.receiverKey);
  const sk = toList(options.senderKey);
  const parties = options.receiverParties?.length ? options.receiverParties : [options.receiverNodeId, options.senderNodeId].filter(Boolean);
  return createNode(graphId, idx, 'data_prep/psi', '隐私求交', {
    x: options.x,
    y: options.y,
    inputs,
    outputs: [outputAnchor(graphId, idx, 0), outputAnchor(graphId, idx, 1)],
    nodeDef: {
      domain: 'data_prep',
      name: 'psi',
      version: '1.0.0',
      // 旧版：只有同时给出双方关联键时才写入 attrs。
      ...(rk.length && sk.length
        ? {
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
              { ss: rk, is_na: false },
              { ss: sk, is_na: false },
              { s: 'PROTOCOL_RR22', is_na: false },
              { b: true, is_na: false },
              { ss: parties, is_na: false },
              { is_na: true },
              { s: 'inner_join', is_na: false },
              { b: true, is_na: false },
              { b: true, is_na: false },
            ],
          }
        : {}),
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

/** 构造 TEE PSI 节点（`preprocessing/psi`）。 */
export function createTeePsiNode(
  graphId: string,
  idx: number,
  inputs: string[],
  options: {
    receiverKey: string | string[];
    senderKey: string | string[];
    x: number;
    y: number;
  }
): GraphNodeInfo {
  const rk = toList(options.receiverKey);
  const sk = toList(options.senderKey);
  return createNode(graphId, idx, 'preprocessing/psi', '隐私求交', {
    x: options.x,
    y: options.y,
    inputs,
    outputs: [outputAnchor(graphId, idx, 0)],
    nodeDef: {
      ...(rk.length && sk.length
        ? {
            attrPaths: ['input/input1/key', 'input/input2/key'],
            // 旧版快速配置写入 `{ ss: [...] }`（可多个关联键）。
            attrs: [ssAttr(rk), ssAttr(sk)],
          }
        : {}),
      domain: 'preprocessing',
      name: 'psi',
      version: '0.0.1',
    },
  });
}

/** 构造 `stats/table_statistics` 全表统计节点。 */
export function createTableStatisticsNode(
  graphId: string,
  idx: number,
  input: string,
  options: {
    x: number;
    y: number;
    features?: string[];
    version?: string;
  }
): GraphNodeInfo {
  const hasFeatures = options.features && options.features.length > 0;
  return createNode(graphId, idx, 'stats/table_statistics', '全表统计', {
    x: options.x,
    y: options.y,
    inputs: [input],
    outputs: [outputAnchor(graphId, idx, 0)],
    nodeDef: {
      ...(hasFeatures
        ? {
            attrPaths: ['input/input_ds/features'],
            attrs: [ssAttr(options.features!)],
          }
        : {}),
      domain: 'stats',
      name: 'table_statistics',
      version: options.version ?? '0.0.1',
    },
  });
}

/** 构造 `data_prep/train_test_split` 或 `preprocessing/train_test_split` 随机分割节点。 */
export function createTrainTestSplitNode(
  graphId: string,
  idx: number,
  input: string,
  options: {
    x: number;
    y: number;
    domain?: 'data_prep' | 'preprocessing';
    version?: string;
  }
): GraphNodeInfo {
  const domain = options.domain ?? 'data_prep';
  return createNode(graphId, idx, `${domain}/train_test_split`, '随机分割', {
    x: options.x,
    y: options.y,
    inputs: [input],
    outputs: [outputAnchor(graphId, idx, 0), outputAnchor(graphId, idx, 1)],
    nodeDef: {
      domain,
      name: 'train_test_split',
      version: options.version ?? '1.0.0',
    },
  });
}

/** 构造 `privacy/differential_privacy` 差分隐私节点。 */
export function createDifferentialPrivacyNode(
  graphId: string,
  idx: number,
  input: string,
  options: {
    x: number;
    y: number;
    queryType?: string;
    queryCol?: string;
    epsilonTotal?: number;
    delta?: number;
    epsilonPerQuery?: number;
    deltaPerQuery?: number;
    mechanism?: string;
    columnSensitivities?: Record<string, number>;
    bins?: unknown[];
    trueCount?: number;
    trueSum?: number;
    trueCounts?: Record<string, unknown>;
    sensitivity?: number;
    randomState?: number;
  }
): GraphNodeInfo {
  return createNode(graphId, idx, 'privacy/differential_privacy', '差分隐私', {
    x: options.x,
    y: options.y,
    inputs: [input],
    outputs: [outputAnchor(graphId, idx, 0)],
    nodeDef: {
      attrPaths: [
        'query_type',
        'query_col',
        'epsilon_total',
        'delta',
        'epsilon_per_query',
        'delta_per_query',
        'mechanism',
        'column_sensitivities_json',
        'bins_json',
        'true_count',
        'true_sum',
        'true_counts_json',
        'sensitivity',
        'random_state',
      ],
      attrs: [
        sAttr(options.queryType ?? 'mean'),
        sAttr(options.queryCol ?? 'age'),
        fAttr(options.epsilonTotal ?? 1.0),
        fAttr(options.delta ?? 0.0),
        fAttr(options.epsilonPerQuery ?? 0.1),
        fAttr(options.deltaPerQuery ?? 0.0),
        sAttr(options.mechanism ?? 'laplace'),
        jsonAttr(options.columnSensitivities ?? { age: 1.0 }),
        jsonAttr(options.bins ?? []),
        fAttr(options.trueCount ?? 0.0),
        fAttr(options.trueSum ?? 0.0),
        jsonAttr(options.trueCounts ?? {}),
        fAttr(options.sensitivity ?? 0.0),
        i64Attr(options.randomState ?? 0),
      ],
      domain: 'privacy',
      name: 'differential_privacy',
      version: '1.1.0',
    },
  });
}
