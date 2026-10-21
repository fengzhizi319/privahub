/**
 * 组件配置模型（对应旧版 `modules/component-config/**`）。
 *
 * 职责：
 * 1. 规范化后端组件定义（兼容 Java camelCase、proto snake_case、数字枚举与字符串数字）；
 * 2. 把 `inputs[].attrs` 展开为 `input/<inputName>/<colName>` 的列选择属性（带 col_min/max_cnt）；
 * 3. renderKey 注册表：按 `codeName` + 属性路径/类型决定渲染器（镜像旧版 config-render-contribution）；
 * 4. 面板样式注册表：isShow / order / isAdvancedConfig / noWrap；
 * 5. 属性校验（必填、数值上下界、列表长度、列数量）与节点配置完成度 isConfigFinished。
 */

/* -------------------------------------------------------------------------- */
/* 类型                                                                         */
/* -------------------------------------------------------------------------- */

export type AttrTypeName =
  | 'ATTR_TYPE_UNSPECIFIED'
  | 'AT_FLOAT'
  | 'AT_INT'
  | 'AT_STRING'
  | 'AT_BOOL'
  | 'AT_FLOATS'
  | 'AT_INTS'
  | 'AT_STRINGS'
  | 'AT_BOOLS'
  | 'AT_STRUCT_GROUP'
  | 'AT_UNION_GROUP'
  | 'AT_CUSTOM_PROTOBUF'
  | 'AT_PARTY'
  | 'AT_COL_PARAMS'
  | 'AT_SF_TABLE_COL'
  | 'AT_SF_TABLE'
  | 'AT_MODEL';

/**
 * proto 枚举值 → 枚举名（secretflow_spec v1 `AttrType`，已用 component_pb2 校对）。
 * 旧版 SecretPad 后端以字符串返回，同样兼容。
 */
export const ATTR_TYPE_BY_NUMBER: Record<number, AttrTypeName> = {
  0: 'ATTR_TYPE_UNSPECIFIED',
  1: 'AT_FLOAT',
  2: 'AT_INT',
  3: 'AT_STRING',
  4: 'AT_BOOL',
  5: 'AT_FLOATS',
  6: 'AT_INTS',
  7: 'AT_STRINGS',
  8: 'AT_BOOLS',
  9: 'AT_STRUCT_GROUP',
  10: 'AT_UNION_GROUP',
  11: 'AT_CUSTOM_PROTOBUF',
  12: 'AT_PARTY',
  13: 'AT_COL_PARAMS',
};

/** 属性值（proto Attribute）。 */
export interface AttributeValue {
  f?: number;
  i64?: number;
  s?: string;
  b?: boolean;
  fs?: number[];
  i64s?: number[];
  ss?: string[];
  bs?: boolean[];
  is_na?: boolean;
  custom_protobuf_cls?: string;
  custom_value?: unknown;
}

export interface AtomicAttrDesc {
  list_min_length_inclusive?: number;
  list_max_length_inclusive?: number;
  is_optional?: boolean;
  default_value?: AttributeValue;
  allowed_values?: AttributeValue;
  lower_bound_enabled?: boolean;
  lower_bound?: AttributeValue;
  lower_bound_inclusive?: boolean;
  upper_bound_enabled?: boolean;
  upper_bound?: AttributeValue;
  upper_bound_inclusive?: boolean;
}

/** 规范化后的属性定义。 */
export interface AttributeDef {
  prefixes?: string[];
  name?: string;
  desc?: string;
  type?: AttrTypeName | number;
  atomic?: AtomicAttrDesc;
  union?: { default_selection?: string };
  custom_protobuf_cls?: string;
  /** 输入表列属性：来自第几个输入。 */
  fromInputIndex?: number;
  col_min_cnt_inclusive?: number;
  col_max_cnt_inclusive?: number;
}

/** attrs 值类型 → Attribute 字段（对应旧版 utils.typesMap）。 */
export const TYPES_MAP: Partial<Record<AttrTypeName, keyof AttributeValue>> = {
  AT_BOOL: 'b',
  AT_INT: 'i64',
  AT_FLOAT: 'f',
  AT_STRING: 's',
  AT_BOOLS: 'bs',
  AT_FLOATS: 'fs',
  AT_INTS: 'i64s',
  AT_STRINGS: 'ss',
  AT_PARTY: 'ss',
  AT_SF_TABLE_COL: 'ss',
  AT_COL_PARAMS: 'ss',
  AT_UNION_GROUP: 's',
  AT_SF_TABLE: 's',
  AT_MODEL: 's',
};

/* -------------------------------------------------------------------------- */
/* 规范化                                                                       */
/* -------------------------------------------------------------------------- */

function pick(obj: Record<string, unknown> | undefined, ...keys: string[]): unknown {
  if (!obj) return undefined;
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k];
  }
  return undefined;
}

function toNum(v: unknown): number | undefined {
  if (v === undefined || v === null || v === '') return undefined;
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isNaN(n) ? undefined : n;
}

function toBool(v: unknown): boolean | undefined {
  if (v === undefined || v === null) return undefined;
  if (typeof v === 'boolean') return v;
  if (v === 'true') return true;
  if (v === 'false') return false;
  return Boolean(v);
}

/** 规范化 Attribute 值（i64 字符串 → 数字，isNa → is_na）。 */
export function normalizeAttrValue(raw: unknown): AttributeValue | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const r = raw as Record<string, unknown>;
  const out: AttributeValue = {};
  if (r.f !== undefined) out.f = toNum(r.f);
  if (r.i64 !== undefined) out.i64 = toNum(r.i64);
  if (r.s !== undefined) out.s = String(r.s);
  if (r.b !== undefined) out.b = toBool(r.b);
  if (Array.isArray(r.fs)) out.fs = r.fs.map((v) => toNum(v) ?? 0);
  if (Array.isArray(r.i64s)) out.i64s = r.i64s.map((v) => toNum(v) ?? 0);
  if (Array.isArray(r.ss)) out.ss = r.ss.map((v) => (v === null || v === undefined ? v : String(v))) as string[];
  if (Array.isArray(r.bs)) out.bs = r.bs.map((v) => !!toBool(v));
  const na = pick(r, 'is_na', 'isNa');
  if (na !== undefined) out.is_na = !!toBool(na);
  if (r.custom_protobuf_cls !== undefined) out.custom_protobuf_cls = String(r.custom_protobuf_cls);
  if (r.custom_value !== undefined) out.custom_value = r.custom_value;
  return out;
}

export function normalizeType(type: unknown): AttrTypeName {
  if (typeof type === 'number') return ATTR_TYPE_BY_NUMBER[type] ?? 'ATTR_TYPE_UNSPECIFIED';
  if (typeof type === 'string' && type.length > 0) {
    if (/^\d+$/.test(type)) return ATTR_TYPE_BY_NUMBER[Number(type)] ?? 'ATTR_TYPE_UNSPECIFIED';
    return type as AttrTypeName;
  }
  return 'ATTR_TYPE_UNSPECIFIED';
}

/** 规范化单个属性定义（兼容 camelCase / snake_case）。 */
export function normalizeAttrDef(raw: Record<string, unknown>): AttributeDef {
  const atomicRaw = (raw.atomic as Record<string, unknown> | undefined) ?? undefined;
  const unionRaw = (raw.union as Record<string, unknown> | undefined) ?? undefined;
  const def: AttributeDef = {
    prefixes: Array.isArray(raw.prefixes) ? (raw.prefixes as unknown[]).map(String) : undefined,
    name: raw.name !== undefined ? String(raw.name) : undefined,
    desc: raw.desc !== undefined ? String(raw.desc) : undefined,
    type: normalizeType(raw.type),
    custom_protobuf_cls: pick(raw, 'custom_protobuf_cls', 'customProtobufCls') as string | undefined,
  };
  if (atomicRaw) {
    def.atomic = {
      list_min_length_inclusive: toNum(pick(atomicRaw, 'list_min_length_inclusive', 'listMinLengthInclusive')),
      list_max_length_inclusive: toNum(pick(atomicRaw, 'list_max_length_inclusive', 'listMaxLengthInclusive')),
      is_optional: toBool(pick(atomicRaw, 'is_optional', 'isOptional')),
      default_value: normalizeAttrValue(pick(atomicRaw, 'default_value', 'defaultValue')),
      allowed_values: normalizeAttrValue(pick(atomicRaw, 'allowed_values', 'allowedValues')),
      lower_bound_enabled: toBool(pick(atomicRaw, 'lower_bound_enabled', 'lowerBoundEnabled')),
      lower_bound: normalizeAttrValue(pick(atomicRaw, 'lower_bound', 'lowerBound')),
      lower_bound_inclusive: toBool(pick(atomicRaw, 'lower_bound_inclusive', 'lowerBoundInclusive')),
      upper_bound_enabled: toBool(pick(atomicRaw, 'upper_bound_enabled', 'upperBoundEnabled')),
      upper_bound: normalizeAttrValue(pick(atomicRaw, 'upper_bound', 'upperBound')),
      upper_bound_inclusive: toBool(pick(atomicRaw, 'upper_bound_inclusive', 'upperBoundInclusive')),
    };
  }
  if (unionRaw) {
    def.union = { default_selection: pick(unionRaw, 'default_selection', 'defaultSelection') as string | undefined };
  }
  if (raw.fromInputIndex !== undefined) def.fromInputIndex = toNum(raw.fromInputIndex);
  const colMin = toNum(pick(raw, 'col_min_cnt_inclusive', 'colMinCntInclusive'));
  const colMax = toNum(pick(raw, 'col_max_cnt_inclusive', 'colMaxCntInclusive'));
  if (colMin !== undefined) def.col_min_cnt_inclusive = colMin;
  if (colMax !== undefined) def.col_max_cnt_inclusive = colMax;
  return def;
}

/**
 * 把 `inputs[].attrs` 展开为列选择属性（对应旧版 ComponentConfigRegistry.createInputConfigNode）：
 * - 路径 `input/<inputName>/<colName>`，类型 AT_SF_TABLE_COL；
 * - col_min_cnt_inclusive 缺省为 0，为 0 时非必填；
 * - 额外生成 `input`、`input/<inputName>` 两级结构组，便于分组展示（结构组不参与序列化）。
 */
export function buildInputColumnDefs(inputs: Array<Record<string, unknown>> | undefined): AttributeDef[] {
  const out: AttributeDef[] = [];
  if (!Array.isArray(inputs)) return out;
  let hasAny = false;
  inputs.forEach((input, inputIndex) => {
    const attrs = input?.attrs as Array<Record<string, unknown>> | undefined;
    if (!Array.isArray(attrs) || attrs.length === 0) return;
    const inputName = String(input.name ?? `input_${inputIndex}`);
    if (!hasAny) {
      out.push({ name: 'input', type: 'AT_STRUCT_GROUP' });
      hasAny = true;
    }
    out.push({ prefixes: ['input'], name: inputName, desc: input.desc as string | undefined, type: 'AT_STRUCT_GROUP' });
    attrs.forEach((attr) => {
      const colMin = toNum(pick(attr, 'col_min_cnt_inclusive', 'colMinCntInclusive')) ?? 0;
      const colMax = toNum(pick(attr, 'col_max_cnt_inclusive', 'colMaxCntInclusive'));
      out.push({
        prefixes: ['input', inputName],
        name: String(attr.name ?? ''),
        desc: attr.desc as string | undefined,
        type: 'AT_SF_TABLE_COL',
        fromInputIndex: inputIndex,
        col_min_cnt_inclusive: colMin,
        col_max_cnt_inclusive: colMax,
        atomic: { is_optional: colMin === 0 },
      });
    });
  });
  return out;
}

/** 组件定义 → 表单属性定义（输入列属性在前，与旧版一致）。 */
export function buildAttributeDefs(component: {
  attrs?: Array<Record<string, unknown>>;
  inputs?: Array<Record<string, unknown>>;
} | null | undefined): AttributeDef[] {
  if (!component) return [];
  const inputDefs = buildInputColumnDefs(component.inputs);
  const attrDefs = (component.attrs || []).map((a) => normalizeAttrDef(a as Record<string, unknown>));
  return [...inputDefs, ...attrDefs];
}

export function attrPathOf(def: Pick<AttributeDef, 'prefixes' | 'name'>): string {
  return [...(def.prefixes ?? []), def.name ?? ''].filter((p) => p !== '').join('/');
}

/* -------------------------------------------------------------------------- */
/* renderKey 注册表                                                             */
/* -------------------------------------------------------------------------- */

/** 旧版 component-config-protocol.ts#codeNameRenderKey。 */
export const CODE_NAME_RENDER_KEY: Record<string, string> = {
  'read_data/datatable': 'DATA_TABLE_SELECT',
  'data_prep/psi': 'UNION_KEY_SELECT',
  'preprocessing/psi': 'UNION_KEY_SELECT',
  'data_prep/psi_tp': 'UNION_KEY_SELECT',
  'preprocessing/sqlite': 'SQL',
  'data_filter/sample': 'SAMPLE',
  'ml.predict/read_model': 'MODEL_SELECT',
  'stats/scql_analysis': 'SQL_ANALYSIS',
  'data_prep/unbalance_psi': 'UNBALANCE_PSI',
  'data_prep/unbalance_psi_cache': 'UNBALANCE_PSI_CACHE',
  'preprocessing/sql_processor': 'SQL_PROCESSOR',
};

/** 渲染器种类。 */
export type RendererKind =
  | 'union'
  | 'struct'
  | 'custom'
  | 'sql'
  | 'taskInitiator'
  | 'unionKeySelect'
  | 'unbalanceColSelect'
  | 'unbalanceReceiver'
  | 'joinNodeSelect'
  | 'nodeSelectSingle'
  | 'tableSelect'
  | 'modelSelect'
  | 'select'
  | 'featureSelect'
  | 'switch'
  | 'number'
  | 'input'
  | 'list'
  | 'nodeSelect'
  | 'quantiles'
  | 'hidden'
  | 'unspecified';

export interface RenderContext {
  codeName?: string;
  path: string;
  name: string;
  typeName: AttrTypeName;
  hasAllowedValues: boolean;
  customProtobufCls?: string;
}

interface RenderRule {
  kind: RendererKind;
  canHandle: (ctx: RenderContext, renderKey?: string) => number | false;
}

/**
 * 规则表：按旧版 DefaultConfigRender.registerConfigRenders 的顺序与优先级（分数越高越优先，
 * 同分取先注册）逐条翻译。
 */
const RENDER_RULES: RenderRule[] = [
  { kind: 'union', canHandle: (c) => (c.typeName === 'AT_UNION_GROUP' ? 1 : false) },
  { kind: 'struct', canHandle: (c) => (c.typeName === 'AT_STRUCT_GROUP' ? 1 : false) },
  { kind: 'custom', canHandle: (c) => (c.typeName === 'AT_CUSTOM_PROTOBUF' && c.customProtobufCls ? 1 : false) },
  { kind: 'sql', canHandle: (c, k) => (c.typeName === 'AT_STRING' && c.name === 'script_input' && k === 'SQL_ANALYSIS' ? 1 : false) },
  { kind: 'taskInitiator', canHandle: (c, k) => (c.typeName === 'AT_PARTY' && c.name === 'task_initiator' && k === 'SQL_ANALYSIS' ? 1 : false) },
  { kind: 'unionKeySelect', canHandle: (c, k) => (k === 'UNION_KEY_SELECT' && isColumnType(c.typeName) ? 3 : false) },
  { kind: 'unbalanceColSelect', canHandle: (c, k) => (k === 'UNBALANCE_PSI' && isColumnType(c.typeName) ? 1 : false) },
  { kind: 'unbalanceReceiver', canHandle: (c, k) => (k === 'UNBALANCE_PSI' && c.typeName === 'AT_PARTY' ? 1 : false) },
  { kind: 'joinNodeSelect', canHandle: (c, k) => (c.typeName === 'AT_PARTY' && k === 'UNBALANCE_PSI_CACHE' ? 1 : false) },
  { kind: 'nodeSelectSingle', canHandle: (c) => (c.name === 'receiver' ? 3 : false) },
  { kind: 'tableSelect', canHandle: (c, k) => (k === 'DATA_TABLE_SELECT' ? 3 : c.typeName === 'AT_SF_TABLE' ? 1 : false) },
  { kind: 'modelSelect', canHandle: (c, k) => (k === 'MODEL_SELECT' ? 3 : c.typeName === 'AT_MODEL' ? 1 : false) },
  { kind: 'select', canHandle: (c) => (c.hasAllowedValues ? 2 : false) },
  { kind: 'featureSelect', canHandle: (c) => (isColumnType(c.typeName) ? 1 : false) },
  { kind: 'sql', canHandle: (c, k) => (c.typeName === 'AT_STRING' && c.name === 'sql' && (k === 'SQL_PROCESSOR' || k === 'SQL') ? 1 : false) },
  { kind: 'switch', canHandle: (c) => (c.typeName === 'AT_BOOL' ? 1 : false) },
  { kind: 'number', canHandle: (c) => (c.typeName === 'AT_INT' || c.typeName === 'AT_FLOAT' ? 1 : false) },
  { kind: 'input', canHandle: (c) => (c.typeName === 'AT_STRING' ? 1 : false) },
  { kind: 'nodeSelect', canHandle: (c) => (c.typeName === 'AT_PARTY' ? 1 : false) },
  { kind: 'quantiles', canHandle: (c, k) => (c.typeName === 'AT_FLOATS' && c.name === 'quantiles' && k === 'SAMPLE' ? 1 : false) },
  { kind: 'hidden', canHandle: (c, k) => (c.typeName === 'AT_BOOLS' && c.name === 'replacements' && k === 'SAMPLE' ? 1 : false) },
  { kind: 'hidden', canHandle: (c, k) => (c.typeName === 'AT_FLOATS' && c.name === 'weights' && k === 'SAMPLE' ? 1 : false) },
  {
    kind: 'list',
    canHandle: (c) =>
      c.typeName === 'AT_STRINGS' || c.typeName === 'AT_INTS' || c.typeName === 'AT_FLOATS' || c.typeName === 'AT_BOOLS' ? 1 : false,
  },
  { kind: 'unspecified', canHandle: (c) => (c.typeName === 'ATTR_TYPE_UNSPECIFIED' ? 1 : false) },
];

function isColumnType(t: AttrTypeName) {
  return t === 'AT_SF_TABLE_COL' || t === 'AT_COL_PARAMS';
}

/**
 * 显式覆盖表，key 为 `codeName:attrPath`（优先级高于规则表）。宿主可通过
 * `registerRenderer` 追加。
 */
const RENDERER_OVERRIDES = new Map<string, RendererKind>();

export function registerRenderer(codeName: string, attrPath: string, kind: RendererKind) {
  RENDERER_OVERRIDES.set(`${codeName}:${attrPath}`, kind);
}

export function resolveRenderer(ctx: RenderContext): RendererKind {
  const override = ctx.codeName ? RENDERER_OVERRIDES.get(`${ctx.codeName}:${ctx.path}`) : undefined;
  if (override) return override;
  const renderKey = ctx.codeName ? CODE_NAME_RENDER_KEY[ctx.codeName] : undefined;
  let best: RendererKind = ctx.typeName === 'AT_CUSTOM_PROTOBUF' ? 'custom' : 'input';
  let bestScore = 0;
  for (const rule of RENDER_RULES) {
    const score = rule.canHandle(ctx, renderKey);
    if (score !== false && score > bestScore) {
      best = rule.kind;
      bestScore = score;
    }
  }
  return best;
}

/* -------------------------------------------------------------------------- */
/* 面板样式注册表                                                               */
/* -------------------------------------------------------------------------- */

export interface AttrPanelStyle {
  /** 是否展示，默认 true。隐藏的属性不渲染、不序列化（与旧版一致）。 */
  isShow?: boolean;
  /** 顺序（基于隐藏后的列表）。 */
  order?: number;
  style?: { noWrap?: boolean };
  /** 是否折叠在“高级配置”中。 */
  isAdvancedConfig?: boolean;
}

export type ComputeMode = 'MPC' | 'TEE';

type PanelStyleMap = Record<string, { attrs: Record<string, AttrPanelStyle> }>;

const PANEL_STYLES: Record<ComputeMode, PanelStyleMap> = {
  MPC: {
    'data_filter/sample': {
      attrs: {
        sample_algorithm: { order: 0 },
        frac: { order: 1, style: { noWrap: true } },
        'sample_algorithm/random/replacement': { order: 2 },
        'sample_algorithm/random/random_state': { order: 3, isAdvancedConfig: true },
        'sample_algorithm/stratify/observe_feature': { order: 2 },
        'sample_algorithm/stratify/quantiles': { order: 3 },
        'sample_algorithm/stratify/random_state': { order: 4, isAdvancedConfig: true },
      },
    },
    'data_prep/psi': {
      attrs: {
        'allow_duplicate_keys/no/check_hash_digest': { isShow: false },
        allow_duplicate_keys: { order: 3 },
        sort_result: { isAdvancedConfig: true },
        fill_value_int: { isAdvancedConfig: true },
        ecdh_curve: { isAdvancedConfig: true },
      },
    },
    'postprocessing/score_card_transformer': {
      attrs: {
        scaled_value: { style: { noWrap: true } },
        odd_base: { style: { noWrap: true } },
        pdo: { style: { noWrap: true } },
        min_score: { isAdvancedConfig: true, style: { noWrap: true } },
        max_score: { isAdvancedConfig: true, style: { noWrap: true } },
      },
    },
    'read_data/datatable': {
      attrs: { datatable_partition: { isShow: false } },
    },
    'preprocessing/binary_op': {
      attrs: { 'input/in_ds/f2': { order: 2 } },
    },
  },
  TEE: {},
};

export function registerPanelStyle(mode: ComputeMode, codeName: string, attrs: Record<string, AttrPanelStyle>) {
  const prev = PANEL_STYLES[mode][codeName]?.attrs ?? {};
  PANEL_STYLES[mode][codeName] = { attrs: { ...prev, ...attrs } };
}

export function getPanelStyle(codeName: string | undefined, path: string, mode: ComputeMode = 'MPC'): AttrPanelStyle | undefined {
  if (!codeName) return undefined;
  return PANEL_STYLES[mode]?.[codeName]?.attrs?.[path];
}

/**
 * 按样式表对同级属性排序并拆分为普通 / 高级两组（对应旧版 config-form-view 中的重排逻辑）。
 */
export function arrangeByPanelStyle<T extends { path: string }>(
  items: T[],
  codeName: string | undefined,
  mode: ComputeMode = 'MPC',
): { common: T[]; advanced: T[] } {
  const visible = items.filter((it) => getPanelStyle(codeName, it.path, mode)?.isShow !== false);
  const ordered = [...visible];
  for (const it of visible) {
    const order = getPanelStyle(codeName, it.path, mode)?.order;
    if (order === undefined) continue;
    const from = ordered.indexOf(it);
    ordered.splice(from, 1);
    ordered.splice(Math.min(order, ordered.length), 0, it);
  }
  return {
    common: ordered.filter((it) => !getPanelStyle(codeName, it.path, mode)?.isAdvancedConfig),
    advanced: ordered.filter((it) => !!getPanelStyle(codeName, it.path, mode)?.isAdvancedConfig),
  };
}

/* -------------------------------------------------------------------------- */
/* 校验                                                                         */
/* -------------------------------------------------------------------------- */

export type ValidationErrorCode =
  | 'REQUIRED'
  | 'MIN'
  | 'MAX'
  | 'NOT_INTEGER'
  | 'NOT_NUMBER'
  | 'NOT_ALLOWED'
  | 'LIST_MIN'
  | 'LIST_MAX'
  | 'COL_MIN'
  | 'COL_MAX';

export interface ValidationError {
  path: string;
  code: ValidationErrorCode;
  /** 相关阈值（用于文案插值）。 */
  limit?: number | string;
}

export function isNaValue(value: AttributeValue | undefined): boolean {
  if (!value) return true;
  if (value.is_na) return true;
  if (value.custom_protobuf_cls !== undefined || value.custom_value !== undefined) return false;
  const emptyList = (l?: unknown[]) => l === undefined || l.filter((v) => v !== null && v !== undefined && v !== '').length === 0;
  return (
    value.f === undefined &&
    value.i64 === undefined &&
    (value.s === undefined || value.s === '') &&
    value.b === undefined &&
    emptyList(value.fs) &&
    emptyList(value.i64s) &&
    emptyList(value.ss) &&
    emptyList(value.bs)
  );
}

function scalarNumber(v: AttributeValue | undefined): number | undefined {
  if (!v) return undefined;
  if (typeof v.i64 === 'number') return v.i64;
  if (typeof v.f === 'number') return v.f;
  return undefined;
}

function listOf(v: AttributeValue | undefined): unknown[] {
  if (!v) return [];
  return (v.ss ?? v.i64s ?? v.fs ?? v.bs ?? []).filter((x) => x !== null && x !== undefined && x !== '');
}

function allowedList(def: AttributeDef): unknown[] {
  const a = def.atomic?.allowed_values;
  if (!a) return [];
  return a.ss ?? a.i64s ?? a.fs ?? a.bs ?? [];
}

/**
 * 校验单个属性值。
 * - 必填：非 is_optional 且值为空；列属性以 col_min_cnt_inclusive>0 为必填；
 * - 数值：整数校验、上下界（区分 inclusive）；
 * - 列表：长度上下限、每个元素的数值合法性与上下界；
 * - allowed_values：值（或列表元素）必须在候选集中；
 * - 列选择：数量上下限。
 */
export function validateAttr(def: AttributeDef, value: AttributeValue | undefined): ValidationError | null {
  const path = attrPathOf(def);
  const typeName = normalizeType(def.type);
  if (typeName === 'AT_STRUCT_GROUP' || typeName === 'ATTR_TYPE_UNSPECIFIED') return null;
  const optional = def.atomic?.is_optional === true;
  const empty = isNaValue(value);

  if (typeName === 'AT_SF_TABLE_COL' || typeName === 'AT_COL_PARAMS') {
    const count = listOf(value).length;
    const min = def.col_min_cnt_inclusive ?? 0;
    const max = def.col_max_cnt_inclusive;
    if (count === 0 && (min > 0 || (!optional && def.col_min_cnt_inclusive === undefined))) {
      return { path, code: 'REQUIRED' };
    }
    if (count > 0 && count < min) return { path, code: 'COL_MIN', limit: min };
    if (max !== undefined && max > 0 && count > max) return { path, code: 'COL_MAX', limit: max };
    return null;
  }

  if (empty) {
    return optional || (typeName === 'AT_UNION_GROUP' && !!def.union?.default_selection) ? null : { path, code: 'REQUIRED' };
  }

  const atomic = def.atomic;
  const checkBounds = (n: number): ValidationError | null => {
    if (!atomic) return null;
    const lower = scalarNumber(atomic.lower_bound) ?? atomic.lower_bound?.fs?.[0] ?? atomic.lower_bound?.i64s?.[0];
    const upper = scalarNumber(atomic.upper_bound) ?? atomic.upper_bound?.fs?.[0] ?? atomic.upper_bound?.i64s?.[0];
    if (atomic.lower_bound_enabled && lower !== undefined) {
      if (atomic.lower_bound_inclusive ? n < lower : n <= lower) return { path, code: 'MIN', limit: `${atomic.lower_bound_inclusive ? '≥' : '>'} ${lower}` };
    }
    if (atomic.upper_bound_enabled && upper !== undefined) {
      if (atomic.upper_bound_inclusive ? n > upper : n >= upper) return { path, code: 'MAX', limit: `${atomic.upper_bound_inclusive ? '≤' : '<'} ${upper}` };
    }
    return null;
  };

  const allowed = allowedList(def);

  if (typeName === 'AT_INT' || typeName === 'AT_FLOAT') {
    const n = scalarNumber(value);
    if (n === undefined || Number.isNaN(n)) return { path, code: 'NOT_NUMBER' };
    if (typeName === 'AT_INT' && !Number.isInteger(n)) return { path, code: 'NOT_INTEGER' };
    if (allowed.length > 0 && !allowed.map(Number).includes(n)) return { path, code: 'NOT_ALLOWED' };
    return checkBounds(n);
  }

  if (typeName === 'AT_STRING') {
    if (allowed.length > 0 && !allowed.map(String).includes(value?.s ?? '')) return { path, code: 'NOT_ALLOWED' };
    return null;
  }

  if (typeName === 'AT_INTS' || typeName === 'AT_FLOATS' || typeName === 'AT_STRINGS' || typeName === 'AT_BOOLS' || typeName === 'AT_PARTY') {
    const list = listOf(value);
    const minLen = atomic?.list_min_length_inclusive;
    const maxLen = atomic?.list_max_length_inclusive;
    if (minLen !== undefined && list.length < minLen) return { path, code: 'LIST_MIN', limit: minLen };
    if (maxLen !== undefined && maxLen >= 0 && list.length > maxLen) return { path, code: 'LIST_MAX', limit: maxLen };
    for (const el of list) {
      if (typeName === 'AT_INTS' || typeName === 'AT_FLOATS') {
        const n = Number(el);
        if (Number.isNaN(n)) return { path, code: 'NOT_NUMBER' };
        if (typeName === 'AT_INTS' && !Number.isInteger(n)) return { path, code: 'NOT_INTEGER' };
        const b = checkBounds(n);
        if (b) return b;
      }
      if (allowed.length > 0 && !allowed.map(String).includes(String(el))) return { path, code: 'NOT_ALLOWED' };
    }
    return null;
  }
  return null;
}

/** 把 nodeDef 平行数组转换为 path → value 映射。 */
export function nodeDefToValueMap(nodeDef: Record<string, unknown> | undefined): Record<string, AttributeValue> {
  const out: Record<string, AttributeValue> = {};
  if (!nodeDef) return out;
  const paths = Array.isArray(nodeDef.attrPaths) ? (nodeDef.attrPaths as string[]) : [];
  const attrs = Array.isArray(nodeDef.attrs) ? (nodeDef.attrs as unknown[]) : [];
  paths.forEach((p, i) => {
    const v = normalizeAttrValue(attrs[i]);
    if (p && v) out[p] = v;
  });
  return out;
}

/**
 * 节点配置完成度（对应旧版 graph-request-service.ts#isConfigFinished）：
 * 1. 存在必填项但节点从未配置过（无 attrPaths/attrs）→ 未完成；
 * 2. 已配置的必填项若为 is_na → 未完成。
 */
export function isConfigFinished(nodeDef: Record<string, unknown> | undefined, defs: AttributeDef[]): boolean {
  const required = defs.filter((d) => {
    const t = normalizeType(d.type);
    if (t === 'AT_STRUCT_GROUP' || t === 'AT_UNION_GROUP' || t === 'ATTR_TYPE_UNSPECIFIED') return false;
    if (t === 'AT_SF_TABLE_COL' || t === 'AT_COL_PARAMS') return (d.col_min_cnt_inclusive ?? 0) > 0;
    return d.atomic?.is_optional !== true && t !== 'AT_CUSTOM_PROTOBUF';
  });
  if (required.length === 0) return true;
  const hasConfig = !!nodeDef && Array.isArray(nodeDef.attrPaths) && Array.isArray(nodeDef.attrs);
  if (!hasConfig) return false;
  const values = nodeDefToValueMap(nodeDef);
  const paths = nodeDef!.attrPaths as string[];
  for (const d of required) {
    const p = attrPathOf(d);
    // 不在 attrPaths 中：可能位于未选中的联合组分支里，按旧版语义不判未完成。
    if (!paths.includes(p)) continue;
    if (isNaValue(values[p])) return false;
  }
  return true;
}

/* -------------------------------------------------------------------------- */
/* 组件翻译（component/i18n）                                                    */
/* -------------------------------------------------------------------------- */

export type TranslationMap = Record<string, string>;

/**
 * 解析 component/i18n：
 * - Java 形态：`{ secretflow: { "domain/name:version": { text: 译文 } }, trustedflow: {...} }`；
 * - 旧 Go 形态：扁平 `{ text: 译文 }`。
 */
export function createComponentTranslator(raw: unknown) {
  const nested: Record<string, Record<string, TranslationMap>> = {};
  const flat: TranslationMap = {};
  if (raw && typeof raw === 'object') {
    for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
      if (typeof v === 'string') {
        flat[k] = v;
      } else if (v && typeof v === 'object') {
        const inner = v as Record<string, unknown>;
        const values = Object.values(inner);
        if (values.length > 0 && values.every((x) => x && typeof x === 'object')) {
          nested[k] = inner as Record<string, TranslationMap>;
        } else {
          // `{ "domain/name:version": { text: 译文 } }` 直接出现在顶层。
          nested.__root = nested.__root || {};
          nested.__root[k] = inner as TranslationMap;
        }
      }
    }
  }
  /** 获取组件级翻译表。app: secretflow(MPC) / trustedflow(TEE)。 */
  const forComponent = (codeName?: string, version?: string, mode: ComputeMode = 'MPC'): TranslationMap => {
    if (!codeName) return flat;
    const app = mode === 'TEE' ? 'trustedflow' : 'secretflow';
    const buckets = [nested[app], nested.__root, ...Object.values(nested)].filter(Boolean);
    for (const bucket of buckets) {
      if (version && bucket[`${codeName}:${version}`]) return { ...flat, ...bucket[`${codeName}:${version}`] };
      const key = Object.keys(bucket).find((k) => k === codeName || k.startsWith(`${codeName}:`));
      if (key) return { ...flat, ...bucket[key] };
    }
    return flat;
  };
  const translate = (text: string | undefined, codeName?: string, version?: string, mode?: ComputeMode) => {
    if (!text) return text ?? '';
    const map = forComponent(codeName, version, mode);
    return map[text] ?? text;
  };
  return { flat, forComponent, translate };
}
