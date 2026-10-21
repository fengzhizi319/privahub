import { describe, expect, it } from 'vitest';
import { validateQuickConfig, quickConfigValues } from '@secretpad/dag-next';
import { templateByKey } from '../../features/dag-templates';
import { quickConfigToTemplateConfigs } from './quick-config';

const project = {
  projectId: 'p',
  nodes: [
    { nodeId: 'alice', nodeName: 'alice', datatables: [{ datatableId: 't1', datatableName: 'a' }] },
    { nodeId: 'bob', nodeName: 'bob', datatables: [{ datatableId: 't2', datatableName: 'b' }] },
  ],
};

const base = {
  receiverKeys: ['id'],
  senderKeys: ['id'],
  featureSelects: [] as string[],
  receiverPSI: [] as string[],
  labelSelects: [] as string[],
  pred: 'pred',
  trainIdSelect: [] as string[],
  saveId: false,
  predictIdSelect: [] as string[],
  saveLabel: false,
  label: 'label',
  score: 'pred',
  qiCols: [] as string[],
  saCols: [] as string[],
  sanitizationCols: [] as string[],
};

describe('模板快速配置（对齐旧版 template-quick-config）', () => {
  it('PSI：校验同表、关联键、特征与接收方', () => {
    const errs = validateQuickConfig('PSI', { ...base, dataTableReceiver: 't1', dataTableSender: 't1', receiverKeys: [''] });
    expect(errs.dataTableSender).toBe('不能选择同一份样本表');
    expect(errs.receiverKeys).toBe('至少选择1列作为关联键');
    expect(errs.featureSelects).toBeTruthy();
    expect(errs.receiverPSI).toBeTruthy();
    expect(validateQuickConfig('PSI_TEE', { ...base, dataTableReceiver: 't1', dataTableSender: 't2' })).toEqual({});
  });

  it('PSI：表单值 → 模板，写入多关联键、接收方与全表统计', () => {
    const state = { ...base, dataTableReceiver: 't1', dataTableSender: 't2', receiverKeys: ['id', 'k2'], featureSelects: ['x'], receiverPSI: ['alice'] };
    const values = quickConfigValues('PSI', state, []);
    expect(values.receiverKey).toEqual({ ss: ['id', 'k2'] });
    const configs = quickConfigToTemplateConfigs('PSI', values, project);
    const { nodes, edges } = templateByKey('psi')!.build({ graphId: 'g', configs });
    const psi = nodes.find((n) => n.codeName === 'data_prep/psi')!;
    const def = psi.nodeDef as { attrs: Array<{ ss?: string[] }> };
    expect(def.attrs[0].ss).toEqual(['id', 'k2']);
    expect(def.attrs[4].ss).toEqual(['alice']);
    expect(nodes.find((n) => n.codeName === 'stats/table_statistics')).toBeTruthy();
    expect(edges).toHaveLength(3);
  });

  it('RISK：预测接收方取快速配置的 receiver', () => {
    const state = { ...base, dataTableReceiver: 't1', dataTableSender: 't2', featureSelects: ['x'], receiverPSI: ['alice', 'bob'], labelSelects: ['y'], receiver: 'bob' };
    expect(validateQuickConfig('RISK', state)).toEqual({});
    const configs = quickConfigToTemplateConfigs('RISK', quickConfigValues('RISK', state, []), project);
    const { nodes } = templateByKey('risk')!.build({ graphId: 'g', configs });
    const predict = nodes.find((n) => n.codeName === 'ml.predict/ss_sgd_predict')!;
    expect((predict.nodeDef as { attrs: Array<{ s?: string }> }).attrs[0].s).toBe('bob');
  });

  it('K-匿名：QI / SA 必填并写入模板', () => {
    expect(validateQuickConfig('K_ANONYMITY', { ...base, dataTable: 't1' }).qiCols).toBeTruthy();
    const configs = quickConfigToTemplateConfigs('K_ANONYMITY', quickConfigValues('K_ANONYMITY', { ...base, dataTable: 't1', qiCols: ['age'], saCols: ['d'] }, []), project);
    expect(configs).toMatchObject({ tableId: 't1', nodeId: 'alice', qiCols: ['age'], saCols: ['d'] });
  });
});
