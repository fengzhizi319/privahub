/**
 * 模板快速配置抽屉 —— 逐字段对齐旧版 `component-config/template-quick-config/*`：
 *
 * | 模板 | 旧文件 | 字段（表单值形状与旧版一致，便于模板构建） |
 * |---|---|---|
 * | PSI（MPC） | quick-config-psi `type="MPC"` | dataTableReceiver{s}、dataTableReceiverPartition（ODPS 分区表）、receiverKey{ss}（可增删多个关联键）、dataTableSender{s}（不能与样本表1相同）、dataTableSenderPartition、senderKey{ss}、featureSelects{ss}（≥1）、receiverPSI{ss}（结果接收方，多选） |
 * | PSI_TEE | quick-config-psi | 同上，但无分区 / 特征 / 接收方 |
 * | RISK | quick-config-risk | PSI(MPC) + labelSelects{ss}（恰 1 列）、pred{s}（默认 pred）、receiver{ss:[节点]}（单选） |
 * | TEE | quick-config-risk-tee | PSI(TEE) + featureSelects{ss}（≥1）、labelSelect{ss}（恰 1）、trainIdSelect{ss}、saveId（开关）、predictIdSelect{ss}（≤1）、saveLabel（开关）、label{ss}（默认 label）、score{ss}（默认 pred） |
 * | 差分隐私 / 本地差分隐私 | quick-config-privacy | dataTable{s}（默认 alice 的第一张表）、queryCol{s}（默认 age 或第一列） |
 * | K-匿名 / L-多样性 | quick-config-k-anonymity | dataTable{s}、qiCols[]（默认 age）、saCols[]（默认最后一列） |
 * | 数据脱敏 | quick-config-sanitization | dataTable{s}、sanitizationCols[] |
 *
 * 顶部警告与底部「保存 / 取消」同旧版；零外部依赖，文案可通过 labels 覆盖。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { MultiSelect } from './attribute-form';

/* -------------------------------------------------------------------------- */
/* 类型定义                                                                    */
/* -------------------------------------------------------------------------- */

/** 模板类型枚举（对应原版 PipelineTemplateType）。 */
export type TemplateType =
  | 'PSI'
  | 'PSI_TEE'
  | 'RISK'
  | 'TEE'
  | 'DIFFERENTIAL_PRIVACY'
  | 'LOCAL_DIFFERENTIAL_PRIVACY'
  | 'K_ANONYMITY'
  | 'L_DIVERSITY'
  | 'SANITIZATION';

/** 表信息。 */
export interface TableInfo {
  datatableId: string;
  datatableName: string;
  nodeId: string;
  nodeName: string;
  /** ODPS 分区表（旧版 partition.type === 'odps' && fields）。 */
  isPartitionTable?: boolean;
  columns?: string[];
}

/** 快速配置表单值（与旧版 Form 值形状一致）。 */
export interface QuickConfigValues {
  [key: string]: unknown;
}

/** 组件文案标签。 */
export interface QuickConfigLabels {
  title?: string;
  save?: string;
  cancel?: string;
  warning?: string;
}

/**
 * 组件 Props。
 *
 * 抽屉按 `absolute` 定位，宿主需把它放进已定位的容器——DAG 页通过
 * `DAGNextWorkspace.centerOverlay` 挂在画布区域内（旧版 Drawer getContainer=.center）。
 */
export interface TemplateQuickConfigProps {
  /** 当前模板类型。 */
  templateType: TemplateType;
  /** 项目已授权数据表。 */
  tables?: TableInfo[];
  /** 读取数据表列（旧版 project/datatable/get 的 configs[].colName）；未提供时使用 table.columns。 */
  fetchColumns?: (table: TableInfo) => Promise<string[]>;
  /** 保存回调。 */
  onSave?: (values: QuickConfigValues) => void | Promise<void>;
  /** 关闭回调。 */
  onClose?: () => void;
  /** 文案标签。 */
  labels?: QuickConfigLabels;
  /** 是否可见。 */
  visible?: boolean;
}

/* -------------------------------------------------------------------------- */
/* 基础控件                                                                    */
/* -------------------------------------------------------------------------- */

const ctrlCls =
  'w-full bg-gray-800 border border-gray-700 rounded px-2 py-1.5 text-xs text-gray-200 focus:border-cyan-500 focus:outline-none';
const labelCls = 'block text-[11px] text-gray-400 mb-1 font-medium';

const Field: React.FC<{ label: string; required?: boolean; tooltip?: string; error?: string; children: React.ReactNode }> = ({
  label,
  required,
  tooltip,
  error,
  children,
}) => (
  <div>
    <label className={labelCls} title={tooltip}>
      {required && <span className="text-red-400 mr-0.5">*</span>}
      {label}
      {tooltip && <span className="ml-1 text-gray-500 cursor-help">ⓘ</span>}
    </label>
    {children}
    {error && <div className="mt-0.5 text-[10px] text-red-400">{error}</div>}
  </div>
);

const PARTITION_TIP = [
  '1. 填写dt=maxpt，则获取最新分区；dt为分区字段',
  '2. 如自定义规则获取分区表，可填写如：dt=${yyyymmdd+/- 3}',
  '3. 如选择多表自动union，则填写如dt=20240607 or dt=20240608',
  '4. 支持and or 作为多个分区列条件聚合，支持 =  !=  < > >= <= 作为分区列比较条件,其他暂不支持',
  '5. 条件列必须是添加数据表时指定的一级或二级分区字段',
].join('\n');

const TableSelect: React.FC<{ tables: TableInfo[]; value?: string; onChange: (v: string) => void; ariaLabel: string; showNode?: boolean }> = ({
  tables,
  value,
  onChange,
  ariaLabel,
  showNode = true,
}) => (
  <select aria-label={ariaLabel} className={ctrlCls} value={value ?? ''} onChange={(e) => onChange(e.target.value)}>
    <option value="">--</option>
    {tables.map((t) => (
      <option key={t.datatableId} value={t.datatableId}>
        {t.datatableName}
        {showNode ? ` (${t.nodeName})` : ''}
        {t.isPartitionTable ? ' [分区表]' : ''}
      </option>
    ))}
  </select>
);

/** 数据表列加载（带缓存）。 */
function useColumns(tables: TableInfo[], tableId: string | undefined, fetchColumns?: (t: TableInfo) => Promise<string[]>) {
  const [cols, setCols] = useState<string[]>([]);
  useEffect(() => {
    let alive = true;
    const table = tables.find((t) => t.datatableId === tableId);
    if (!table) {
      setCols([]);
      return;
    }
    if (!fetchColumns) {
      setCols(table.columns || []);
      return;
    }
    fetchColumns(table)
      .then((c) => alive && setCols(c))
      .catch(() => alive && setCols(table.columns || []));
    return () => {
      alive = false;
    };
  }, [tables, tableId, fetchColumns]);
  return cols;
}

/** 关联键列表（旧版 Form.List：首个必填，+ 追加，删除其余）。 */
const KeyList: React.FC<{ cols: string[]; value: string[]; onChange: (v: string[]) => void; error?: string; prefix: string }> = ({ cols, value, onChange, error, prefix }) => (
  <div className="space-y-2">
    {value.map((k, i) => (
      <div key={i} className="flex items-end gap-1">
        <div className="flex-1">
          <Field label={`关联键${i + 1}`} required={i === 0} error={i === 0 ? error : !k && error ? '关联键为空，请删除' : undefined}>
            <select
              aria-label={`${prefix}-key-${i + 1}`}
              className={ctrlCls}
              value={k}
              onChange={(e) => onChange(value.map((x, j) => (j === i ? e.target.value : x)))}
            >
              <option value="">--</option>
              {cols.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </Field>
        </div>
        {i === 0 ? (
          <button type="button" aria-label={`${prefix}-add-key`} className="mb-1 px-1.5 text-cyan-400 hover:text-cyan-300" onClick={() => onChange([...value, ''])}>
            ＋
          </button>
        ) : (
          <button type="button" aria-label={`${prefix}-remove-key-${i + 1}`} className="mb-1 px-1.5 text-gray-500 hover:text-red-400" onClick={() => onChange(value.filter((_, j) => j !== i))}>
            🗑
          </button>
        )}
      </div>
    ))}
  </div>
);

/** 两张表的列合集（旧版 MultiTableFeatureSelection）。 */
function useTwoTableColumns(tables: TableInfo[], a: string | undefined, b: string | undefined, fetchColumns?: (t: TableInfo) => Promise<string[]>) {
  const colsA = useColumns(tables, a, fetchColumns);
  const colsB = useColumns(tables, b, fetchColumns);
  return useMemo(() => {
    const ta = tables.find((t) => t.datatableId === a);
    const tb = tables.find((t) => t.datatableId === b);
    const opts: Array<{ value: string; label: string; hint?: string }> = [];
    const seen = new Set<string>();
    for (const [cols, t] of [
      [colsA, ta],
      [colsB, tb],
    ] as const) {
      for (const c of cols) {
        if (seen.has(c)) continue;
        seen.add(c);
        opts.push({ value: c, label: c, hint: t?.nodeName });
      }
    }
    return { colsA, colsB, options: opts };
  }, [tables, a, b, colsA, colsB]);
}

/* -------------------------------------------------------------------------- */
/* 表单状态与校验                                                               */
/* -------------------------------------------------------------------------- */

type Errors = Record<string, string | undefined>;

interface FormState {
  dataTableReceiver?: string;
  dataTableReceiverPartition?: string;
  receiverKeys: string[];
  dataTableSender?: string;
  dataTableSenderPartition?: string;
  senderKeys: string[];
  featureSelects: string[];
  receiverPSI: string[];
  labelSelects: string[];
  pred: string;
  receiver?: string;
  trainIdSelect: string[];
  saveId: boolean;
  predictIdSelect: string[];
  saveLabel: boolean;
  label: string;
  score: string;
  dataTable?: string;
  queryCol?: string;
  qiCols: string[];
  saCols: string[];
  sanitizationCols: string[];
}

const initialState = (): FormState => ({
  receiverKeys: [''],
  senderKeys: [''],
  featureSelects: [],
  receiverPSI: [],
  labelSelects: [],
  pred: 'pred',
  trainIdSelect: [],
  saveId: false,
  predictIdSelect: [],
  saveLabel: false,
  label: 'label',
  score: 'pred',
  qiCols: [],
  saCols: [],
  sanitizationCols: [],
});

const isTwoTable = (t: TemplateType) => t === 'PSI' || t === 'PSI_TEE' || t === 'RISK' || t === 'TEE';
const isMpcPsi = (t: TemplateType) => t === 'PSI' || t === 'RISK';

/** 旧版表单校验规则。 */
export function validateQuickConfig(type: TemplateType, s: FormState): Errors {
  const e: Errors = {};
  if (isTwoTable(type)) {
    if (!s.dataTableReceiver) e.dataTableReceiver = '请输入样本表';
    if (!s.dataTableSender) e.dataTableSender = '请输入样本表';
    else if (s.dataTableSender === s.dataTableReceiver) e.dataTableSender = '不能选择同一份样本表';
    if (!s.receiverKeys[0]) e.receiverKeys = '至少选择1列作为关联键';
    else if (s.receiverKeys.slice(1).some((k) => !k)) e.receiverKeys = '关联键为空，请删除';
    if (!s.senderKeys[0]) e.senderKeys = '至少选择1列作为关联键';
    else if (s.senderKeys.slice(1).some((k) => !k)) e.senderKeys = '关联键为空，请删除';
  }
  if (isMpcPsi(type) || type === 'TEE') {
    if (s.featureSelects.length === 0) e.featureSelects = '请选择特征列';
  }
  if (isMpcPsi(type) && s.receiverPSI.length === 0) e.receiverPSI = '请输入接收方';
  if (type === 'RISK') {
    if (s.labelSelects.length !== 1) e.labelSelects = '请选择标签列';
    if (!s.pred.trim()) e.pred = '请填写预测结果列名';
    if (!s.receiver) e.receiver = '请输入接收方';
  }
  if (type === 'TEE') {
    if (s.labelSelects.length !== 1) e.labelSelects = '请选择标签列';
    if (s.predictIdSelect.length > 1) e.predictIdSelect = '最多选择 1 列';
    if (!s.label.trim()) e.label = '请输入预测值标签';
    if (!s.score.trim()) e.score = '请输入预测值预测得分';
  }
  if (!isTwoTable(type)) {
    if (!s.dataTable) e.dataTable = '请输入样本表';
    if ((type === 'DIFFERENTIAL_PRIVACY' || type === 'LOCAL_DIFFERENTIAL_PRIVACY') && !s.queryCol) e.queryCol = '请输入查询列';
    if (type === 'K_ANONYMITY' || type === 'L_DIVERSITY') {
      if (s.qiCols.length === 0) e.qiCols = '请选择至少一个准标识符列';
      if (s.saCols.length === 0) e.saCols = '请选择至少一个敏感属性列';
    }
    if (type === 'SANITIZATION' && s.sanitizationCols.length === 0) e.sanitizationCols = '请选择至少一个需要脱敏的列';
  }
  return Object.fromEntries(Object.entries(e).filter(([, v]) => v));
}

/** 表单状态 → 旧版 Form 值形状（模板 content(graphId, options) 的 options）。 */
export function quickConfigValues(type: TemplateType, s: FormState, tables: TableInfo[]): QuickConfigValues {
  const partitionOf = (id?: string, p?: string) => (tables.find((t) => t.datatableId === id)?.isPartitionTable && p ? p : undefined);
  if (isTwoTable(type)) {
    const v: QuickConfigValues = {
      dataTableReceiver: { s: s.dataTableReceiver },
      dataTableSender: { s: s.dataTableSender },
      receiverKey: { ss: s.receiverKeys.filter(Boolean) },
      senderKey: { ss: s.senderKeys.filter(Boolean) },
    };
    if (isMpcPsi(type)) {
      v.dataTableReceiverPartition = partitionOf(s.dataTableReceiver, s.dataTableReceiverPartition);
      v.dataTableSenderPartition = partitionOf(s.dataTableSender, s.dataTableSenderPartition);
      v.featureSelects = { ss: s.featureSelects };
      v.receiverPSI = { ss: s.receiverPSI };
    }
    if (type === 'RISK') {
      v.labelSelects = { ss: s.labelSelects };
      v.pred = { s: s.pred };
      v.receiver = { ss: s.receiver ? [s.receiver] : [] };
    }
    if (type === 'TEE') {
      v.featureSelects = { ss: s.featureSelects };
      v.labelSelect = { ss: s.labelSelects };
      if (s.trainIdSelect.length) v.trainIdSelect = { ss: s.trainIdSelect };
      v.saveId = s.saveId;
      if (s.predictIdSelect.length) v.predictIdSelect = { ss: s.predictIdSelect };
      v.saveLabel = s.saveLabel;
      v.label = { ss: [s.label] };
      v.score = { ss: [s.score] };
    }
    return v;
  }
  const v: QuickConfigValues = { dataTable: { s: s.dataTable } };
  if (type === 'DIFFERENTIAL_PRIVACY' || type === 'LOCAL_DIFFERENTIAL_PRIVACY') v.queryCol = { s: s.queryCol };
  if (type === 'K_ANONYMITY' || type === 'L_DIVERSITY') {
    v.qiCols = s.qiCols;
    v.saCols = s.saCols;
  }
  if (type === 'SANITIZATION') v.sanitizationCols = s.sanitizationCols;
  return v;
}

/* -------------------------------------------------------------------------- */
/* 分模板表单                                                                   */
/* -------------------------------------------------------------------------- */

interface SubProps {
  type: TemplateType;
  tables: TableInfo[];
  state: FormState;
  errors: Errors;
  set: (patch: Partial<FormState>) => void;
  fetchColumns?: (t: TableInfo) => Promise<string[]>;
}

const TwoTableForm: React.FC<SubProps> = ({ type, tables, state, errors, set, fetchColumns }) => {
  const mpc = isMpcPsi(type);
  const { colsA, colsB, options } = useTwoTableColumns(tables, state.dataTableReceiver, state.dataTableSender, fetchColumns);
  const nodeOptions = useMemo(() => {
    const seen = new Set<string>();
    return tables
      .filter((t) => t.datatableId === state.dataTableReceiver || t.datatableId === state.dataTableSender)
      .filter((t) => (seen.has(t.nodeId) ? false : (seen.add(t.nodeId), true)))
      .map((t) => ({ value: t.nodeId, label: t.nodeName }));
  }, [tables, state.dataTableReceiver, state.dataTableSender]);
  const partitionOf = (id?: string) => mpc && !!tables.find((t) => t.datatableId === id)?.isPartitionTable;
  // 旧版：只有两张表都选中时才可选特征列。
  const featureOptions = state.dataTableReceiver && state.dataTableSender ? options : [];

  return (
    <div className="space-y-3">
      <Field label="样本表1" required error={errors.dataTableReceiver}>
        <TableSelect ariaLabel="dataTableReceiver" tables={tables} value={state.dataTableReceiver} onChange={(v) => set({ dataTableReceiver: v, receiverKeys: [''] })} />
      </Field>
      {partitionOf(state.dataTableReceiver) && (
        <Field label="分区" tooltip={PARTITION_TIP}>
          <input aria-label="dataTableReceiverPartition" className={ctrlCls} value={state.dataTableReceiverPartition ?? ''} onChange={(e) => set({ dataTableReceiverPartition: e.target.value })} />
        </Field>
      )}
      <KeyList prefix="receiver" cols={colsA} value={state.receiverKeys} onChange={(v) => set({ receiverKeys: v })} error={errors.receiverKeys} />

      <Field label="样本表2" required error={errors.dataTableSender}>
        <TableSelect ariaLabel="dataTableSender" tables={tables} value={state.dataTableSender} onChange={(v) => set({ dataTableSender: v, senderKeys: [''] })} />
      </Field>
      {partitionOf(state.dataTableSender) && (
        <Field label="分区" tooltip={PARTITION_TIP}>
          <input aria-label="dataTableSenderPartition" className={ctrlCls} value={state.dataTableSenderPartition ?? ''} onChange={(e) => set({ dataTableSenderPartition: e.target.value })} />
        </Field>
      )}
      <KeyList prefix="sender" cols={colsB} value={state.senderKeys} onChange={(v) => set({ senderKeys: v })} error={errors.senderKeys} />

      {(mpc || type === 'TEE') && (
        <Field label="选择特征" required error={errors.featureSelects}>
          <MultiSelect options={featureOptions} selected={state.featureSelects} onChange={(v) => set({ featureSelects: v })} placeholder="请选择特征列" />
        </Field>
      )}
      {mpc && (
        <Field label="PSI结果接收方" required tooltip="PSI结果的接收方" error={errors.receiverPSI}>
          <MultiSelect options={nodeOptions} selected={state.receiverPSI} onChange={(v) => set({ receiverPSI: v })} />
        </Field>
      )}

      {type === 'RISK' && (
        <>
          <Field label="选择标签" required error={errors.labelSelects}>
            <MultiSelect options={featureOptions} selected={state.labelSelects} max={1} onChange={(v) => set({ labelSelects: v.slice(-1) })} placeholder="请选择标签列" />
          </Field>
          <Field label="预测结果列名" required error={errors.pred}>
            <input aria-label="pred" className={ctrlCls} value={state.pred} onChange={(e) => set({ pred: e.target.value })} />
          </Field>
          <Field label="结果接收方" required tooltip="逻辑回归预测结果的接收方" error={errors.receiver}>
            <select aria-label="receiver" className={ctrlCls} value={state.receiver ?? ''} onChange={(e) => set({ receiver: e.target.value || undefined })}>
              <option value="">--</option>
              {nodeOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </Field>
        </>
      )}

      {type === 'TEE' && (
        <>
          <Field label="选择标签列" required error={errors.labelSelects}>
            <MultiSelect options={featureOptions} selected={state.labelSelects} max={1} onChange={(v) => set({ labelSelects: v.slice(-1) })} placeholder="请选择标签列" />
          </Field>
          <Field label="选择训练id列">
            <MultiSelect options={featureOptions} selected={state.trainIdSelect} onChange={(v) => set({ trainIdSelect: v })} placeholder="请选择训练id列" />
          </Field>
          <label className="flex items-center justify-between text-[11px] text-gray-400" title="是否将 id 列保存到输出预测表中；如果为 true，则输入feature_dataset必须包含 id 列">
            保存Id列
            <input aria-label="saveId" type="checkbox" checked={state.saveId} onChange={(e) => set({ saveId: e.target.checked })} />
          </label>
          <Field label="选择预测id列" error={errors.predictIdSelect}>
            <MultiSelect options={featureOptions} selected={state.predictIdSelect} max={1} onChange={(v) => set({ predictIdSelect: v.slice(-1) })} placeholder="请选择预测id列" />
          </Field>
          <label className="flex items-center justify-between text-[11px] text-gray-400" title="是否将真实的标签列保存到输出预测文件中；如果为 true，则输入feature_dataset必须包含标签列">
            保存label列
            <input aria-label="saveLabel" type="checkbox" checked={state.saveLabel} onChange={(e) => set({ saveLabel: e.target.checked })} />
          </label>
          <Field label="预测值 标签" required tooltip="标签值列名" error={errors.label}>
            <input aria-label="label" autoComplete="off" className={ctrlCls} value={state.label} onChange={(e) => set({ label: e.target.value })} />
          </Field>
          <Field label="预测值 预测得分" required tooltip="预测得分列名" error={errors.score}>
            <input aria-label="score" autoComplete="off" className={ctrlCls} value={state.score} onChange={(e) => set({ score: e.target.value })} />
          </Field>
        </>
      )}
    </div>
  );
};

const SingleTableForm: React.FC<SubProps> = ({ type, tables, state, errors, set, fetchColumns }) => {
  const cols = useColumns(tables, state.dataTable, fetchColumns);
  const options = useMemo(() => cols.map((c) => ({ value: c, label: c })), [cols]);

  // 旧版默认值：列加载后，隐私 → queryCol=age|第一列；K 匿名 → qi=[age]、sa=[最后一列]。
  useEffect(() => {
    if (!cols.length) return;
    if (type === 'DIFFERENTIAL_PRIVACY' || type === 'LOCAL_DIFFERENTIAL_PRIVACY') {
      set({ queryCol: cols.includes('age') ? 'age' : cols[0] });
    } else if (type === 'K_ANONYMITY' || type === 'L_DIVERSITY') {
      set({ ...(cols.includes('age') ? { qiCols: ['age'] } : {}), saCols: [cols[cols.length - 1]] });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- 仅在列变化时套用默认值
  }, [cols, type]);

  return (
    <div className="space-y-3">
      <Field label="样本表" required error={errors.dataTable}>
        <TableSelect ariaLabel="dataTable" tables={tables} value={state.dataTable} onChange={(v) => set({ dataTable: v, queryCol: undefined, qiCols: [], saCols: [], sanitizationCols: [] })} />
      </Field>
      {(type === 'DIFFERENTIAL_PRIVACY' || type === 'LOCAL_DIFFERENTIAL_PRIVACY') && (
        <Field label="查询列" required error={errors.queryCol}>
          <select aria-label="queryCol" className={ctrlCls} value={state.queryCol ?? ''} onChange={(e) => set({ queryCol: e.target.value || undefined })}>
            <option value="">--</option>
            {cols.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </Field>
      )}
      {(type === 'K_ANONYMITY' || type === 'L_DIVERSITY') && (
        <>
          <Field label="准标识符列 (QI)" required error={errors.qiCols}>
            <MultiSelect options={options} selected={state.qiCols} onChange={(v) => set({ qiCols: v })} placeholder="选择用于泛化/抑制的准标识符列" />
          </Field>
          <Field label="敏感属性列 (SA)" required error={errors.saCols}>
            <MultiSelect options={options} selected={state.saCols} onChange={(v) => set({ saCols: v })} placeholder="选择需要保护的敏感属性列" />
          </Field>
        </>
      )}
      {type === 'SANITIZATION' && (
        <Field label="脱敏列" required error={errors.sanitizationCols}>
          <MultiSelect options={options} selected={state.sanitizationCols} onChange={(v) => set({ sanitizationCols: v })} placeholder="选择需要脱敏处理的列（自动识别类型）" />
        </Field>
      )}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 主组件                                                                      */
/* -------------------------------------------------------------------------- */

/** 旧版单表模板默认选中 alice 的第一张表。 */
function defaultTable(tables: TableInfo[]): string | undefined {
  return (tables.find((t) => (t.nodeName || '').toLowerCase().includes('alice')) || tables[0])?.datatableId;
}

const Body: React.FC<Omit<TemplateQuickConfigProps, 'visible'>> = ({ templateType, tables = [], fetchColumns, onSave, onClose, labels = {} }) => {
  const [state, setState] = useState<FormState>(initialState);
  const [errors, setErrors] = useState<Errors>({});
  const [submitted, setSubmitted] = useState(false);
  const [saving, setSaving] = useState(false);
  const set = (patch: Partial<FormState>) => setState((prev) => ({ ...prev, ...patch }));

  useEffect(() => {
    if (!isTwoTable(templateType) && !state.dataTable && tables.length) set({ dataTable: defaultTable(tables) });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- 表列表到达后只设置一次默认值
  }, [tables, templateType]);

  useEffect(() => {
    if (submitted) setErrors(validateQuickConfig(templateType, state));
  }, [state, submitted, templateType]);

  const handleSave = async () => {
    setSubmitted(true);
    const errs = validateQuickConfig(templateType, state);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    setSaving(true);
    try {
      await onSave?.(quickConfigValues(templateType, state, tables));
    } finally {
      setSaving(false);
    }
  };

  const sub: SubProps = { type: templateType, tables, state, errors, set, fetchColumns };

  return (
    <div role="dialog" aria-label={labels.title ?? '快速配置'} className="absolute inset-y-0 right-0 z-40 w-[300px] flex flex-col bg-gray-900 border-l border-gray-800 shadow-2xl">
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-800">
        <h3 className="text-sm font-semibold text-gray-100">{labels.title ?? '快速配置'}</h3>
        <button type="button" aria-label="close" onClick={onClose} className="text-gray-500 hover:text-gray-300 text-sm">
          ✕
        </button>
      </div>
      <div className="mx-3 mt-3 px-3 py-2 bg-yellow-900/30 border border-yellow-700/50 rounded text-[10px] text-yellow-300">
        {labels.warning ?? '若无数据集，请先到节点授权数据到项目，再按组件进行配置，配置内容需要保存才能生效'}
      </div>
      <div className="flex-1 overflow-y-auto px-4 py-3">{isTwoTable(templateType) ? <TwoTableForm {...sub} /> : <SingleTableForm {...sub} />}</div>
      <div className="flex items-center gap-2 px-4 py-3 border-t border-gray-800">
        <button
          type="button"
          onClick={() => void handleSave()}
          disabled={saving}
          className="flex-1 px-3 py-1.5 bg-cyan-600 hover:bg-cyan-500 disabled:opacity-50 text-white text-xs rounded font-medium transition-colors"
        >
          {saving ? '...' : (labels.save ?? '保存')}
        </button>
        <button type="button" onClick={onClose} className="flex-1 px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded font-medium transition-colors">
          {labels.cancel ?? '取消'}
        </button>
      </div>
    </div>
  );
};

/**
 * 模板快速配置抽屉。关闭 / 切换模板时表单重置（旧版 onClose → form.resetFields）。
 */
export const TemplateQuickConfig: React.FC<TemplateQuickConfigProps> = ({ visible = true, ...rest }) => {
  if (!visible) return null;
  return <Body key={rest.templateType} {...rest} />;
};
