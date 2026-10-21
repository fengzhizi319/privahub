/**
 * DAG 组件属性动态表单（对应旧版 component-config/config-form-view.tsx + config-item-render/**）。
 *
 * - 属性树：attrs（prefixes）+ `inputs[].attrs` 展开的 `input/<in>/<col>` 列属性；
 * - 渲染器：按 renderKey 注册表（codeName + 属性路径/类型）选择，镜像旧版 config-render-contribution；
 * - 面板样式：isShow / order / isAdvancedConfig；
 * - 校验：必填、数值上下界（inclusive）、列表长度与元素、列数量；
 * - 序列化：`nodeDef.attrPaths` + `nodeDef.attrs`（与旧前端格式一致，自定义 protobuf 走序列化器注册表）；
 * - 每个输入控件都是独立组件，Hooks 均在顶层调用（修复旧实现中条件 return 之后调用 Hooks 的问题）。
 */
import React, { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react';
import { SqlEditor } from './sql-editor';
import type { AttrTypeName, AttributeDef, AttributeValue, ComputeMode, RendererKind, ValidationError } from './component-config';
import {
  TYPES_MAP,
  arrangeByPanelStyle,
  attrPathOf,
  buildAttributeDefs,
  getPanelStyle,
  isConfigFinished,
  isNaValue,
  normalizeAttrDef,
  normalizeAttrValue,
  normalizeType,
  resolveRenderer,
  validateAttr,
  CODE_NAME_RENDER_KEY,
} from './component-config';
import { DRAWER_EDITED_PROTOBUF_CLS, getCustomSerializer } from './custom-serializers';
import { CustomProtobufField } from './custom-renderers';

export type { AttrTypeName, AttributeDef, AttributeValue } from './component-config';

/* -------------------------------------------------------------------------- */
/* 公共类型                                                                     */
/* -------------------------------------------------------------------------- */

export interface AttributeFormLabels {
  issues?: string;
  validate?: string;
  valid?: string;
  advanced?: string;
  noAttrs?: string;
  optional?: string;
  required?: string;
  none?: string;
  listPlaceholder?: string;
  loading?: string;
  selectPlaceholder?: string;
  searchPlaceholder?: string;
  noMatch?: string;
  add?: string;
  editBinning?: string;
  editModelParams?: string;
}

export interface ColumnOption {
  name: string;
  type?: string;
  /** 所属参与方 / 表。 */
  party?: string;
}

export interface OptionItem {
  id: string;
  name: string;
}

/**
 * 复合类型数据提供器：宿主提供列 / 表 / 模型 / 参与方等候选数据。
 */
export interface AttrDataProvider {
  /** 列候选：来自上游输出表 schema；inputIndex 为该列属性所属输入序号。 */
  fetchColumns?: (attrPath: string, ctx?: { inputIndex?: number; codeName?: string }) => Promise<Array<string | ColumnOption>>;
  /** 数据表候选（read_data/datatable 等）。 */
  fetchTables?: (attrPath: string) => Promise<OptionItem[]>;
  /** 模型候选（ml.predict/read_model）。 */
  fetchModels?: (attrPath: string) => Promise<OptionItem[]>;
  /** 参与方候选：project 为项目节点，upstream 为上游输出参与方（scql task_initiator）。 */
  fetchParties?: (attrPath: string, ctx?: { source: 'project' | 'upstream' }) => Promise<OptionItem[]>;
  /** 上游输出表特征（FeatureColumnConfig）。 */
  fetchUpstreamFeatures?: () => Promise<Array<{ tableName: string; tableFeatures: string[]; nodeName: string }>>;
}

export interface AttributeFormHandle {
  /** 触发全量校验并展示错误，返回错误列表。 */
  validate: () => ValidationError[];
}

export interface AttributeFormChangeMeta {
  isFinished: boolean;
  errors: ValidationError[];
}

export interface AttributeFormProps {
  /** 组件属性定义（ComponentDef.attrs，原始或规范化均可）。 */
  defs?: Array<Record<string, unknown>> | AttributeDef[];
  /** 组件输入定义（用于展开 inputs[].attrs 列属性）。 */
  inputs?: Array<Record<string, unknown>>;
  nodeDef: Record<string, unknown> | undefined;
  codeName?: string;
  computeMode?: ComputeMode;
  readOnly?: boolean;
  onNodeDefChange?: (nodeDef: Record<string, unknown>, meta: AttributeFormChangeMeta) => void;
  labels?: AttributeFormLabels;
  dataProvider?: AttrDataProvider;
  /** 属性名 / 描述翻译（component/i18n）。 */
  translate?: (text: string) => string;
  /** 错误文案格式化。 */
  formatError?: (err: ValidationError) => string;
  /** 打开分箱 / 线性模型参数修改面板。 */
  onOpenModification?: (cls: string) => void;
}

/* -------------------------------------------------------------------------- */
/* 属性树                                                                       */
/* -------------------------------------------------------------------------- */

export interface AttrTreeNode {
  def: AttributeDef;
  path: string;
  typeName: AttrTypeName;
  children: AttrTreeNode[];
}

/** 将扁平定义还原为森林（按 prefixes 连边；父节点缺失时作为根）。 */
export function buildAttrTree(defs: AttributeDef[]): AttrTreeNode[] {
  const nodes = new Map<string, AttrTreeNode>();
  const order: AttrTreeNode[] = [];
  for (const def of defs) {
    const path = attrPathOf(def);
    if (!path) continue;
    const node = { def, path, typeName: normalizeType(def.type), children: [] };
    nodes.set(path, node);
    order.push(node);
  }
  const roots: AttrTreeNode[] = [];
  for (const node of order) {
    if (nodes.get(node.path) !== node) continue;
    const parentPath = (node.def.prefixes ?? []).join('/');
    if (parentPath && nodes.has(parentPath)) nodes.get(parentPath)!.children.push(node);
    else roots.push(node);
  }
  return roots;
}

/** 规范化属性定义并合并 `inputs[].attrs` 展开的列属性（输入列属性在前，与旧版一致）。 */
export function prepareDefs(defs: AttributeFormProps['defs'], inputs?: Array<Record<string, unknown>>): AttributeDef[] {
  const attrs = (Array.isArray(defs) ? defs : []).map((d) => normalizeAttrDef(d as Record<string, unknown>));
  const existing = new Set(attrs.map(attrPathOf));
  const inputDefs = buildAttributeDefs({ inputs }).filter((d) => !existing.has(attrPathOf(d)));
  return [...inputDefs, ...attrs];
}

/* -------------------------------------------------------------------------- */
/* 状态编解码                                                                   */
/* -------------------------------------------------------------------------- */

export interface FormState {
  values: Record<string, AttributeValue>;
  /** 自定义 protobuf 的表单态值（未序列化）。 */
  custom: Record<string, unknown>;
}

function walk(nodes: AttrTreeNode[], fn: (n: AttrTreeNode) => void) {
  for (const n of nodes) {
    fn(n);
    walk(n.children, fn);
  }
}

/** 初始状态：default_value / union default_selection，再用 nodeDef 覆盖；自定义 protobuf 走 unserializer。 */
export function buildInitialState(tree: AttrTreeNode[], nodeDef: Record<string, unknown> | undefined): FormState {
  const values: Record<string, AttributeValue> = {};
  const custom: Record<string, unknown> = {};
  const paths = Array.isArray(nodeDef?.attrPaths) ? (nodeDef!.attrPaths as string[]) : [];
  const attrs = Array.isArray(nodeDef?.attrs) ? (nodeDef!.attrs as unknown[]) : [];
  const saved = new Map<string, unknown>();
  paths.forEach((p, i) => saved.set(p, attrs[i]));
  walk(tree, (node) => {
    const { typeName, path, def } = node;
    if (typeName === 'AT_UNION_GROUP' && def.union?.default_selection) {
      values[path] = { s: def.union.default_selection, is_na: false };
    } else if (typeName === 'AT_CUSTOM_PROTOBUF') {
      const ser = getCustomSerializer(def.custom_protobuf_cls);
      const raw = saved.get(path) as Record<string, unknown> | undefined;
      custom[path] = ser ? ser.unserializer(raw) : raw;
      return;
    } else {
      const dv = def.atomic?.default_value;
      if (dv && !isNaValue(dv)) values[path] = dv;
    }
    if (saved.has(path)) {
      const v = normalizeAttrValue(saved.get(path));
      if (v && !v.is_na) values[path] = v;
      else if (v?.is_na) delete values[path];
    }
  });
  return { values, custom };
}

/**
 * 序列化（对应旧版 onSaveConfig）：
 * - 结构组不产出；联合组产出 `{ s: 选中子项 }` 并只展开选中分支；
 * - 原子属性产出 `{ <typesMap 字段>: 值, is_na }`；ss 类型标量自动包成数组；
 * - 面板样式 isShow=false 的属性不产出；
 * - 自定义 protobuf 通过注册表 serializer；分箱 / 线性模型保留 nodeDef 中已存的值。
 */
export function serializeState(
  tree: AttrTreeNode[],
  state: FormState,
  opts: { codeName?: string; computeMode?: ComputeMode; nodeDef?: Record<string, unknown> } = {},
): { attrPaths: string[]; attrs: Array<Record<string, unknown>> } {
  const out = { attrPaths: [] as string[], attrs: [] as Array<Record<string, unknown>> };
  const savedPaths = Array.isArray(opts.nodeDef?.attrPaths) ? (opts.nodeDef!.attrPaths as string[]) : [];
  const savedAttrs = Array.isArray(opts.nodeDef?.attrs) ? (opts.nodeDef!.attrs as Array<Record<string, unknown>>) : [];
  const visit = (nodes: AttrTreeNode[]) => {
    for (const node of nodes) {
      const { typeName, path, children, def } = node;
      if (getPanelStyle(opts.codeName, path, opts.computeMode)?.isShow === false) continue;
      if (typeName === 'AT_STRUCT_GROUP') {
        visit(children);
        continue;
      }
      if (typeName === 'ATTR_TYPE_UNSPECIFIED') {
        visit(children);
        continue;
      }
      if (typeName === 'AT_UNION_GROUP') {
        const sel = state.values[path];
        const selected = sel && !sel.is_na ? sel.s ?? '' : '';
        out.attrPaths.push(path);
        out.attrs.push(selected ? { s: selected, is_na: false } : { is_na: true });
        const child = children.find((c) => c.def.name === selected);
        if (child) visit([child]);
        continue;
      }
      if (typeName === 'AT_CUSTOM_PROTOBUF') {
        const cls = def.custom_protobuf_cls ?? '';
        if (DRAWER_EDITED_PROTOBUF_CLS.includes(cls)) {
          const idx = savedPaths.indexOf(path);
          if (idx >= 0 && savedAttrs[idx]) {
            out.attrPaths.push(path);
            out.attrs.push(savedAttrs[idx]);
          }
          continue;
        }
        const ser = getCustomSerializer(cls);
        const formVal = state.custom[path];
        if (ser && formVal !== undefined) {
          out.attrPaths.push(path);
          out.attrs.push(ser.serializer(formVal, cls));
        }
        continue;
      }
      const key = TYPES_MAP[typeName];
      const value = state.values[path];
      out.attrPaths.push(path);
      if (!key) {
        out.attrs.push(value ? { ...value } : { is_na: true });
        continue;
      }
      let v: unknown = value ? (value as Record<string, unknown>)[key] : undefined;
      // 旧版：ss 类型的标量值（单选）自动包成数组。
      if (key === 'ss' && v === undefined && value?.s !== undefined && value.s !== '') v = value.s;
      if (key === 'ss' && v !== undefined && !Array.isArray(v)) v = [v];
      const na = v === undefined || v === null || (Array.isArray(v) && v.length === 0) || (key === 's' && v === '');
      const entry: Record<string, unknown> = { is_na: na };
      if (v !== undefined) entry[key] = v;
      else if (key === 'ss' || key === 'fs' || key === 'i64s' || key === 'bs') entry[key] = [];
      out.attrs.push(entry);
    }
  };
  visit(tree);
  return out;
}

/** 收集当前可见（选中分支内）原子属性的校验错误。 */
export function collectErrors(tree: AttrTreeNode[], state: FormState, opts: { codeName?: string; computeMode?: ComputeMode } = {}): ValidationError[] {
  const errors: ValidationError[] = [];
  const visit = (nodes: AttrTreeNode[]) => {
    for (const node of nodes) {
      const { typeName, path, children, def } = node;
      if (getPanelStyle(opts.codeName, path, opts.computeMode)?.isShow === false) continue;
      if (typeName === 'AT_STRUCT_GROUP' || typeName === 'ATTR_TYPE_UNSPECIFIED') {
        visit(children);
        continue;
      }
      if (typeName === 'AT_UNION_GROUP') {
        const sel = state.values[path];
        const selected = sel && !sel.is_na ? sel.s : '';
        if (!selected && def.atomic?.is_optional !== true) errors.push({ path, code: 'REQUIRED' });
        const child = children.find((c) => c.def.name === selected);
        if (child) visit([child]);
        continue;
      }
      if (typeName === 'AT_CUSTOM_PROTOBUF') continue;
      const kind = resolveRenderer({
        codeName: opts.codeName,
        path,
        name: def.name ?? '',
        typeName,
        hasAllowedValues: !!def.atomic?.allowed_values && !isNaValue(def.atomic.allowed_values),
      });
      if (kind === 'hidden') continue;
      const err = validateAttr(def, state.values[path]);
      if (err) errors.push(err);
    }
  };
  visit(tree);
  return errors;
}

/* -------------------------------------------------------------------------- */
/* 值工具                                                                       */
/* -------------------------------------------------------------------------- */

function valueToList(value: AttributeValue | undefined): string[] {
  if (!value) return [];
  if (value.ss) return value.ss.filter((v) => v !== null && v !== undefined).map(String);
  if (value.i64s) return value.i64s.map(String);
  if (value.fs) return value.fs.map(String);
  if (value.bs) return value.bs.map(String);
  if (value.s !== undefined && value.s !== '') return [value.s];
  return [];
}

function listToValue(typeName: AttrTypeName, list: string[]): AttributeValue {
  if (list.length === 0) return { is_na: true };
  switch (typeName) {
    case 'AT_INTS':
      return { i64s: list.map((v) => Number(v)), is_na: false };
    case 'AT_FLOATS':
      return { fs: list.map((v) => Number(v)), is_na: false };
    case 'AT_BOOLS':
      return { bs: list.map((v) => v === 'true' || v === '1'), is_na: false };
    default:
      return { ss: list, is_na: false };
  }
}

function scalarToString(value: AttributeValue | undefined): string {
  if (!value || value.is_na) return '';
  if (value.s !== undefined) return value.s;
  if (value.i64 !== undefined) return String(value.i64);
  if (value.f !== undefined) return String(value.f);
  if (value.b !== undefined) return String(value.b);
  if (value.ss && value.ss.length > 0) return String(value.ss[0] ?? '');
  return '';
}

const DEFAULT_ERROR_TEXT: Record<ValidationError['code'], string> = {
  REQUIRED: '该项为必填项',
  MIN: '取值需 {limit}',
  MAX: '取值需 {limit}',
  NOT_INTEGER: '请输入整数',
  NOT_NUMBER: '请输入数字',
  NOT_ALLOWED: '取值不在允许范围内',
  LIST_MIN: '至少填写 {limit} 项',
  LIST_MAX: '最多填写 {limit} 项',
  COL_MIN: '至少选择 {limit} 列',
  COL_MAX: '最多选择 {limit} 列',
};

export function defaultFormatError(err: ValidationError): string {
  return DEFAULT_ERROR_TEXT[err.code].replace('{limit}', String(err.limit ?? ''));
}

const inputCls =
  'w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] focus:outline-none focus:border-blue-500 disabled:opacity-50';

/* -------------------------------------------------------------------------- */
/* 字段组件（每个组件内部 Hooks 均无条件调用）                                     */
/* -------------------------------------------------------------------------- */

interface FieldProps {
  node: AttrTreeNode;
  value: AttributeValue | undefined;
  readOnly?: boolean;
  labels: AttributeFormLabels;
  dataProvider?: AttrDataProvider;
  codeName?: string;
  onChange: (value: AttributeValue) => void;
}

/** 通用异步加载 Hook。 */
function useAsyncOptions<T>(loader: (() => Promise<T[]>) | undefined, deps: unknown[]): { options: T[]; loading: boolean } {
  const [options, setOptions] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const loaderRef = useRef(loader);
  useEffect(() => {
    loaderRef.current = loader;
  });
  useEffect(() => {
    const fn = loaderRef.current;
    if (!fn) return;
    let cancelled = false;
    setLoading(true);
    fn()
      .then((list) => {
        if (!cancelled) setOptions(Array.isArray(list) ? list : []);
      })
      .catch(() => {
        if (!cancelled) setOptions([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  return { options, loading };
}

/** 多选标签控件。 */
export const MultiSelect: React.FC<{
  options: Array<{ value: string; label: string; hint?: string }>;
  selected: string[];
  readOnly?: boolean;
  placeholder?: string;
  searchPlaceholder?: string;
  noMatch?: string;
  max?: number;
  onChange: (list: string[]) => void;
}> = ({ options, selected, readOnly, placeholder, searchPlaceholder, noMatch, max, onChange }) => {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const filtered = options.filter((o) => o.label.toLowerCase().includes(search.toLowerCase()));
  return (
    <div className="relative">
      <div
        role="listbox"
        aria-multiselectable
        className="w-full min-h-[28px] p-1 rounded bg-gray-900 border border-gray-700 text-[11px] flex flex-wrap gap-1 cursor-pointer"
        onClick={() => !readOnly && setOpen((o) => !o)}
      >
        {selected.length === 0 && <span className="text-gray-600">{placeholder ?? '点击选择...'}</span>}
        {selected.map((item) => (
          <span key={item} className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-blue-900/40 border border-blue-700 text-blue-300">
            {options.find((o) => o.value === item)?.label ?? item}
            {!readOnly && (
              <button
                type="button"
                aria-label={`remove ${item}`}
                className="text-blue-400 hover:text-red-400 ml-0.5"
                onClick={(e) => {
                  e.stopPropagation();
                  onChange(selected.filter((s) => s !== item));
                }}
              >
                ×
              </button>
            )}
          </span>
        ))}
      </div>
      {open && !readOnly && (
        <div className="absolute z-50 mt-1 w-full max-h-48 overflow-auto rounded border border-gray-700 bg-gray-900 shadow-xl">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={searchPlaceholder ?? '搜索...'}
            className="w-full px-2 py-1 text-[10px] bg-gray-800 border-b border-gray-700 text-gray-200 focus:outline-none"
            autoFocus
          />
          {filtered.length === 0 && <div className="px-2 py-1.5 text-[10px] text-gray-600">{noMatch ?? '无匹配项'}</div>}
          {filtered.map((opt) => {
            const checked = selected.includes(opt.value);
            const disabled = !checked && max !== undefined && max > 0 && selected.length >= max;
            return (
              <label key={opt.value} className={`flex items-center gap-2 px-2 py-1 hover:bg-gray-800 text-[10px] text-gray-300 ${disabled ? 'opacity-40' : 'cursor-pointer'}`}>
                <input
                  type="checkbox"
                  checked={checked}
                  disabled={disabled}
                  onChange={() => onChange(checked ? selected.filter((s) => s !== opt.value) : [...selected, opt.value])}
                  className="w-3 h-3"
                />
                <span className="flex-1 truncate">{opt.label}</span>
                {opt.hint && <span className="text-gray-500">{opt.hint}</span>}
              </label>
            );
          })}
        </div>
      )}
    </div>
  );
};

const SingleSelect: React.FC<{
  options: OptionItem[];
  value: string;
  readOnly?: boolean;
  loading?: boolean;
  placeholder?: string;
  allowEmpty?: boolean;
  onChange: (id: string) => void;
}> = ({ options, value, readOnly, loading, placeholder, allowEmpty = true, onChange }) => (
  <select value={value} disabled={readOnly || loading} onChange={(e) => onChange(e.target.value)} className={inputCls}>
    {(allowEmpty || !value) && <option value="">{loading ? '加载中...' : placeholder ?? '请选择...'}</option>}
    {value && !options.some((o) => o.id === value) && <option value={value}>{value}</option>}
    {options.map((o) => (
      <option key={o.id} value={o.id}>
        {o.name}
      </option>
    ))}
  </select>
);

/** 列选择（AT_SF_TABLE_COL / AT_COL_PARAMS）：候选来自上游输出表 schema，受 col_min/max 约束。 */
const ColumnField: React.FC<FieldProps> = ({ node, value, readOnly, labels, dataProvider, codeName, onChange }) => {
  const { path, def } = node;
  const { options, loading } = useAsyncOptions<string | ColumnOption>(
    dataProvider?.fetchColumns ? () => dataProvider.fetchColumns!(path, { inputIndex: def.fromInputIndex, codeName }) : undefined,
    [path, def.fromInputIndex, codeName, dataProvider],
  );
  const opts = options.map((o) => (typeof o === 'string' ? { value: o, label: o } : { value: o.name, label: o.name, hint: [o.type, o.party].filter(Boolean).join(' · ') }));
  const selected = valueToList(value);
  const max = def.col_max_cnt_inclusive;
  if (max === 1) {
    return (
      <SingleSelect
        options={opts.map((o) => ({ id: o.value, name: o.hint ? `${o.label} (${o.hint})` : o.label }))}
        value={selected[0] ?? ''}
        readOnly={readOnly}
        loading={loading}
        placeholder={labels.selectPlaceholder}
        onChange={(v) => onChange(v ? { ss: [v], is_na: false } : { is_na: true })}
      />
    );
  }
  return (
    <div className="space-y-1">
      {loading && <div className="text-[9px] text-gray-600">{labels.loading ?? '加载列信息...'}</div>}
      <MultiSelect
        options={opts}
        selected={selected}
        readOnly={readOnly}
        max={max}
        placeholder={labels.selectPlaceholder}
        searchPlaceholder={labels.searchPlaceholder}
        noMatch={labels.noMatch}
        onChange={(list) => onChange(listToValue('AT_STRINGS', list))}
      />
    </div>
  );
};

const TableField: React.FC<FieldProps> = ({ node, value, readOnly, labels, dataProvider, onChange }) => {
  const { options, loading } = useAsyncOptions<OptionItem>(dataProvider?.fetchTables ? () => dataProvider.fetchTables!(node.path) : undefined, [node.path, dataProvider]);
  return (
    <SingleSelect
      options={options}
      value={scalarToString(value)}
      readOnly={readOnly}
      loading={loading}
      placeholder={labels.selectPlaceholder}
      onChange={(id) => onChange(id ? { s: id, is_na: false } : { is_na: true })}
    />
  );
};

const ModelField: React.FC<FieldProps> = ({ node, value, readOnly, labels, dataProvider, onChange }) => {
  const { options, loading } = useAsyncOptions<OptionItem>(dataProvider?.fetchModels ? () => dataProvider.fetchModels!(node.path) : undefined, [node.path, dataProvider]);
  return (
    <SingleSelect
      options={options}
      value={scalarToString(value)}
      readOnly={readOnly}
      loading={loading}
      placeholder={labels.selectPlaceholder}
      onChange={(id) => onChange(id ? { s: id, is_na: false } : { is_na: true })}
    />
  );
};

/** 参与方选择：多选（AT_PARTY）或单选（receiver / task_initiator / 非平衡 PSI）。 */
const PartyField: React.FC<FieldProps & { single?: boolean; source?: 'project' | 'upstream' }> = ({
  node,
  value,
  readOnly,
  labels,
  dataProvider,
  single,
  source = 'project',
  onChange,
}) => {
  const { options, loading } = useAsyncOptions<OptionItem>(
    dataProvider?.fetchParties ? () => dataProvider.fetchParties!(node.path, { source }) : undefined,
    [node.path, source, dataProvider],
  );
  const selected = valueToList(value);
  const listMax = node.def.atomic?.list_max_length_inclusive;
  if (single || listMax === 1) {
    return (
      <SingleSelect
        options={options}
        value={selected[0] ?? ''}
        readOnly={readOnly}
        loading={loading}
        placeholder={labels.selectPlaceholder}
        onChange={(id) => onChange(id ? { ss: [id], is_na: false } : { is_na: true })}
      />
    );
  }
  return (
    <MultiSelect
      options={options.map((o) => ({ value: o.id, label: o.name || o.id }))}
      selected={selected}
      readOnly={readOnly}
      max={listMax && listMax > 0 ? listMax : undefined}
      placeholder={labels.selectPlaceholder}
      searchPlaceholder={labels.searchPlaceholder}
      noMatch={labels.noMatch}
      onChange={(list) => onChange(listToValue('AT_PARTY', list))}
    />
  );
};

/** allowed_values 下拉：标量单选，列表类型多选。 */
const SelectField: React.FC<FieldProps> = ({ node, value, readOnly, labels, onChange }) => {
  const { typeName, def } = node;
  const allowed = valueToList(def.atomic?.allowed_values);
  const isList = ['AT_STRINGS', 'AT_INTS', 'AT_FLOATS', 'AT_BOOLS', 'AT_PARTY'].includes(typeName);
  if (isList) {
    return (
      <MultiSelect
        options={allowed.map((a) => ({ value: a, label: a }))}
        selected={valueToList(value)}
        readOnly={readOnly}
        max={def.atomic?.list_max_length_inclusive}
        placeholder={labels.selectPlaceholder}
        onChange={(list) => onChange(listToValue(typeName, list))}
      />
    );
  }
  const current = scalarToString(value);
  return (
    <select
      value={current}
      disabled={readOnly}
      onChange={(e) => {
        const raw = e.target.value;
        if (raw === '') return onChange({ is_na: true });
        if (typeName === 'AT_INT') onChange({ i64: Number(raw), is_na: false });
        else if (typeName === 'AT_FLOAT') onChange({ f: Number(raw), is_na: false });
        else onChange({ s: raw, is_na: false });
      }}
      className={inputCls}
    >
      {(def.atomic?.is_optional || !current) && <option value="">{labels.none ?? '（不设置）'}</option>}
      {allowed.map((opt) => (
        <option key={opt} value={opt}>
          {opt}
        </option>
      ))}
    </select>
  );
};

const NumberField: React.FC<FieldProps> = ({ node, value, readOnly, onChange }) => {
  const { typeName, def } = node;
  const a = def.atomic;
  const lower = a?.lower_bound_enabled ? a.lower_bound?.i64 ?? a.lower_bound?.f : undefined;
  const upper = a?.upper_bound_enabled ? a.upper_bound?.i64 ?? a.upper_bound?.f : undefined;
  // 本地保留原始输入文本，避免输入 "0." 之类中间态被吞掉。
  const [text, setText] = useState(scalarToString(value));
  const external = scalarToString(value);
  const lastExternal = useRef(external);
  useEffect(() => {
    if (external !== lastExternal.current) {
      lastExternal.current = external;
      setText(external);
    }
  }, [external]);
  return (
    <input
      type="number"
      value={text}
      disabled={readOnly}
      min={lower}
      max={upper}
      step={typeName === 'AT_INT' ? 1 : 'any'}
      onChange={(e) => {
        const raw = e.target.value;
        setText(raw);
        if (raw === '') return onChange({ is_na: true });
        const n = Number(raw);
        if (Number.isNaN(n)) return;
        lastExternal.current = raw;
        onChange(typeName === 'AT_INT' ? { i64: n, is_na: false } : { f: n, is_na: false });
      }}
      className={`${inputCls} font-mono`}
    />
  );
};

const SwitchField: React.FC<FieldProps> = ({ value, readOnly, onChange }) => {
  const checked = value?.b === true;
  return (
    <label className="inline-flex items-center gap-2 cursor-pointer select-none">
      <input type="checkbox" checked={checked} disabled={readOnly} onChange={(e) => onChange({ b: e.target.checked, is_na: false })} className="w-3.5 h-3.5" />
      <span className="text-gray-300">{checked ? 'true' : 'false'}</span>
    </label>
  );
};

const TextField: React.FC<FieldProps> = ({ value, readOnly, onChange }) => (
  <input
    type="text"
    value={scalarToString(value)}
    disabled={readOnly}
    onChange={(e) => onChange(e.target.value ? { s: e.target.value, is_na: false } : { is_na: true })}
    className={`${inputCls} font-mono`}
  />
);

/** 列表（AT_STRINGS/INTS/FLOATS/BOOLS）：标签式输入，回车 / 逗号添加。 */
const ListField: React.FC<FieldProps> = ({ node, value, readOnly, labels, onChange }) => {
  const { typeName } = node;
  const [draft, setDraft] = useState('');
  const list = valueToList(value);
  const commit = () => {
    const parts = draft
      .split(',')
      .map((v) => v.trim())
      .filter(Boolean);
    if (parts.length === 0) return;
    if ((typeName === 'AT_INTS' || typeName === 'AT_FLOATS') && parts.some((p) => Number.isNaN(Number(p)))) return;
    onChange(listToValue(typeName, [...list, ...parts]));
    setDraft('');
  };
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap gap-1">
        {list.map((item, i) => (
          <span key={`${item}-${i}`} className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-gray-800 border border-gray-700 text-[10px] text-gray-300">
            {item}
            {!readOnly && (
              <button type="button" aria-label={`remove ${item}`} className="hover:text-red-400" onClick={() => onChange(listToValue(typeName, list.filter((_, j) => j !== i)))}>
                ×
              </button>
            )}
          </span>
        ))}
      </div>
      {!readOnly &&
        (typeName === 'AT_BOOLS' ? (
          <div className="flex gap-1">
            {['true', 'false'].map((b) => (
              <button key={b} type="button" className="px-2 py-0.5 rounded border border-gray-700 text-[10px] text-gray-300" onClick={() => onChange(listToValue(typeName, [...list, b]))}>
                + {b}
              </button>
            ))}
          </div>
        ) : (
          <input
            type="text"
            value={draft}
            placeholder={labels.listPlaceholder ?? '输入后回车添加，逗号分隔'}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commit();
              }
            }}
            onBlur={commit}
            className={`${inputCls} font-mono`}
          />
        ))}
    </div>
  );
};

const SqlField: React.FC<FieldProps> = ({ value, readOnly, onChange }) => (
  <SqlEditor
    value={scalarToString(value)}
    readOnly={readOnly}
    height={140}
    onChange={(v) => onChange(v ? { s: v, is_na: false } : { is_na: true })}
    labels={{ placeholder: 'SELECT col1, col2 FROM my_table WHERE ...' }}
  />
);

/**
 * 观测值分位点（data_filter/sample stratify）：逗号分隔分位点 + 各区间是否放回 / 权重，
 * 同时写入兄弟属性 `replacements`（bs）与 `weights`（fs）。
 */
const QuantilesField: React.FC<FieldProps & { siblingValue: (name: string) => AttributeValue | undefined; setSibling: (name: string, v: AttributeValue) => void }> = ({
  value,
  readOnly,
  siblingValue,
  setSibling,
  onChange,
}) => {
  const quantiles = value?.fs ?? [];
  const [text, setText] = useState(quantiles.join(','));
  const intervals = quantiles.length + 1;
  const replacements = siblingValue('replacements')?.bs ?? Array.from({ length: intervals }, () => true);
  const weights = siblingValue('weights')?.fs ?? [];
  const valid = /^(-?\d+(\.\d+)?)(,-?\d+(\.\d+)?)*$/.test(text.trim());
  return (
    <div className="space-y-1">
      <input
        type="text"
        value={text}
        disabled={readOnly}
        placeholder="1,2"
        onChange={(e) => {
          setText(e.target.value);
          const t = e.target.value.trim();
          if (/^(-?\d+(\.\d+)?)(,-?\d+(\.\d+)?)*$/.test(t)) {
            const list = t.split(',').map(Number);
            onChange({ fs: list, is_na: false });
            setSibling('replacements', { bs: Array.from({ length: list.length + 1 }, (_, i) => replacements[i] ?? true), is_na: false });
          } else if (t === '') {
            onChange({ is_na: true });
          }
        }}
        className={`${inputCls} font-mono ${!valid && text ? 'border-red-500' : ''}`}
      />
      {quantiles.length > 0 && (
        <table className="w-full text-[10px] text-gray-300">
          <thead>
            <tr className="text-gray-500">
              <th className="text-left">区间</th>
              <th className="text-left">放回采样</th>
              <th className="text-left">权重</th>
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: intervals }).map((_, i) => (
              <tr key={i}>
                <td>{i === 0 ? `(-∞, ${quantiles[0]}]` : i === intervals - 1 ? `(${quantiles[i - 1]}, +∞)` : `(${quantiles[i - 1]}, ${quantiles[i]}]`}</td>
                <td>
                  <input
                    type="checkbox"
                    disabled={readOnly}
                    checked={replacements[i] ?? true}
                    onChange={(e) => {
                      const next = Array.from({ length: intervals }, (_, j) => replacements[j] ?? true);
                      next[i] = e.target.checked;
                      setSibling('replacements', { bs: next, is_na: false });
                    }}
                  />
                </td>
                <td>
                  <input
                    type="number"
                    step="0.01"
                    min={0}
                    max={1}
                    disabled={readOnly}
                    value={weights[i] ?? ''}
                    onChange={(e) => {
                      const next = Array.from({ length: intervals }, (_, j) => weights[j] ?? 0);
                      next[i] = Math.round(Number(e.target.value) * 100) / 100;
                      setSibling('weights', { fs: next, is_na: false });
                    }}
                    className="w-16 bg-gray-900 border border-gray-700 rounded px-1"
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 渲染                                                                         */
/* -------------------------------------------------------------------------- */

interface RenderCtx {
  state: FormState;
  readOnly?: boolean;
  labels: AttributeFormLabels;
  dataProvider?: AttrDataProvider;
  codeName?: string;
  computeMode?: ComputeMode;
  errors: Record<string, string>;
  translate: (t: string) => string;
  setValue: (path: string, v: AttributeValue) => void;
  setCustom: (path: string, v: unknown) => void;
  onOpenModification?: (cls: string) => void;
}

function rendererOf(node: AttrTreeNode, codeName?: string): RendererKind {
  return resolveRenderer({
    codeName,
    path: node.path,
    name: node.def.name ?? '',
    typeName: node.typeName,
    hasAllowedValues: !!node.def.atomic?.allowed_values && !isNaValue(node.def.atomic.allowed_values),
    customProtobufCls: node.def.custom_protobuf_cls,
  });
}

const FieldSwitch: React.FC<{ node: AttrTreeNode; kind: RendererKind; ctx: RenderCtx }> = ({ node, kind, ctx }) => {
  const { path } = node;
  const common: FieldProps = {
    node,
    value: ctx.state.values[path],
    readOnly: ctx.readOnly,
    labels: ctx.labels,
    dataProvider: ctx.dataProvider,
    codeName: ctx.codeName,
    onChange: (v) => ctx.setValue(path, v),
  };
  const parentPrefix = (node.def.prefixes ?? []).join('/');
  const siblingPath = (name: string) => (parentPrefix ? `${parentPrefix}/${name}` : name);
  switch (kind) {
    case 'unionKeySelect':
    case 'unbalanceColSelect':
    case 'featureSelect':
      return <ColumnField {...common} />;
    case 'tableSelect':
      return <TableField {...common} />;
    case 'modelSelect':
      return <ModelField {...common} />;
    case 'nodeSelect':
      return <PartyField {...common} />;
    case 'nodeSelectSingle':
    case 'unbalanceReceiver':
    case 'joinNodeSelect':
      return <PartyField {...common} single />;
    case 'taskInitiator':
      return <PartyField {...common} single source="upstream" />;
    case 'select':
      return <SelectField {...common} />;
    case 'number':
      return <NumberField {...common} />;
    case 'switch':
      return <SwitchField {...common} />;
    case 'sql':
      return <SqlField {...common} />;
    case 'list':
      return <ListField {...common} />;
    case 'quantiles':
      return (
        <QuantilesField
          {...common}
          siblingValue={(name) => ctx.state.values[siblingPath(name)]}
          setSibling={(name, v) => ctx.setValue(siblingPath(name), v)}
        />
      );
    case 'custom':
      return (
        <CustomProtobufField
          cls={node.def.custom_protobuf_cls ?? ''}
          value={ctx.state.custom[path]}
          readOnly={ctx.readOnly}
          dataProvider={ctx.dataProvider}
          onChange={(v) => ctx.setCustom(path, v)}
          onOpenModification={ctx.onOpenModification}
          labels={ctx.labels}
        />
      );
    default:
      return <TextField {...common} />;
  }
};

const FieldLabel: React.FC<{ node: AttrTreeNode; ctx: RenderCtx; optional: boolean }> = ({ node, ctx, optional }) => {
  const name = ctx.translate(node.def.name ?? node.path);
  const noWrap = getPanelStyle(ctx.codeName, node.path, ctx.computeMode)?.style?.noWrap;
  return (
    <div className={`flex items-center justify-between ${noWrap ? 'inline-flex gap-2' : ''}`}>
      <span className="text-gray-300 text-[11px] font-medium" title={node.def.desc ? ctx.translate(node.def.desc) : undefined}>
        {name}
        <span className={`ml-1.5 text-[9px] ${optional ? 'text-gray-600' : 'text-amber-500'}`}>
          {optional ? ctx.labels.optional ?? '可选' : ctx.labels.required ?? '必填'}
        </span>
      </span>
      <span className="text-gray-600 text-[9px] font-mono truncate max-w-[45%]">{node.path}</span>
    </div>
  );
};

const NodeView: React.FC<{ node: AttrTreeNode; ctx: RenderCtx; depth: number }> = ({ node, ctx, depth }) => {
  const { typeName, path, def, children } = node;
  const kind = rendererOf(node, ctx.codeName);
  const desc = def.desc ? ctx.translate(def.desc) : undefined;
  const error = ctx.errors[path];

  if (typeName === 'AT_STRUCT_GROUP') {
    return (
      <div className="space-y-2">
        <div className="text-gray-400 text-[10px] font-bold uppercase tracking-wide flex items-center gap-1.5">
          <span className="w-1 h-3 bg-blue-500/60 rounded-full" />
          {ctx.translate(def.name ?? path)}
        </div>
        {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
        <NodeList nodes={children} ctx={ctx} depth={depth + 1} />
      </div>
    );
  }

  if (typeName === 'AT_UNION_GROUP') {
    const sel = ctx.state.values[path];
    const selected = sel && !sel.is_na ? sel.s ?? '' : '';
    const selectedChild = children.find((c) => c.def.name === selected);
    return (
      <div className="space-y-2" data-attr-path={path}>
        <FieldLabel node={node} ctx={ctx} optional={false} />
        {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
        <select
          value={selected}
          disabled={ctx.readOnly}
          aria-label={def.name}
          onChange={(e) => ctx.setValue(path, e.target.value ? { s: e.target.value, is_na: false } : { is_na: true })}
          className={inputCls}
        >
          <option value="">{ctx.labels.none ?? '（不选择）'}</option>
          {children.map((child) => (
            <option key={child.path} value={child.def.name ?? ''}>
              {ctx.translate(child.def.name ?? '')}
            </option>
          ))}
        </select>
        {error && <div className="text-red-400 text-[10px]">{error}</div>}
        {selectedChild && selectedChild.typeName !== 'ATTR_TYPE_UNSPECIFIED' && <NodeList nodes={[selectedChild]} ctx={ctx} depth={depth + 1} />}
        {selectedChild && selectedChild.typeName === 'ATTR_TYPE_UNSPECIFIED' && selectedChild.children.length > 0 && (
          <NodeList nodes={selectedChild.children} ctx={ctx} depth={depth + 1} />
        )}
      </div>
    );
  }

  if (typeName === 'ATTR_TYPE_UNSPECIFIED' || kind === 'hidden') return null;

  const optional =
    typeName === 'AT_SF_TABLE_COL' || typeName === 'AT_COL_PARAMS' ? (def.col_min_cnt_inclusive ?? 0) === 0 : def.atomic?.is_optional === true;
  return (
    <div className="space-y-1" data-attr-path={path}>
      <FieldLabel node={node} ctx={ctx} optional={optional && typeName !== 'AT_CUSTOM_PROTOBUF'} />
      {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
      <FieldSwitch node={node} kind={kind} ctx={ctx} />
      {error && (
        <div role="alert" className="text-red-400 text-[10px]">
          {error}
        </div>
      )}
    </div>
  );
};

const NodeList: React.FC<{ nodes: AttrTreeNode[]; ctx: RenderCtx; depth: number }> = ({ nodes, ctx, depth }) => {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const { common, advanced } = arrangeByPanelStyle(nodes, ctx.codeName, ctx.computeMode);
  return (
    <div className={depth > 0 ? 'pl-3 border-l border-gray-800 space-y-3' : 'space-y-3'}>
      {common.map((n) => (
        <NodeView key={n.path} node={n} ctx={ctx} depth={depth} />
      ))}
      {advanced.length > 0 && (
        <div className="space-y-2">
          <label className="inline-flex items-center gap-2 text-[10px] text-gray-400 cursor-pointer">
            <input type="checkbox" checked={showAdvanced} onChange={(e) => setShowAdvanced(e.target.checked)} />
            {ctx.labels.advanced ?? '高级配置'}
          </label>
          {showAdvanced && advanced.map((n) => <NodeView key={n.path} node={n} ctx={ctx} depth={depth} />)}
        </div>
      )}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 主组件                                                                       */
/* -------------------------------------------------------------------------- */

export const AttributeForm = forwardRef<AttributeFormHandle, AttributeFormProps>(function AttributeForm(
  { defs, inputs, nodeDef, codeName, computeMode = 'MPC', readOnly, onNodeDefChange, labels = {}, dataProvider, translate, formatError = defaultFormatError, onOpenModification },
  ref,
) {
  const normalizedDefs = useMemo(() => prepareDefs(defs, inputs), [defs, inputs]);
  const tree = useMemo(() => buildAttrTree(normalizedDefs), [normalizedDefs]);

  // 最新 nodeDef 仅在定义变化时用于重建初始值，编辑回写不触发重置。
  const nodeDefRef = useRef(nodeDef);
  useEffect(() => {
    nodeDefRef.current = nodeDef;
  });

  const [state, setState] = useState<FormState>(() => buildInitialState(tree, nodeDef));
  const [showErrors, setShowErrors] = useState(false);
  const stateRef = useRef(state);
  useEffect(() => {
    stateRef.current = state;
  });

  useEffect(() => {
    setState(buildInitialState(tree, nodeDefRef.current));
    setShowErrors(false);
  }, [tree]);

  const errorsList = useMemo(() => collectErrors(tree, state, { codeName, computeMode }), [tree, state, codeName, computeMode]);
  const errors = useMemo(() => {
    const map: Record<string, string> = {};
    if (showErrors) errorsList.forEach((e) => (map[e.path] = formatError(e)));
    return map;
  }, [errorsList, showErrors, formatError]);

  const emit = useCallback(
    (next: FormState) => {
      if (!onNodeDefChange) return;
      const base = nodeDefRef.current ?? {};
      const serialized = serializeState(tree, next, { codeName, computeMode, nodeDef: base });
      const nextNodeDef = { ...base, attrPaths: serialized.attrPaths, attrs: serialized.attrs };
      onNodeDefChange(nextNodeDef, {
        isFinished: isConfigFinished(nextNodeDef, normalizedDefs),
        errors: collectErrors(tree, next, { codeName, computeMode }),
      });
    },
    [onNodeDefChange, tree, codeName, computeMode, normalizedDefs],
  );

  const setValue = useCallback(
    (path: string, v: AttributeValue) => {
      const next = { ...stateRef.current, values: { ...stateRef.current.values, [path]: v } };
      stateRef.current = next;
      setState(next);
      emit(next);
    },
    [emit],
  );

  const setCustom = useCallback(
    (path: string, v: unknown) => {
      const next = { ...stateRef.current, custom: { ...stateRef.current.custom, [path]: v } };
      stateRef.current = next;
      setState(next);
      emit(next);
    },
    [emit],
  );

  useImperativeHandle(
    ref,
    () => ({
      validate: () => {
        setShowErrors(true);
        return collectErrors(tree, stateRef.current, { codeName, computeMode });
      },
    }),
    [tree, codeName, computeMode],
  );

  const translateFn = useCallback((t: string) => (translate ? translate(t) : t), [translate]);

  if (tree.length === 0) {
    return <div className="text-gray-600 text-[10px]">{labels.noAttrs ?? '该组件无可配置属性'}</div>;
  }

  const ctx: RenderCtx = {
    state,
    readOnly,
    labels,
    dataProvider,
    codeName,
    computeMode,
    errors,
    translate: translateFn,
    setValue,
    setCustom,
    onOpenModification,
  };

  return (
    <div className="space-y-1" data-render-key={codeName ? CODE_NAME_RENDER_KEY[codeName] ?? '' : ''}>
      <div className={`sticky top-0 z-10 mb-3 rounded-lg border p-3 text-xs ${errorsList.length ? 'border-amber-500/40 bg-gray-950 text-amber-300' : 'border-emerald-500/30 bg-gray-950 text-emerald-300'}`}>
        <button type="button" className="flex w-full items-center justify-between gap-2 text-left" onClick={()=>setShowErrors(value=>!value)} aria-expanded={showErrors}>
          <span>{errorsList.length ? `${labels.issues ?? 'Configuration issues'} (${errorsList.length})` : labels.valid ?? 'Configuration complete'}</span>
          <span className="text-blue-300">{labels.validate ?? 'Check configuration'}</span>
        </button>
        {showErrors && errorsList.length>0 && <ul className="mt-2 max-h-40 space-y-1 overflow-auto">{errorsList.map((error,index)=><li key={`${error.path}-${index}`}><button type="button" className="text-left underline decoration-dotted underline-offset-2" onClick={event=>{
          const form=event.currentTarget.closest('[data-render-key]');
          const target=[...(form?.querySelectorAll<HTMLElement>('[data-attr-path]')||[])].find(item=>item.dataset.attrPath===error.path);
          target?.scrollIntoView({block:'center'});target?.querySelector<HTMLElement>('input,select,textarea,button')?.focus();
        }}>{translateFn(error.path)}: {formatError(error)}</button></li>)}</ul>}
      </div>
      <NodeList nodes={tree} ctx={ctx} depth={0} />
    </div>
  );
});
