/**
 * AT_CUSTOM_PROTOBUF 自定义序列化器注册表。
 *
 * 逐字对齐旧版 `config-item-render/custom-serializer-registry.ts` 及各渲染器中的
 * serializer / unserializer：
 * - 表单值 → nodeDef.attrs 元素：`{ custom_value: <proto JSON>, custom_protobuf_cls: <cls> }`；
 * - 分箱修改 / 线性模型参数修改不走表单，保存为 `{ s: JSON.stringify(...) }`。
 */

/* -------------------------------------------------------------------------- */
/* FeatureColumnConfig（上游输出特征）                                           */
/* -------------------------------------------------------------------------- */

export type Feature = { tableName: string; tableFeatures: string[]; nodeName: string };

export const upstreamOutputFeatureSerializer = (val: Feature[], clsName: string) => ({
  custom_value: val,
  custom_protobuf_cls: clsName,
});

export const upstreamOutputFeatureUnserializer = (val?: { custom_value?: Feature[] }): Feature[] => {
  if (!val || !val.custom_value) return [];
  return val.custom_value;
};

/* -------------------------------------------------------------------------- */
/* CaseWhenRule                                                                 */
/* -------------------------------------------------------------------------- */

export type CaseWhenValueType = 'COLUMN' | 'CONST_INT' | 'CONST_FLOAT' | 'CONST_STR';
export interface CaseWhenValue {
  type?: CaseWhenValueType | string;
  value?: string | number;
  i?: number;
  f?: number;
  s?: string;
  column_name?: string;
}
export interface CaseWhenCond {
  cond_column?: string;
  op?: 'EQ' | 'NE' | 'LT' | 'GT' | 'LE' | 'GE' | string;
  cond_value?: CaseWhenValue;
  connection?: 'AND' | 'OR' | string;
}
export interface CaseWhenFormWhen {
  when?: CaseWhenCond;
  conds?: CaseWhenCond[];
  then?: CaseWhenValue;
}
export interface CaseWhenForm {
  whens?: CaseWhenFormWhen[];
  else_value?: CaseWhenValue;
  output_column?: string;
  as_label?: boolean;
  float_epsilon?: number;
}

export const caseWhenSerializer = (val: CaseWhenForm, clsName: string) => {
  const cw = val;
  const { whens, else_value, output_column, as_label, float_epsilon } = cw || {};
  if (whens && whens.length > 0) {
    const convertedWhens = whens.map((w) => {
      const connections: string[] = [];
      const { conds, when, then } = w;
      const convertedConds: CaseWhenCond[] = [when as CaseWhenCond];
      conds?.forEach((cond) => {
        const { connection, ...rest } = cond;
        if (connection) connections.push(connection);
        if (rest) convertedConds.push(rest);
      });
      return { conds: convertedConds, then, connections };
    });
    return {
      custom_protobuf_cls: clsName,
      custom_value: { whens: convertedWhens, else_value, output_column, as_label, float_epsilon },
    };
  }
  return { custom_value: cw, custom_protobuf_cls: clsName };
};

export const caseWhenUnserializer = (val?: { custom_value?: Record<string, unknown> }): CaseWhenForm => {
  if (!val || !val.custom_value) {
    return {
      whens: [{ then: { type: 'CONST_FLOAT' }, when: { cond_value: { type: 'CONST_FLOAT' }, op: 'EQ' } }],
      else_value: { type: 'CONST_FLOAT', value: undefined },
      output_column: undefined,
      as_label: false,
      float_epsilon: 10e-7,
    };
  }
  const { whens = [], ...rest } = val.custom_value as { whens?: Array<{ connections?: string[]; conds?: CaseWhenCond[]; then?: CaseWhenValue }> };
  const convertedWhens = whens.map((w) => {
    const { connections, then } = w;
    const conds = [...(w.conds || [])].map((c) => ({ ...c }));
    let when: CaseWhenCond | undefined;
    if (conds.length > 0) when = conds.shift();
    if (connections && connections.length > 0) {
      connections.forEach((connection, index) => {
        if (conds[index]) conds[index].connection = connection;
      });
    }
    return when ? { when, conds, then } : { conds, then };
  });
  return { whens: convertedWhens, ...(rest as Omit<CaseWhenForm, 'whens'>) };
};

/* -------------------------------------------------------------------------- */
/* CalculateOpRules                                                             */
/* -------------------------------------------------------------------------- */

export const CALCULATE_OPS = [
  'STANDARDIZE',
  'NORMALIZATION',
  'RANGE_LIMIT',
  'UNARY',
  'ROUND',
  'LOG_ROUND',
  'SQRT',
  'LOG',
  'EXP',
  'LENGTH',
  'SUBSTR',
  'RECIPROCAL',
] as const;
export type CalculateOp = (typeof CALCULATE_OPS)[number];

export interface CalculateForm {
  op: CalculateOp | string;
  operands?: Array<string | number | null>;
  newColNames?: string;
}

export const calculateSerializer = (val: CalculateForm, clsName: string) => {
  const cw: Record<string, unknown> = { ...(val || {}) };
  const { newColNames } = (val || {}) as CalculateForm;
  if (newColNames) {
    cw.newColNames = newColNames.split(',').filter((col: string) => col.length > 0);
  }
  return { custom_value: cw, custom_protobuf_cls: clsName };
};

export const calculateUnserializer = (val?: { custom_value?: Record<string, unknown> }): CalculateForm => {
  if (!val || !val.custom_value) return { op: 'STANDARDIZE' };
  const customValue = { ...val.custom_value } as Record<string, unknown>;
  const { newColNames } = customValue;
  if (Array.isArray(newColNames)) customValue.newColNames = newColNames.join(',');
  return customValue as unknown as CalculateForm;
};

/** 各算子的操作数初始值（旧版 operandRenderMap）。 */
export const CALCULATE_OPERAND_INIT: Partial<Record<CalculateOp, Array<string | number | null>>> = {
  UNARY: ['+', '/', null],
  LOG: ['e', null],
  LOG_ROUND: [null],
  RANGE_LIMIT: [null, null],
  SUBSTR: [null],
};

/* -------------------------------------------------------------------------- */
/* GroupbyAggregationConfig                                                     */
/* -------------------------------------------------------------------------- */

export const AGGREGATION_FUNCTIONS = ['SUM', 'MEAN', 'VAR', 'MIN', 'MAX', 'COUNT'] as const;
export interface GroupByConfig {
  column_queries: Array<{ function?: string; column_name?: string }>;
}

export const groupbySerializer = (val: GroupByConfig, clsName: string) => {
  const { column_queries } = val;
  return { custom_value: { column_queries }, custom_protobuf_cls: clsName };
};

export const groupbyUnserializer = (val?: { custom_value?: GroupByConfig; custom_protobuf_cls?: string }): GroupByConfig => {
  if (!val) return { column_queries: [{}, {}] };
  return { ...(val.custom_value as GroupByConfig) };
};

/* -------------------------------------------------------------------------- */
/* 分箱修改（Binning_modifications）                                             */
/* -------------------------------------------------------------------------- */

export interface BinFormItem {
  key: string;
  label: string;
  markForMerge: boolean;
  totalCount: number;
  woe?: number;
  order?: number;
}
export interface BinningFormRecord {
  key: string;
  partyName: string;
  feature: string;
  type: string;
  isWoe: boolean;
  binCount: number;
  iv?: number;
  bins: BinFormItem[];
}
export interface BinningFormData {
  modelHash: string;
  variableBins: BinningFormRecord[];
}

/** 渲染区间标签（旧版 helper.getLabel）。 */
export const getBinLabel = (leftBound: number | string, rightBound: number | string) => {
  if (leftBound === 0 && rightBound === 0) return 'ELSE';
  let rightBoundBracket = ']';
  if (typeof rightBound === 'string' && rightBound.includes('Infinity')) rightBoundBracket = ')';
  return `(${leftBound}, ${rightBound}${rightBoundBracket}`;
};

/** 标签 → 引擎区间值（旧版 helper.getBoundValue）。 */
export const getBinBoundValue = (label: string): [number | string, number | string] => {
  if (label === 'ELSE') return [0, 0];
  const left = label.split(',')[0].replace('(', '').trim();
  const right = label.split(',')[1].replace(']', '').replace(')', '').trim();
  return [left.includes('Infinity') ? left : Number(left), right.includes('Infinity') ? right : Number(right)];
};

export const binModificationsSerializer = (parametersData: BinningFormData) => {
  const variableBins = parametersData?.variableBins?.map((record) => {
    const { feature, isWoe, binCount, type, iv, bins, partyName } = record;
    const recordParams = isWoe ? { iv } : {};
    const elseBin = bins.find((bin) => bin.label === 'ELSE');
    const validBins = bins.filter((bin) => bin.label !== 'ELSE');
    return {
      ...recordParams,
      isWoe,
      featureName: feature,
      featureType: type === 'float' ? 'numeric' : type,
      validBinCount: binCount,
      partyName,
      elseBin: {
        fillingValue: isWoe ? elseBin?.woe : elseBin?.order,
        totalCount: elseBin?.totalCount,
        leftBound: 0,
        rightBound: 0,
        markForMerge: false,
      },
      validBins: validBins.map((bin) => {
        const { markForMerge, label, woe, order, totalCount } = bin;
        return {
          markForMerge,
          leftBound: getBinBoundValue(label)[0],
          rightBound: getBinBoundValue(label)[1],
          fillingValue: isWoe ? woe : order,
          totalCount,
        };
      }),
    };
  });
  return { modelHash: parametersData.modelHash, variableBins };
};

interface RawBin {
  markForMerge?: boolean;
  leftBound: number | string;
  rightBound: number | string;
  fillingValue?: number;
  totalCount?: number;
}

export const binModificationsUnSerializer = (origin?: {
  modelHash: string;
  variableBins: Array<{
    featureName: string;
    validBinCount: number;
    iv?: number;
    featureType: string;
    validBins: RawBin[];
    isWoe: boolean;
    elseBin?: RawBin;
    partyName: string;
  }>;
}): BinningFormData | undefined => {
  if (!origin) return undefined;
  const variableBins = origin.variableBins?.map((record) => {
    const { featureName, validBinCount, iv, featureType, validBins, isWoe, elseBin, partyName } = record;
    const recordParams = isWoe ? { iv } : {};
    const bins = elseBin ? [...validBins, elseBin] : [...validBins];
    return {
      ...recordParams,
      key: featureName,
      feature: featureName,
      binCount: validBinCount,
      type: featureType === 'numeric' ? 'float' : featureType,
      isWoe,
      partyName,
      bins: bins.map((bin, index) => {
        const { markForMerge, leftBound, rightBound, fillingValue, totalCount } = bin;
        const binParams = isWoe ? { woe: fillingValue } : { order: fillingValue };
        const label = getBinLabel(leftBound, rightBound);
        return {
          ...binParams,
          key: `${index}/${featureName}/${label}`,
          label,
          markForMerge: !!markForMerge,
          totalCount: totalCount ?? 0,
        };
      }),
    };
  });
  return { modelHash: origin.modelHash, variableBins };
};

/* -------------------------------------------------------------------------- */
/* 线性模型参数修改（linear_model_pb2）                                          */
/* -------------------------------------------------------------------------- */

export interface LinearModelDatum {
  key?: string;
  featureName: string;
  party: string;
  featureWeight: number;
}
export interface LinearModelData {
  modelHash: string;
  featureWeights: LinearModelDatum[];
  bias: number;
}

export const modelModificationsSerializer = (data: LinearModelData) => ({
  modelHash: data.modelHash,
  featureWeights: data?.featureWeights?.map(({ featureName, party, featureWeight }) => ({ featureName, party, featureWeight })),
  bias: data.bias,
});

export const modelModificationsUnSerializer = (origin?: LinearModelData): LinearModelData | undefined => {
  if (!origin) return undefined;
  return {
    modelHash: origin.modelHash,
    featureWeights: origin.featureWeights?.map(({ featureWeight, featureName, party }) => ({
      key: `${featureName}-${party}`,
      featureName,
      party,
      featureWeight,
    })),
    bias: origin.bias,
  };
};

/* -------------------------------------------------------------------------- */
/* 注册表                                                                       */
/* -------------------------------------------------------------------------- */

/**
 * 自定义 protobuf 类的序列化器。用方法签名声明（参数双变），
 * 各类的具体序列化器（如 `CustomSerializer<BinningFormData>`）因此可以放进
 * 按 `unknown` 存储的注册表，而无需 `any`。
 */
export interface CustomSerializer<T = unknown> {
  /** 表单值 → nodeDef attr。 */
  serializer(val: T, clsName: string): Record<string, unknown>;
  /** nodeDef attr（可空）→ 表单值。 */
  unserializer(val?: Record<string, unknown>): T;
}

/** 以 `{ s: JSON }` 形式保存的“参数修改”类（不经表单序列化）。 */
function jsonStringSerializer<T>(ser: (v: T) => unknown, unser: (v: never) => T | undefined): CustomSerializer<T | undefined> {
  return {
    serializer: (val) => ({ s: JSON.stringify(ser(val as T)) }),
    unserializer: (attr) => {
      const s = attr?.s;
      if (typeof s !== 'string' || !s) return undefined;
      try {
        return unser(JSON.parse(s) as never);
      } catch {
        return undefined;
      }
    },
  };
}

export const customSerializerRegistry: Record<string, CustomSerializer> = {
  'feature_column_config_pb2.FeatureColumnConfig': {
    serializer: (v, cls) => upstreamOutputFeatureSerializer(v as Feature[], cls),
    unserializer: (v) => upstreamOutputFeatureUnserializer(v as { custom_value?: Feature[] }),
  },
  'case_when_rules_pb2.CaseWhenRule': {
    serializer: (v, cls) => caseWhenSerializer(v as CaseWhenForm, cls),
    unserializer: (v) => caseWhenUnserializer(v as { custom_value?: Record<string, unknown> }),
  },
  'calculate_rules_pb2.CalculateOpRules': {
    serializer: (v, cls) => calculateSerializer(v as CalculateForm, cls),
    unserializer: (v) => calculateUnserializer(v as { custom_value?: Record<string, unknown> }),
  },
  'groupby_aggregation_config_pb2.GroupbyAggregationConfig': {
    serializer: (v, cls) => groupbySerializer(v as GroupByConfig, cls),
    unserializer: (v) => groupbyUnserializer(v as { custom_value?: GroupByConfig }),
  },
  Binning_modifications: jsonStringSerializer<BinningFormData>(binModificationsSerializer, binModificationsUnSerializer as never),
  linear_model_pb2: jsonStringSerializer<LinearModelData>(modelModificationsSerializer, modelModificationsUnSerializer as never),
};

/** 这两类组件的配置通过专用抽屉编辑，不在表单中显示“保存”。 */
export const DRAWER_EDITED_PROTOBUF_CLS = ['Binning_modifications', 'linear_model_pb2'];

export function getCustomSerializer(cls?: string): CustomSerializer | undefined {
  return cls ? customSerializerRegistry[cls] : undefined;
}
