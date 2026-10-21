import React, { createRef } from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { AttributeForm, buildAttrTree, buildInitialState, prepareDefs, serializeState } from '../attribute-form';
import type { AttributeFormHandle } from '../attribute-form';
import {
  ATTR_TYPE_BY_NUMBER,
  arrangeByPanelStyle,
  buildInputColumnDefs,
  createComponentTranslator,
  isConfigFinished,
  normalizeAttrDef,
  resolveRenderer,
  validateAttr,
} from '../component-config';

/** Java 形态（camelCase + 字符串数字）的 data_prep/psi 定义，取自 privahub config/components.json。 */
const psiComponent = {
  attrs: [
    { name: 'protocol', desc: 'PSI protocol.', type: 'AT_UNION_GROUP', union: { defaultSelection: 'PROTOCOL_RR22' } },
    { prefixes: ['protocol'], name: 'PROTOCOL_ECDH', type: 'AT_UNION_GROUP', union: { defaultSelection: 'CURVE_25519' } },
    { prefixes: ['protocol', 'PROTOCOL_ECDH'], name: 'CURVE_25519' },
    { prefixes: ['protocol'], name: 'PROTOCOL_RR22', desc: 'RR22 protocol.' },
    { name: 'sort_result', type: 'AT_BOOL', atomic: { isOptional: true, defaultValue: { b: true } } },
    { name: 'receiver_parties', type: 'AT_PARTY', atomic: { listMaxLengthInclusive: '2' } },
    { name: 'allow_duplicate_keys', type: 'AT_UNION_GROUP', union: { defaultSelection: 'no' } },
    { prefixes: ['allow_duplicate_keys'], name: 'no', type: 'AT_STRUCT_GROUP' },
    { prefixes: ['allow_duplicate_keys', 'no'], name: 'check_hash_digest', type: 'AT_BOOL', atomic: { isOptional: true, defaultValue: { b: false } } },
    { name: 'fill_value_int', type: 'AT_INT', atomic: { isOptional: true, lowerBoundEnabled: true, lowerBound: { i64: '0' }, lowerBoundInclusive: true, upperBoundEnabled: true, upperBound: { i64: '10' } } },
  ],
  inputs: [
    { name: 'input_ds1', types: ['sf.table.individual'], attrs: [{ name: 'keys', colMinCntInclusive: '1', colMaxCntInclusive: '1' }] },
    { name: 'input_ds2', types: ['sf.table.individual'], attrs: [{ name: 'keys', colMinCntInclusive: '1', colMaxCntInclusive: '1' }] },
  ],
};

describe('属性定义规范化', () => {
  it('ATTR_TYPE_BY_NUMBER 与 secretflow_spec v1 AttrType 一致', () => {
    expect(ATTR_TYPE_BY_NUMBER[11]).toBe('AT_CUSTOM_PROTOBUF');
    expect(ATTR_TYPE_BY_NUMBER[12]).toBe('AT_PARTY');
    expect(ATTR_TYPE_BY_NUMBER[13]).toBe('AT_COL_PARAMS');
    expect(normalizeAttrDef({ name: 'x', type: 2 }).type).toBe('AT_INT');
    expect(normalizeAttrDef({ name: 'x', type: '4' }).type).toBe('AT_BOOL');
  });

  it('camelCase atomic 与字符串数字被规范化', () => {
    const d = normalizeAttrDef(psiComponent.attrs[5]);
    expect(d.atomic?.list_max_length_inclusive).toBe(2);
    const f = normalizeAttrDef(psiComponent.attrs[9]);
    expect(f.atomic).toMatchObject({ is_optional: true, lower_bound: { i64: 0 }, upper_bound: { i64: 10 }, lower_bound_inclusive: true });
  });

  it('inputs[].attrs 展开为 input/<in>/<col> 列属性，保留 col_min/max_cnt', () => {
    const defs = buildInputColumnDefs(psiComponent.inputs);
    const cols = defs.filter((d) => d.type === 'AT_SF_TABLE_COL');
    expect(cols.map((d) => [...(d.prefixes ?? []), d.name].join('/'))).toEqual(['input/input_ds1/keys', 'input/input_ds2/keys']);
    expect(cols[1]).toMatchObject({ fromInputIndex: 1, col_min_cnt_inclusive: 1, col_max_cnt_inclusive: 1, atomic: { is_optional: false } });
  });
});

describe('renderKey 注册表与面板样式', () => {
  it('按 codeName + 属性决定渲染器（镜像旧版 config-render-contribution）', () => {
    const r = (codeName: string, name: string, typeName: never, extra: Partial<Parameters<typeof resolveRenderer>[0]> = {}) =>
      resolveRenderer({ codeName, path: name, name, typeName, hasAllowedValues: false, ...extra });
    expect(r('data_prep/psi', 'input/input_ds1/keys', 'AT_SF_TABLE_COL' as never)).toBe('unionKeySelect');
    expect(r('read_data/datatable', 'datatable_selected', 'AT_STRING' as never)).toBe('tableSelect');
    expect(r('ml.predict/read_model', 'model', 'AT_STRING' as never)).toBe('modelSelect');
    expect(r('preprocessing/sqlite', 'sql', 'AT_STRING' as never)).toBe('sql');
    expect(r('preprocessing/sql_processor', 'sql', 'AT_STRING' as never)).toBe('sql');
    expect(r('stats/scql_analysis', 'script_input', 'AT_STRING' as never)).toBe('sql');
    expect(r('stats/scql_analysis', 'task_initiator', 'AT_PARTY' as never)).toBe('taskInitiator');
    expect(r('data_prep/unbalance_psi', 'receiver_parties', 'AT_PARTY' as never)).toBe('unbalanceReceiver');
    expect(r('data_prep/unbalance_psi_cache', 'x', 'AT_PARTY' as never)).toBe('joinNodeSelect');
    expect(r('any/comp', 'receiver', 'AT_STRING' as never, { hasAllowedValues: true })).toBe('nodeSelectSingle');
    expect(r('any/comp', 'mode', 'AT_STRING' as never, { hasAllowedValues: true })).toBe('select');
    expect(r('data_filter/sample', 'weights', 'AT_FLOATS' as never)).toBe('hidden');
    expect(r('data_filter/sample', 'quantiles', 'AT_FLOATS' as never)).toBe('quantiles');
    expect(r('x/y', 'n', 'AT_INTS' as never)).toBe('list');
  });

  it('面板样式：隐藏 psi check_hash_digest / read_table datatable_partition，排序与高级配置分组', () => {
    const items = ['protocol', 'sort_result', 'allow_duplicate_keys', 'fill_value_int', 'receiver_parties'].map((path) => ({ path }));
    const { common, advanced } = arrangeByPanelStyle(items, 'data_prep/psi');
    expect(common.map((i) => i.path)).toEqual(['protocol', 'allow_duplicate_keys', 'receiver_parties']);
    expect(advanced.map((i) => i.path)).toEqual(['sort_result', 'fill_value_int']);
    expect(arrangeByPanelStyle([{ path: 'datatable_partition' }, { path: 'datatable_selected' }], 'read_data/datatable').common).toEqual([{ path: 'datatable_selected' }]);
  });
});

describe('属性校验', () => {
  const intDef = normalizeAttrDef({ name: 'n', type: 'AT_INT', atomic: { lowerBoundEnabled: true, lowerBound: { i64: 1 }, lowerBoundInclusive: false, upperBoundEnabled: true, upperBound: { i64: 5 }, upperBoundInclusive: true } });

  it('必填 / 整数 / 上下界（区分 inclusive）', () => {
    expect(validateAttr(intDef, undefined)?.code).toBe('REQUIRED');
    expect(validateAttr(intDef, { i64: 1 })?.code).toBe('MIN');
    expect(validateAttr(intDef, { i64: 5 })).toBeNull();
    expect(validateAttr(intDef, { i64: 6 })?.code).toBe('MAX');
    expect(validateAttr(intDef, { i64: 2.5 })?.code).toBe('NOT_INTEGER');
  });

  it('列表长度与元素校验', () => {
    const d = normalizeAttrDef({ name: 'l', type: 'AT_FLOATS', atomic: { listMinLengthInclusive: 2, listMaxLengthInclusive: 3, lowerBoundEnabled: true, lowerBound: { f: 0 }, lowerBoundInclusive: true } });
    expect(validateAttr(d, { fs: [1] })?.code).toBe('LIST_MIN');
    expect(validateAttr(d, { fs: [1, 2, 3, 4] })?.code).toBe('LIST_MAX');
    expect(validateAttr(d, { fs: [1, -1] })?.code).toBe('MIN');
    expect(validateAttr(d, { fs: [1, 2] })).toBeNull();
  });

  it('allowed_values 与列数量', () => {
    const s = normalizeAttrDef({ name: 's', type: 'AT_STRING', atomic: { allowedValues: { ss: ['a', 'b'] } } });
    expect(validateAttr(s, { s: 'c' })?.code).toBe('NOT_ALLOWED');
    const col = buildInputColumnDefs([{ name: 'in', attrs: [{ name: 'f', colMinCntInclusive: 2, colMaxCntInclusive: 3 }] }]).find((d) => d.type === 'AT_SF_TABLE_COL')!;
    expect(validateAttr(col, undefined)?.code).toBe('REQUIRED');
    expect(validateAttr(col, { ss: ['a'] })?.code).toBe('COL_MIN');
    expect(validateAttr(col, { ss: ['a', 'b', 'c', 'd'] })?.code).toBe('COL_MAX');
  });

  it('isConfigFinished：未配置过 / 必填为 is_na → 未完成', () => {
    const defs = prepareDefs(psiComponent.attrs, psiComponent.inputs);
    expect(isConfigFinished({ domain: 'data_prep', name: 'psi' }, defs)).toBe(false);
    expect(isConfigFinished({ attrPaths: ['input/input_ds1/keys', 'receiver_parties'], attrs: [{ ss: ['id'] }, { is_na: true }] }, defs)).toBe(false);
    expect(
      isConfigFinished(
        { attrPaths: ['input/input_ds1/keys', 'input/input_ds2/keys', 'receiver_parties'], attrs: [{ ss: ['id'] }, { ss: ['id'] }, { ss: ['alice'] }] },
        defs,
      ),
    ).toBe(true);
  });
});

describe('序列化与表单', () => {
  it('序列化：联合组只展开选中分支、ss 标量包数组、隐藏属性不产出、is_na 标记', () => {
    const defs = prepareDefs(psiComponent.attrs, psiComponent.inputs);
    const tree = buildAttrTree(defs);
    const state = buildInitialState(tree, { attrPaths: ['input/input_ds1/keys'], attrs: [{ ss: ['id'], is_na: false }] });
    state.values['receiver_parties'] = { s: 'alice' } as never;
    const out = serializeState(tree, state, { codeName: 'data_prep/psi' });
    expect(out.attrPaths).toEqual([
      'input/input_ds1/keys',
      'input/input_ds2/keys',
      'protocol',
      'sort_result',
      'receiver_parties',
      'allow_duplicate_keys',
      'fill_value_int',
    ]);
    const byPath = Object.fromEntries(out.attrPaths.map((p, i) => [p, out.attrs[i]]));
    expect(byPath['input/input_ds1/keys']).toEqual({ ss: ['id'], is_na: false });
    expect(byPath['input/input_ds2/keys']).toEqual({ ss: [], is_na: true });
    expect(byPath.protocol).toEqual({ s: 'PROTOCOL_RR22', is_na: false });
    expect(byPath.receiver_parties).toEqual({ ss: ['alice'], is_na: false });
    expect(out.attrPaths).not.toContain('allow_duplicate_keys/no/check_hash_digest');
    expect(byPath.fill_value_int).toEqual({ is_na: true });
  });

  it('列选择使用 dataProvider（按输入序号取上游列）并在应用前校验必填', async () => {
    const fetchColumns = vi.fn(async (_path: string, ctx?: { inputIndex?: number }) =>
      ctx?.inputIndex === 0 ? [{ name: 'id', type: 'str' }, { name: 'age', type: 'int' }] : [{ name: 'uid', type: 'str' }],
    );
    const fetchParties = vi.fn(async () => [
      { id: 'alice', name: 'alice' },
      { id: 'bob', name: 'bob' },
    ]);
    const onChange = vi.fn();
    const ref = createRef<AttributeFormHandle>();
    render(
      <AttributeForm
        ref={ref}
        defs={psiComponent.attrs}
        inputs={psiComponent.inputs}
        codeName="data_prep/psi"
        nodeDef={{ domain: 'data_prep', name: 'psi' }}
        dataProvider={{ fetchColumns, fetchParties }}
        onNodeDefChange={onChange}
        translate={(t) => (t === 'receiver_parties' ? '结果接收方' : t)}
      />,
    );
    await waitFor(() => expect(fetchColumns).toHaveBeenCalledWith('input/input_ds1/keys', expect.objectContaining({ inputIndex: 0 })));
    expect(screen.getByText('结果接收方')).toBeTruthy();
    expect(screen.queryByText('check_hash_digest')).toBeNull();

    let errors: unknown[] = [];
    act(() => {
      errors = ref.current!.validate();
    });
    expect(errors.map((e) => (e as { path: string }).path)).toEqual(['input/input_ds1/keys', 'input/input_ds2/keys', 'receiver_parties']);
    expect(screen.getAllByRole('alert').length).toBe(3);

    const [sel1] = await screen.findAllByRole('combobox');
    await waitFor(() => expect(sel1.querySelectorAll('option').length).toBeGreaterThan(1));
    fireEvent.change(sel1, { target: { value: 'id' } });
    const last = onChange.mock.calls.at(-1)!;
    expect(last[0].attrPaths[0]).toBe('input/input_ds1/keys');
    expect(last[0].attrs[0]).toEqual({ ss: ['id'], is_na: false });
    expect(last[1].isFinished).toBe(false);
  });

  it('数值字段越界时显示错误（应用前校验）', () => {
    const ref = createRef<AttributeFormHandle>();
    render(<AttributeForm ref={ref} defs={[{ name: 'n', type: 'AT_INT', atomic: { upperBoundEnabled: true, upperBound: { i64: 3 }, upperBoundInclusive: true } }]} nodeDef={{ attrPaths: ['n'], attrs: [{ i64: 9 }] }} />);
    act(() => {
      ref.current!.validate();
    });
    expect(screen.getByRole('alert').textContent).toContain('≤ 3');
  });

  it('组件翻译：兼容 Java 嵌套形态与扁平形态', () => {
    const t = createComponentTranslator({ secretflow: { 'data_prep/psi:1.0.0': { receiver_parties: '接收方' } }, trustedflow: {} });
    expect(t.translate('receiver_parties', 'data_prep/psi', '1.0.0')).toBe('接收方');
    expect(createComponentTranslator({ psi: '隐私求交' }).translate('psi')).toBe('隐私求交');
  });
});

it('shows an actionable validation summary without changing configured values',()=>{
 const onChange=vi.fn();
 render(<AttributeForm defs={[{name:'n',type:'AT_INT',atomic:{upperBoundEnabled:true,upperBound:{i64:3},upperBoundInclusive:true}}]} nodeDef={{attrPaths:['n'],attrs:[{i64:9}]}} onNodeDefChange={onChange} labels={{issues:'待完善配置',validate:'检查配置',valid:'配置完整'}}/>);
 const summary=screen.getByRole('button',{name:/待完善配置/});
 expect(summary.textContent).toContain('(1)');
 fireEvent.click(summary);
 expect(screen.getByRole('alert').textContent).toContain('≤ 3');
 expect(screen.getByRole('spinbutton')).toHaveProperty('value','9');
 expect(onChange).not.toHaveBeenCalled();
});
