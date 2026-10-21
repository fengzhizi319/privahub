/**
 * AT_CUSTOM_PROTOBUF 自定义渲染器（对应旧版 config-item-render/custom-render/**）。
 *
 * 值均为“表单态”（经 custom-serializers 中的 unserializer 得到），修改后由 AttributeForm
 * 通过 serializer 序列化回 nodeDef。
 */
import React, { useEffect, useRef, useState } from 'react';
import type { AttrDataProvider, AttributeFormLabels, ColumnOption } from './attribute-form';
import type { CaseWhenCond, CaseWhenForm, CaseWhenValue, CalculateForm, Feature, GroupByConfig } from './custom-serializers';
import { AGGREGATION_FUNCTIONS, CALCULATE_OPERAND_INIT, CALCULATE_OPS } from './custom-serializers';

const inputCls =
  'p-1 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px] focus:outline-none focus:border-blue-500 disabled:opacity-50';

/** 读取第一个输入的列候选（含类型），用于 CaseWhen / GroupBy。 */
function useColumns(dataProvider?: AttrDataProvider): ColumnOption[] {
  const [cols, setCols] = useState<ColumnOption[]>([]);
  const ref = useRef(dataProvider);
  useEffect(() => {
    ref.current = dataProvider;
  });
  useEffect(() => {
    let cancelled = false;
    const fn = ref.current?.fetchColumns;
    if (!fn) return;
    fn('', { inputIndex: 0 })
      .then((list) => {
        if (!cancelled) setCols((list || []).map((c) => (typeof c === 'string' ? { name: c } : c)));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [dataProvider]);
  return cols;
}

const ColumnSelect: React.FC<{ cols: ColumnOption[]; value?: string; readOnly?: boolean; onChange: (v: string) => void; placeholder?: string }> = ({
  cols,
  value,
  readOnly,
  onChange,
  placeholder = '列名',
}) => (
  <select className={inputCls} value={value ?? ''} disabled={readOnly} onChange={(e) => onChange(e.target.value)}>
    <option value="">{placeholder}</option>
    {value && !cols.some((c) => c.name === value) && <option value={value}>{value}</option>}
    {cols.map((c) => (
      <option key={c.name} value={c.name}>
        {c.name}
      </option>
    ))}
  </select>
);

/* -------------------------------------------------------------------------- */
/* CaseWhen                                                                     */
/* -------------------------------------------------------------------------- */

const OPS = [
  { value: 'EQ', label: '==' },
  { value: 'NE', label: '!=' },
  { value: 'LT', label: '<' },
  { value: 'GT', label: '>' },
  { value: 'LE', label: '<=' },
  { value: 'GE', label: '>=' },
];

const VALUE_FIELD: Record<string, keyof CaseWhenValue> = { CONST_FLOAT: 'f', CONST_INT: 'i', CONST_STR: 's', COLUMN: 'column_name' };

/** 列类型 → 常量类型（旧版：int→CONST_INT，float→CONST_FLOAT，其余 CONST_STR）。 */
function constTypeForColumn(cols: ColumnOption[], col?: string): string {
  const t = (cols.find((c) => c.name === col)?.type || '').toLowerCase();
  if (t.includes('int')) return 'CONST_INT';
  if (t.includes('float') || t.includes('double')) return 'CONST_FLOAT';
  return 'CONST_STR';
}

const ValueEditor: React.FC<{ value?: CaseWhenValue; cols: ColumnOption[]; constType: string; readOnly?: boolean; onChange: (v: CaseWhenValue) => void }> = ({
  value,
  cols,
  constType,
  readOnly,
  onChange,
}) => {
  const isColumn = value?.type === 'COLUMN';
  const type = isColumn ? 'COLUMN' : constType;
  const field = VALUE_FIELD[type];
  const raw = (value as Record<string, unknown> | undefined)?.[field as string];
  return (
    <span className="inline-flex gap-1">
      <select
        className={inputCls}
        value={isColumn ? 'COLUMN' : 'CONST'}
        disabled={readOnly}
        onChange={(e) => onChange({ type: e.target.value === 'COLUMN' ? 'COLUMN' : constType })}
      >
        <option value="CONST">常量</option>
        <option value="COLUMN">列</option>
      </select>
      {isColumn ? (
        <ColumnSelect cols={cols} value={raw as string} readOnly={readOnly} onChange={(v) => onChange({ type: 'COLUMN', column_name: v })} />
      ) : (
        <input
          className={`${inputCls} w-24`}
          placeholder="请填写常量"
          disabled={readOnly}
          value={raw === undefined || raw === null ? '' : String(raw)}
          onChange={(e) => {
            const s = e.target.value;
            const v: Record<string, unknown> = { type };
            if (type === 'CONST_STR') v.s = s;
            else if (s !== '' && /^(-|\+)?\d+(\.\d+)?$/.test(s)) v[field as string] = Number(s);
            else v[field as string] = s;
            onChange(v as CaseWhenValue);
          }}
        />
      )}
    </span>
  );
};

const CondEditor: React.FC<{ cond?: CaseWhenCond; cols: ColumnOption[]; readOnly?: boolean; onChange: (c: CaseWhenCond) => void }> = ({ cond, cols, readOnly, onChange }) => (
  <span className="inline-flex flex-wrap gap-1 items-center">
    <ColumnSelect cols={cols} value={cond?.cond_column} readOnly={readOnly} onChange={(v) => onChange({ ...cond, cond_column: v, cond_value: { type: constTypeForColumn(cols, v) } })} />
    <select className={inputCls} value={cond?.op ?? 'EQ'} disabled={readOnly} onChange={(e) => onChange({ ...cond, op: e.target.value })}>
      {OPS.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
    <ValueEditor value={cond?.cond_value} cols={cols} constType={constTypeForColumn(cols, cond?.cond_column)} readOnly={readOnly} onChange={(v) => onChange({ ...cond, cond_value: v })} />
  </span>
);

export const CaseWhenEditor: React.FC<{ value: CaseWhenForm; readOnly?: boolean; dataProvider?: AttrDataProvider; onChange: (v: CaseWhenForm) => void }> = ({
  value,
  readOnly,
  dataProvider,
  onChange,
}) => {
  const cols = useColumns(dataProvider);
  const whens = value?.whens ?? [];
  const setWhen = (i: number, w: CaseWhenForm['whens'] extends Array<infer T> | undefined ? T : never) =>
    onChange({ ...value, whens: whens.map((x, j) => (j === i ? w : x)) });
  return (
    <div className="space-y-2 text-[11px] text-gray-300">
      <div className="flex justify-between items-center">
        <span className="font-semibold">Case</span>
        {!readOnly && (
          <button type="button" className="text-blue-400" onClick={() => onChange({ ...value, whens: [...whens, { when: { op: 'EQ', cond_value: { type: 'CONST_FLOAT' } }, then: { type: 'CONST_FLOAT' } }] })}>
            + WHEN
          </button>
        )}
      </div>
      {whens.map((w, i) => (
        <div key={i} className="p-1.5 rounded border border-gray-800 space-y-1">
          <div className="flex items-center gap-1 flex-wrap">
            <span className="text-gray-500 w-10">WHEN</span>
            <CondEditor cond={w.when} cols={cols} readOnly={readOnly} onChange={(c) => setWhen(i, { ...w, when: c })} />
            {!readOnly && whens.length > 1 && (
              <button type="button" className="text-red-400 ml-auto" aria-label="remove when" onClick={() => onChange({ ...value, whens: whens.filter((_, j) => j !== i) })}>
                ✕
              </button>
            )}
          </div>
          {(w.conds || []).map((c, ci) => (
            <div key={ci} className="flex items-center gap-1 flex-wrap pl-4">
              <select
                className={inputCls}
                value={c.connection ?? 'AND'}
                disabled={readOnly}
                onChange={(e) => setWhen(i, { ...w, conds: (w.conds || []).map((x, k) => (k === ci ? { ...x, connection: e.target.value } : x)) })}
              >
                <option value="AND">AND</option>
                <option value="OR">OR</option>
              </select>
              <CondEditor
                cond={c}
                cols={cols}
                readOnly={readOnly}
                onChange={(nc) => setWhen(i, { ...w, conds: (w.conds || []).map((x, k) => (k === ci ? { ...nc, connection: x.connection ?? 'AND' } : x)) })}
              />
              {!readOnly && (
                <button type="button" className="text-red-400" aria-label="remove cond" onClick={() => setWhen(i, { ...w, conds: (w.conds || []).filter((_, k) => k !== ci) })}>
                  ✕
                </button>
              )}
            </div>
          ))}
          {!readOnly && (
            <button type="button" className="text-blue-400 pl-4" onClick={() => setWhen(i, { ...w, conds: [...(w.conds || []), { connection: 'AND', op: 'EQ', cond_value: { type: 'CONST_FLOAT' } }] })}>
              + AND/OR
            </button>
          )}
          <div className="flex items-center gap-1">
            <span className="text-gray-500 w-10">THEN</span>
            <ValueEditor value={w.then} cols={cols} constType={w.then?.type && w.then.type !== 'COLUMN' ? String(w.then.type) : 'CONST_FLOAT'} readOnly={readOnly} onChange={(v) => setWhen(i, { ...w, then: v })} />
          </div>
        </div>
      ))}
      <div className="flex items-center gap-1">
        <span className="text-gray-500 w-10">ELSE</span>
        <ValueEditor
          value={value?.else_value}
          cols={cols}
          constType={value?.else_value?.type && value.else_value.type !== 'COLUMN' ? String(value.else_value.type) : 'CONST_FLOAT'}
          readOnly={readOnly}
          onChange={(v) => onChange({ ...value, else_value: v })}
        />
      </div>
      <label className="flex items-center gap-1">
        <span className="text-gray-500 w-24">output_column</span>
        <input className={`${inputCls} flex-1`} disabled={readOnly} value={value?.output_column ?? ''} onChange={(e) => onChange({ ...value, output_column: e.target.value })} />
      </label>
      <label className="flex items-center gap-1">
        <span className="text-gray-500 w-24">float_epsilon</span>
        <input
          type="number"
          className={`${inputCls} flex-1`}
          disabled={readOnly}
          value={value?.float_epsilon ?? ''}
          onChange={(e) => onChange({ ...value, float_epsilon: e.target.value === '' ? undefined : Number(e.target.value) })}
        />
      </label>
      <label className="flex items-center gap-1">
        <span className="text-gray-500 w-24">as_label</span>
        <input type="checkbox" disabled={readOnly} checked={!!value?.as_label} onChange={(e) => onChange({ ...value, as_label: e.target.checked })} />
      </label>
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* CalculateOp                                                                  */
/* -------------------------------------------------------------------------- */

const OP_LABELS: Record<string, string> = {
  STANDARDIZE: 'Standardize(标准化)',
  NORMALIZATION: '归一化',
  RANGE_LIMIT: 'Range(范围限制)',
  UNARY: 'Unary(单变量四则运算)',
  ROUND: 'Round(圆整)',
  LOG_ROUND: 'LogRound(取log后圆整)',
  SQRT: 'sqrt(根号)',
  LOG: 'Log',
  EXP: 'exp',
  LENGTH: 'length',
  SUBSTR: 'Substr',
  RECIPROCAL: 'Reciprocal(倒数)',
};

export const CalculateOpEditor: React.FC<{ value: CalculateForm; readOnly?: boolean; onChange: (v: CalculateForm) => void }> = ({ value, readOnly, onChange }) => {
  const op = value?.op ?? 'STANDARDIZE';
  const operands = value?.operands ?? CALCULATE_OPERAND_INIT[op as keyof typeof CALCULATE_OPERAND_INIT] ?? [];
  const setOperand = (i: number, v: string | number | null) => onChange({ ...value, op, operands: operands.map((x, j) => (j === i ? v : x)) });
  const numInput = (i: number, placeholder: string) => (
    <input
      key={i}
      className={`${inputCls} w-20`}
      placeholder={placeholder}
      disabled={readOnly}
      value={operands[i] === null || operands[i] === undefined ? '' : String(operands[i])}
      onChange={(e) => setOperand(i, e.target.value === '' ? null : e.target.value)}
    />
  );
  return (
    <div className="space-y-1 text-[11px] text-gray-300">
      <label className="flex items-center gap-1">
        <span className="text-gray-500 w-16">算子选择</span>
        <select
          className={`${inputCls} flex-1`}
          value={op}
          disabled={readOnly}
          onChange={(e) => {
            const next = e.target.value as keyof typeof CALCULATE_OPERAND_INIT;
            onChange({ ...value, op: next, operands: CALCULATE_OPERAND_INIT[next] ? [...CALCULATE_OPERAND_INIT[next]!] : undefined });
          }}
        >
          {CALCULATE_OPS.map((o) => (
            <option key={o} value={o}>
              {OP_LABELS[o]}
            </option>
          ))}
        </select>
      </label>
      {op === 'UNARY' && (
        <div className="flex gap-1 items-center">
          <select className={inputCls} disabled={readOnly} value={String(operands[0] ?? '+')} onChange={(e) => setOperand(0, e.target.value)}>
            <option value="+">列在前</option>
            <option value="-">列在后</option>
          </select>
          <select className={inputCls} disabled={readOnly} value={String(operands[1] ?? '/')} onChange={(e) => setOperand(1, e.target.value)}>
            {['+', '-', '*', '/'].map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </select>
          {numInput(2, '值')}
        </div>
      )}
      {op === 'LOG' && (
        <div className="flex gap-1 items-center">
          <select className={inputCls} disabled={readOnly} value={String(operands[0] ?? 'e')} onChange={(e) => setOperand(0, e.target.value)}>
            <option value="e">e</option>
            <option value="custom">自定义底数</option>
          </select>
          {numInput(1, '加数')}
        </div>
      )}
      {op === 'LOG_ROUND' && <div className="flex gap-1 items-center">{numInput(0, '加数')}</div>}
      {op === 'RANGE_LIMIT' && (
        <div className="flex gap-1 items-center">
          {numInput(0, '下界')}
          {numInput(1, '上界')}
        </div>
      )}
      {op === 'SUBSTR' && <div className="flex gap-1 items-center">{numInput(0, '起始位置')}{numInput(1, '长度')}</div>}
      {operands.some((o) => o === null || o === undefined || o === '') && <div className="text-red-400 text-[10px]">值不能为空</div>}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* GroupBy                                                                      */
/* -------------------------------------------------------------------------- */

export const GroupByEditor: React.FC<{ value: GroupByConfig; readOnly?: boolean; dataProvider?: AttrDataProvider; onChange: (v: GroupByConfig) => void }> = ({
  value,
  readOnly,
  dataProvider,
  onChange,
}) => {
  const cols = useColumns(dataProvider);
  const queries = value?.column_queries ?? [];
  return (
    <div className="space-y-1 text-[11px] text-gray-300">
      {queries.map((q, i) => (
        <div key={i} className="flex gap-1 items-center">
          <select
            className={inputCls}
            disabled={readOnly}
            value={q.function ?? ''}
            onChange={(e) => onChange({ column_queries: queries.map((x, j) => (j === i ? { ...x, function: e.target.value } : x)) })}
          >
            <option value="">统计方式</option>
            {AGGREGATION_FUNCTIONS.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
          </select>
          <ColumnSelect cols={cols} value={q.column_name} readOnly={readOnly} onChange={(v) => onChange({ column_queries: queries.map((x, j) => (j === i ? { ...x, column_name: v } : x)) })} />
          {!readOnly && (
            <button type="button" className="text-red-400" aria-label="remove query" onClick={() => onChange({ column_queries: queries.filter((_, j) => j !== i) })}>
              ✕
            </button>
          )}
        </div>
      ))}
      {!readOnly && (
        <button type="button" className="text-blue-400" onClick={() => onChange({ column_queries: [...queries, {}] })}>
          + 添加统计
        </button>
      )}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* FeatureColumnConfig（上游输出特征）                                           */
/* -------------------------------------------------------------------------- */

export const UpstreamFeatureView: React.FC<{ value: Feature[]; dataProvider?: AttrDataProvider; readOnly?: boolean; onChange: (v: Feature[]) => void }> = ({
  value,
  dataProvider,
  readOnly,
  onChange,
}) => {
  const onChangeRef = useRef(onChange);
  const valueRef = useRef(value);
  useEffect(() => {
    onChangeRef.current = onChange;
    valueRef.current = value;
  });
  useEffect(() => {
    let cancelled = false;
    if (!dataProvider?.fetchUpstreamFeatures) return;
    dataProvider
      .fetchUpstreamFeatures()
      .then((list) => {
        if (cancelled || readOnly) return;
        if (JSON.stringify(list) !== JSON.stringify(valueRef.current)) onChangeRef.current(list);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [dataProvider, readOnly]);
  return (
    <div className="space-y-1 text-[11px] text-gray-300">
      <div className="text-amber-400 text-[10px]">上游算子输出的结果表与特征，SQL 中需使用下列表名</div>
      {(value || []).length === 0 && <div className="text-gray-500">暂无上游输出，请先执行上游算子</div>}
      {(value || []).map((f) => (
        <div key={f.tableName} className="p-1.5 rounded border border-gray-800">
          <div className="font-mono text-blue-300">{f.tableName}</div>
          <div className="text-gray-500">{f.nodeName}</div>
          <div className="text-gray-400 break-all">{f.tableFeatures.join(', ')}</div>
        </div>
      ))}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 分派                                                                         */
/* -------------------------------------------------------------------------- */

export const CustomProtobufField: React.FC<{
  cls: string;
  value: unknown;
  readOnly?: boolean;
  dataProvider?: AttrDataProvider;
  labels: AttributeFormLabels;
  onChange: (v: unknown) => void;
  onOpenModification?: (cls: string) => void;
}> = ({ cls, value, readOnly, dataProvider, labels, onChange, onOpenModification }) => {
  switch (cls) {
    case 'case_when_rules_pb2.CaseWhenRule':
      return <CaseWhenEditor value={value as CaseWhenForm} readOnly={readOnly} dataProvider={dataProvider} onChange={onChange} />;
    case 'calculate_rules_pb2.CalculateOpRules':
      return <CalculateOpEditor value={value as CalculateForm} readOnly={readOnly} onChange={onChange} />;
    case 'groupby_aggregation_config_pb2.GroupbyAggregationConfig':
      return <GroupByEditor value={value as GroupByConfig} readOnly={readOnly} dataProvider={dataProvider} onChange={onChange} />;
    case 'feature_column_config_pb2.FeatureColumnConfig':
      return <UpstreamFeatureView value={(value as Feature[]) || []} readOnly={readOnly} dataProvider={dataProvider} onChange={onChange} />;
    case 'Binning_modifications':
    case 'linear_model_pb2':
      return (
        <button type="button" className="text-blue-400 text-[11px] hover:underline" onClick={() => onOpenModification?.(cls)}>
          {cls === 'Binning_modifications' ? labels.editBinning ?? '编辑分箱' : labels.editModelParams ?? '编辑模型参数'}
        </button>
      );
    default:
      return <JsonField value={value} readOnly={readOnly} onChange={onChange} />;
  }
};

const JsonField: React.FC<{ value: unknown; readOnly?: boolean; onChange: (v: unknown) => void }> = ({ value, readOnly, onChange }) => {
  const [text, setText] = useState(() => (value === undefined ? '' : JSON.stringify(value, null, 2)));
  const [invalid, setInvalid] = useState(false);
  return (
    <div>
      <textarea
        rows={4}
        className={`${inputCls} w-full font-mono ${invalid ? 'border-red-500' : ''}`}
        disabled={readOnly}
        value={text}
        placeholder='{"key": "value"}'
        onChange={(e) => {
          setText(e.target.value);
          try {
            onChange(e.target.value ? JSON.parse(e.target.value) : undefined);
            setInvalid(false);
          } catch {
            setInvalid(true);
          }
        }}
      />
    </div>
  );
};
