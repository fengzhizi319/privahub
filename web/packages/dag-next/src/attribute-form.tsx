/**
 * DAG 高级配置抽屉 —— SecretFlow 组件属性动态表单。
 *
 * 本模块依据后端 `component/batch` 返回的组件定义（`ComponentDef.attrs`，
 * 对应 secretflow spec `component.proto` 中的 `AttributeDef` 列表），
 * 动态渲染出可视化的参数配置表单，替代原先只能手工编辑 JSON 的方式。
 *
 * 核心能力：
 * - 将扁平的 `AttributeDef[]`（通过 `prefixes` 描述祖先路径）还原为属性树；
 * - 按属性类型（AT_INT / AT_STRING / AT_BOOL / AT_STRINGS / 结构组 / 联合组等）
 *   渲染对应的输入控件；
 * - 联合组（AT_UNION_GROUP）渲染为互斥单选，仅展开被选中的子树；
 * - 编辑结果实时回写为 `nodeDef.attrPaths` + `nodeDef.attrs` 两个平行数组，
 *   与旧前端以及模板构建器（builder.ts）产出的结构完全一致。
 *
 * 设计约束：
 * - 该组件位于 `@secretpad/dag-next` 包内，不依赖应用层 i18n，文案通过
 *   `labels` 属性注入，保持画布包的可复用性。
 */
import React, { useEffect, useMemo, useRef, useState } from 'react';

/* -------------------------------------------------------------------------- */
/* 类型定义（与 secretflow spec component.proto 对齐）                          */
/* -------------------------------------------------------------------------- */

/** 属性类型枚举名（对应 proto 中 AttrType 的字符串序列化形式）。 */
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
  | 'AT_PARTY';

/** proto 枚举值 -> 枚举名 的映射，兼容后端返回数字或字符串两种形式。 */
const ATTR_TYPE_BY_NUMBER: Record<number, AttrTypeName> = {
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
};

/** 属性值（对应 proto 中 Attribute 消息，字段名保持下划线风格以便直接透传）。 */
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
}

/** 原子属性附加描述（对应 AtomicAttrDesc）。 */
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

/** 属性定义（对应 AttributeDef，字段名保持下划线风格）。 */
export interface AttributeDef {
  prefixes?: string[];
  name?: string;
  desc?: string;
  type?: AttrTypeName | number;
  atomic?: AtomicAttrDesc;
  union?: { default_selection?: string };
  custom_protobuf_cls?: string;
}

/** 属性树节点：携带完整路径与子节点。 */
interface AttrTreeNode {
  def: AttributeDef;
  /** 完整属性路径，如 `input/input_ds1/keys`。 */
  path: string;
  /** 规范化后的类型名。 */
  typeName: AttrTypeName;
  children: AttrTreeNode[];
}

/** 表单可识别的文案标签（由宿主应用注入，实现国际化）。 */
export interface AttributeFormLabels {
  /** 分组折叠/展开提示。 */
  advanced?: string;
  /** 无属性时的占位文案。 */
  noAttrs?: string;
  /** 可选属性标记。 */
  optional?: string;
  /** 必填属性标记。 */
  required?: string;
  /** 联合组“未选择”选项。 */
  none?: string;
  /** 列表输入占位（逗号分隔）。 */
  listPlaceholder?: string;
}

/** 组件 Props。 */
export interface AttributeFormProps {
  /** 后端返回的属性定义列表（ComponentDef.attrs）。 */
  defs: Array<Record<string, unknown>> | undefined;
  /** 当前节点定义，提供 attrPaths / attrs 初始值。 */
  nodeDef: Record<string, unknown> | undefined;
  /** 只读模式（P2P 只读视图）。 */
  readOnly?: boolean;
  /** 任一输入变化时回调，参数为更新后的完整 nodeDef。 */
  onNodeDefChange?: (nodeDef: Record<string, unknown>) => void;
  labels?: AttributeFormLabels;
}

/* -------------------------------------------------------------------------- */
/* 工具函数                                                                    */
/* -------------------------------------------------------------------------- */

/** 将后端可能返回的数字或字符串类型统一为枚举名。 */
function normalizeType(type: AttributeDef['type']): AttrTypeName {
  if (typeof type === 'number') {
    return ATTR_TYPE_BY_NUMBER[type] ?? 'ATTR_TYPE_UNSPECIFIED';
  }
  if (typeof type === 'string' && type.length > 0) {
    return type as AttrTypeName;
  }
  return 'ATTR_TYPE_UNSPECIFIED';
}

/** 计算属性完整路径：prefixes 以 `/` 连接后追加 name。 */
function attrPath(def: AttributeDef): string {
  const prefixes = def.prefixes ?? [];
  const name = def.name ?? '';
  return [...prefixes, name].filter(Boolean).join('/');
}

/**
 * 将扁平的 AttributeDef 列表还原为森林。
 *
 * 每个 def 的父节点路径即其 `prefixes.join('/')`；若该路径存在于映射表中则挂到
 * 对应父节点下，否则视为根节点。先整体建点、再统一连边，避免顺序依赖。
 */
export function buildAttrTree(defs: AttributeDef[]): AttrTreeNode[] {
  const nodes = new Map<string, AttrTreeNode>();
  // 第一遍：为每个 def 创建节点。
  for (const def of defs) {
    const path = attrPath(def);
    if (!path) continue;
    nodes.set(path, { def, path, typeName: normalizeType(def.type), children: [] });
  }
  const roots: AttrTreeNode[] = [];
  // 第二遍：根据 prefixes 连边。
  for (const node of nodes.values()) {
    const prefixes = node.def.prefixes ?? [];
    const parentPath = prefixes.join('/');
    if (parentPath && nodes.has(parentPath)) {
      nodes.get(parentPath)!.children.push(node);
    } else {
      roots.push(node);
    }
  }
  return roots;
}

/** 判断一个属性值是否为“空”（is_na 或所有字段缺失）。 */
function isNa(value: AttributeValue | undefined): boolean {
  if (!value) return true;
  if (value.is_na) return true;
  return (
    value.f === undefined &&
    value.i64 === undefined &&
    value.s === undefined &&
    value.b === undefined &&
    (value.fs === undefined || value.fs.length === 0) &&
    (value.i64s === undefined || value.i64s.length === 0) &&
    (value.ss === undefined || value.ss.length === 0) &&
    (value.bs === undefined || value.bs.length === 0)
  );
}

/** 依据属性类型构造一个表示“缺失”的值。 */
function naValue(): AttributeValue {
  return { is_na: true };
}

/** 从 AttributeValue 中取出字符串列表（兼容 ss / i64s / fs / bs）。 */
function valueToList(value: AttributeValue | undefined): string[] {
  if (!value) return [];
  if (value.ss) return value.ss.map(String);
  if (value.i64s) return value.i64s.map(String);
  if (value.fs) return value.fs.map(String);
  if (value.bs) return value.bs.map(String);
  if (value.s !== undefined) return value.s === '' ? [] : [value.s];
  return [];
}

/** 依据属性类型，把表单字符串列表打包回 AttributeValue。 */
function listToValue(typeName: AttrTypeName, list: string[]): AttributeValue {
  switch (typeName) {
    case 'AT_INTS':
      return { i64s: list.map((v) => Number.parseInt(v, 10)).filter((v) => !Number.isNaN(v)), is_na: false };
    case 'AT_FLOATS':
      return { fs: list.map((v) => Number.parseFloat(v)).filter((v) => !Number.isNaN(v)), is_na: false };
    case 'AT_BOOLS':
      return { bs: list.map((v) => v === 'true' || v === '1'), is_na: false };
    case 'AT_STRINGS':
    case 'AT_PARTY':
    default:
      return { ss: list, is_na: false };
  }
}

/** 取出标量字符串表示（用于单值输入框）。 */
function scalarToString(value: AttributeValue | undefined): string {
  if (!value || value.is_na) return '';
  if (value.s !== undefined) return value.s;
  if (value.i64 !== undefined) return String(value.i64);
  if (value.f !== undefined) return String(value.f);
  if (value.b !== undefined) return String(value.b);
  return '';
}

/* -------------------------------------------------------------------------- */
/* 表单状态编解码                                                              */
/* -------------------------------------------------------------------------- */

/**
 * 从 nodeDef 的 attrPaths / attrs 平行数组构建 path -> value 的映射，
 * 作为表单的初始值来源。
 */
function initialValuesFromNodeDef(nodeDef: Record<string, unknown> | undefined): Record<string, AttributeValue> {
  const result: Record<string, AttributeValue> = {};
  if (!nodeDef) return result;
  const paths = Array.isArray(nodeDef.attrPaths) ? (nodeDef.attrPaths as string[]) : [];
  const attrs = Array.isArray(nodeDef.attrs) ? (nodeDef.attrs as AttributeValue[]) : [];
  paths.forEach((path, idx) => {
    const value = attrs[idx];
    if (path && value !== undefined) {
      result[path] = value as AttributeValue;
    }
  });
  return result;
}

/**
 * 构建表单初始状态：先用属性定义的 default_value 填充所有原子属性，
 * 再用 nodeDef 中的既有值覆盖，保证“未填写的属性使用默认值、已填写的保留原值”。
 */
function buildInitialState(
  tree: AttrTreeNode[],
  nodeDef: Record<string, unknown> | undefined
): Record<string, AttributeValue> {
  const next: Record<string, AttributeValue> = {};
  const fillDefaults = (nodes: AttrTreeNode[]) => {
    for (const node of nodes) {
      const { typeName, path } = node;
      if (typeName === 'AT_STRUCT_GROUP') {
        fillDefaults(node.children);
        continue;
      }
      if (typeName === 'AT_UNION_GROUP') {
        // 联合组默认选中 default_selection。
        const defaultSel = node.def.union?.default_selection;
        if (defaultSel) {
          next[path] = { s: defaultSel, is_na: false };
        }
        fillDefaults(node.children);
        continue;
      }
      const dv = node.def.atomic?.default_value;
      if (dv && !isNa(dv)) {
        next[path] = dv;
      }
    }
  };
  fillDefaults(tree);
  // nodeDef 既有值覆盖默认值。
  Object.assign(next, initialValuesFromNodeDef(nodeDef));
  return next;
}

/**
 * 遍历属性树，将当前表单状态序列化为 attrPaths / attrs 平行数组。
 *
 * 规则：
 * - 结构组（AT_STRUCT_GROUP）仅作为分组，不产出值，递归其子节点；
 * - 联合组（AT_UNION_GROUP）产出自身路径上的“选中子项名”值，并仅递归选中子树；
 * - 原子属性产出对应类型的值；若值为空且属性可选则跳过，保持 nodeDef 精简；
 * - ATTR_TYPE_UNSPECIFIED 为联合组的哑子项，不产出值。
 */
function serializeTree(
  nodes: AttrTreeNode[],
  state: Record<string, AttributeValue>,
  out: { paths: string[]; attrs: AttributeValue[] }
): void {
  for (const node of nodes) {
    const { typeName, path, children } = node;
    if (typeName === 'AT_STRUCT_GROUP') {
      serializeTree(children, state, out);
      continue;
    }
    if (typeName === 'AT_UNION_GROUP') {
      const selection = state[path];
      const selectedName = selection && !selection.is_na ? selection.s ?? '' : '';
      // 联合组自身存储选中子项名。
      out.paths.push(path);
      out.attrs.push(selectedName ? { s: selectedName, is_na: false } : naValue());
      // 仅展开选中的子树。
      const selectedChild = children.find((c) => c.def.name === selectedName);
      if (selectedChild) {
        serializeTree([selectedChild], state, out);
      }
      continue;
    }
    if (typeName === 'ATTR_TYPE_UNSPECIFIED') {
      // 联合组哑子项：不产出值，但递归其后代（若有）。
      serializeTree(children, state, out);
      continue;
    }
    // 原子类型 / 自定义 protobuf。
    const value = state[path];
    const optional = node.def.atomic?.is_optional === true;
    if (isNa(value)) {
      if (!optional) {
        // 必填但为空：显式写入 is_na，交由后端校验提示。
        out.paths.push(path);
        out.attrs.push(naValue());
      }
      continue;
    }
    out.paths.push(path);
    out.attrs.push(value as AttributeValue);
  }
}

/* -------------------------------------------------------------------------- */
/* 单个原子属性输入控件                                                        */
/* -------------------------------------------------------------------------- */

interface AtomicFieldProps {
  node: AttrTreeNode;
  value: AttributeValue | undefined;
  readOnly?: boolean;
  labels: AttributeFormLabels;
  onChange: (value: AttributeValue) => void;
}

/** 渲染单个原子属性的输入控件。 */
const AtomicField: React.FC<AtomicFieldProps> = ({ node, value, readOnly, labels, onChange }) => {
  const { typeName, def } = node;
  const atomic = def.atomic;
  const allowed = valueToList(atomic?.allowed_values);
  const isList = typeName === 'AT_STRINGS' || typeName === 'AT_INTS' || typeName === 'AT_FLOATS' || typeName === 'AT_BOOLS' || typeName === 'AT_PARTY';

  // 布尔类型：渲染为复选框。
  if (typeName === 'AT_BOOL') {
    const checked = value?.b === true;
    return (
      <label className="inline-flex items-center gap-2 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={checked}
          disabled={readOnly}
          onChange={(e) => onChange({ b: e.target.checked, is_na: false })}
          className="w-3.5 h-3.5 rounded border-gray-600 bg-gray-800 text-blue-500 focus:ring-blue-500"
        />
        <span className="text-gray-300">{checked ? 'true' : 'false'}</span>
      </label>
    );
  }

  // 列表类型：渲染为逗号分隔文本输入。
  if (isList) {
    const text = valueToList(value).join(', ');
    return (
      <input
        type="text"
        value={text}
        disabled={readOnly}
        placeholder={labels.listPlaceholder ?? '逗号分隔，如 a, b, c'}
        onChange={(e) => {
          const list = e.target.value
            .split(',')
            .map((v) => v.trim())
            .filter((v) => v.length > 0);
          onChange(list.length > 0 ? listToValue(typeName, list) : naValue());
        }}
        className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] font-mono focus:outline-none focus:border-blue-500 disabled:opacity-50"
      />
    );
  }

  // 数值类型：渲染为 number 输入。
  if (typeName === 'AT_INT' || typeName === 'AT_FLOAT') {
    const num = scalarToString(value);
    return (
      <input
        type="number"
        value={num}
        disabled={readOnly}
        step={typeName === 'AT_INT' ? 1 : 'any'}
        onChange={(e) => {
          const raw = e.target.value;
          if (raw === '') {
            onChange(naValue());
            return;
          }
          if (typeName === 'AT_INT') {
            onChange({ i64: Number.parseInt(raw, 10), is_na: false });
          } else {
            onChange({ f: Number.parseFloat(raw), is_na: false });
          }
        }}
        className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] font-mono focus:outline-none focus:border-blue-500 disabled:opacity-50"
      />
    );
  }

  // 字符串类型：若存在 allowed_values 则渲染为下拉选择，否则为文本输入。
  if (allowed.length > 0) {
    const current = scalarToString(value);
    return (
      <select
        value={current}
        disabled={readOnly}
        onChange={(e) => onChange({ s: e.target.value, is_na: false })}
        className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] focus:outline-none focus:border-blue-500 disabled:opacity-50"
      >
        {atomic?.is_optional && <option value="">{labels.none ?? '（不设置）'}</option>}
        {allowed.map((opt) => (
          <option key={opt} value={opt}>
            {opt}
          </option>
        ))}
      </select>
    );
  }

  // 自定义 protobuf：渲染为 JSON 文本域。
  if (typeName === 'AT_CUSTOM_PROTOBUF') {
    const text = value && !value.is_na ? (value.s ?? JSON.stringify(value)) : '';
    return (
      <textarea
        value={text}
        disabled={readOnly}
        rows={3}
        placeholder='{"key": "value"}'
        onChange={(e) => onChange(e.target.value ? { s: e.target.value, is_na: false } : naValue())}
        className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] font-mono focus:outline-none focus:border-blue-500 disabled:opacity-50"
      />
    );
  }

  // 默认：字符串文本输入。
  return (
    <input
      type="text"
      value={scalarToString(value)}
      disabled={readOnly}
      onChange={(e) => onChange(e.target.value ? { s: e.target.value, is_na: false } : naValue())}
      className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] font-mono focus:outline-none focus:border-blue-500 disabled:opacity-50"
    />
  );
};

/* -------------------------------------------------------------------------- */
/* 递归渲染属性树                                                              */
/* -------------------------------------------------------------------------- */

interface RenderTreeProps {
  nodes: AttrTreeNode[];
  state: Record<string, AttributeValue>;
  readOnly?: boolean;
  labels: AttributeFormLabels;
  depth: number;
  onChange: (path: string, value: AttributeValue) => void;
}

/** 递归渲染属性树（结构组折叠、联合组单选）。 */
const RenderTree: React.FC<RenderTreeProps> = ({ nodes, state, readOnly, labels, depth, onChange }) => {
  return (
    <div className={depth > 0 ? 'pl-3 border-l border-gray-800 space-y-3' : 'space-y-3'}>
      {nodes.map((node) => {
        const { typeName, path, def, children } = node;
        const displayName = def.name ?? path;
        const desc = def.desc;

        // 结构组：渲染为分组标题 + 递归子节点。
        if (typeName === 'AT_STRUCT_GROUP') {
          return (
            <div key={path} className="space-y-2">
              <div className="text-gray-400 text-[10px] font-bold uppercase tracking-wide flex items-center gap-1.5">
                <span className="w-1 h-3 bg-blue-500/60 rounded-full" />
                {displayName}
              </div>
              {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
              <RenderTree nodes={children} state={state} readOnly={readOnly} labels={labels} depth={depth + 1} onChange={onChange} />
            </div>
          );
        }

        // 联合组：渲染为互斥单选 + 仅展开选中子树。
        if (typeName === 'AT_UNION_GROUP') {
          const selection = state[path];
          const selectedName = selection && !selection.is_na ? selection.s ?? '' : '';
          return (
            <div key={path} className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-gray-300 text-[11px] font-semibold">{displayName}</span>
                <span className="text-gray-600 text-[9px] font-mono">{path}</span>
              </div>
              {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
              <select
                value={selectedName}
                disabled={readOnly}
                onChange={(e) => onChange(path, e.target.value ? { s: e.target.value, is_na: false } : naValue())}
                className="w-full p-1.5 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] focus:outline-none focus:border-blue-500 disabled:opacity-50"
              >
                <option value="">{labels.none ?? '（不选择）'}</option>
                {children.map((child) => (
                  <option key={child.path} value={child.def.name ?? ''}>
                    {child.def.name}
                  </option>
                ))}
              </select>
              {selectedName && (
                <RenderTree
                  nodes={children.filter((c) => c.def.name === selectedName)}
                  state={state}
                  readOnly={readOnly}
                  labels={labels}
                  depth={depth + 1}
                  onChange={onChange}
                />
              )}
            </div>
          );
        }

        // 哑子项（联合组下无配置的占位）：不渲染。
        if (typeName === 'ATTR_TYPE_UNSPECIFIED') {
          return null;
        }

        // 原子属性：标签 + 输入控件。
        const optional = def.atomic?.is_optional === true;
        return (
          <div key={path} className="space-y-1">
            <div className="flex items-center justify-between">
              <span className="text-gray-300 text-[11px] font-medium">
                {displayName}
                <span className={`ml-1.5 text-[9px] ${optional ? 'text-gray-600' : 'text-amber-500'}`}>
                  {optional ? labels.optional ?? '可选' : labels.required ?? '必填'}
                </span>
              </span>
              <span className="text-gray-600 text-[9px] font-mono truncate max-w-[45%]">{path}</span>
            </div>
            {desc && <div className="text-gray-500 text-[10px] leading-relaxed">{desc}</div>}
            <AtomicField node={node} value={state[path]} readOnly={readOnly} labels={labels} onChange={(v) => onChange(path, v)} />
          </div>
        );
      })}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 主组件                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * 组件属性动态表单。
 *
 * 内部维护 path -> AttributeValue 的表单状态；初始值优先取自 nodeDef 的
 * attrPaths/attrs，其次回退到属性定义中的 default_value。任一输入变化即
 * 重新序列化为 attrPaths/attrs 并通过 onNodeDefChange 回写宿主节点。
 */
export const AttributeForm: React.FC<AttributeFormProps> = ({ defs, nodeDef, readOnly, onNodeDefChange, labels = {} }) => {
  // 规范化属性定义列表。
  const normalizedDefs = useMemo<AttributeDef[]>(() => {
    if (!Array.isArray(defs)) return [];
    return defs as AttributeDef[];
  }, [defs]);

  // 构建属性树。
  const tree = useMemo(() => buildAttrTree(normalizedDefs), [normalizedDefs]);

  /**
   * 用 ref 持有最新 nodeDef，供“属性定义加载完成”时的重初始化使用，
   * 同时避免把 nodeDef 放进 effect 依赖导致“编辑 -> 回写 -> 重初始化”
   * 的循环（那会用默认值覆盖用户刚清空的可选字段）。
   *
   * 注意：ref 的同步更新放在 effect 中而非渲染期间，以符合 React
   * “不要在渲染阶段写 ref” 的约束（react-hooks/refs）。
   */
  const nodeDefRef = useRef(nodeDef);
  useEffect(() => {
    nodeDefRef.current = nodeDef;
  });

  // 表单状态：path -> AttributeValue。挂载时按当前 tree + nodeDef 初始化一次。
  const [state, setState] = useState<Record<string, AttributeValue>>(() => buildInitialState(tree, nodeDef));

  /**
   * 当属性定义（tree）变化时重建初始状态。
   *
   * 典型场景：组件定义为异步加载，表单先以空 tree 挂载，待定义返回后 tree
   * 更新，此时用最新 tree 与当前 nodeDef 重新初始化。注意这里刻意不依赖
   * nodeDef —— 用户编辑触发的 nodeDef 回写不应导致表单重置。
   */
  useEffect(() => {
    setState(buildInitialState(tree, nodeDefRef.current));
  }, [tree]);

  /** 属性无定义时给出占位提示。 */
  if (tree.length === 0) {
    return <div className="text-gray-600 text-[10px]">{labels.noAttrs ?? '该组件无可配置属性'}</div>;
  }

  /** 更新某个路径的值，并立即序列化回写宿主。 */
  const handleChange = (path: string, value: AttributeValue) => {
    const nextState = { ...state, [path]: value };
    setState(nextState);
    if (!onNodeDefChange) return;
    const out = { paths: [] as string[], attrs: [] as AttributeValue[] };
    serializeTree(tree, nextState, out);
    onNodeDefChange({
      ...(nodeDef ?? {}),
      attrPaths: out.paths,
      attrs: out.attrs,
    });
  };

  return (
    <div className="space-y-1">
      <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wider mb-2">
        {labels.advanced ?? '高级配置'}
      </div>
      <RenderTree nodes={tree} state={state} readOnly={readOnly} labels={labels} depth={0} onChange={handleChange} />
    </div>
  );
};
