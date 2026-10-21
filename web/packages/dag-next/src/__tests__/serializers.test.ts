import { describe, it, expect } from 'vitest';
import {
  binModificationsSerializer,
  binModificationsUnSerializer,
  calculateSerializer,
  calculateUnserializer,
  caseWhenSerializer,
  caseWhenUnserializer,
  customSerializerRegistry,
  getBinBoundValue,
  getBinLabel,
  groupbySerializer,
  groupbyUnserializer,
  modelModificationsSerializer,
  modelModificationsUnSerializer,
  upstreamOutputFeatureSerializer,
  upstreamOutputFeatureUnserializer,
} from '../custom-serializers';

/**
 * 以旧前端实际写入 nodeDef.attrs 的格式为基准做往返（unserialize → serialize）校验，
 * 确保新前端保存的 custom_protobuf 与旧版 / 后端契约逐字一致。
 */
describe('custom_protobuf 序列化器（与旧版格式往返一致）', () => {
  it('CaseWhenRule：conds[0] 拆为 when，connections 回填到后续条件', () => {
    const cls = 'case_when_rules_pb2.CaseWhenRule';
    const stored = {
      custom_protobuf_cls: cls,
      custom_value: {
        whens: [
          {
            conds: [
              { cond_column: 'age', op: 'GT', cond_value: { type: 'CONST_INT', i: 18 } },
              { cond_column: 'income', op: 'LE', cond_value: { type: 'CONST_FLOAT', f: 1000.5 } },
            ],
            then: { type: 'CONST_STR', s: 'adult' },
            connections: ['AND'],
          },
        ],
        else_value: { type: 'CONST_STR', s: 'minor' },
        output_column: 'age_group',
        as_label: false,
        float_epsilon: 1e-6,
      },
    };
    const form = caseWhenUnserializer(stored);
    expect(form.whens?.[0].when).toEqual({ cond_column: 'age', op: 'GT', cond_value: { type: 'CONST_INT', i: 18 } });
    expect(form.whens?.[0].conds?.[0].connection).toBe('AND');
    expect(caseWhenSerializer(form, cls)).toEqual(stored);
    // 原始对象不应被反序列化过程修改（旧版会 shift 原数组）。
    expect(stored.custom_value.whens[0].conds).toHaveLength(2);
  });

  it('CaseWhenRule：空值给出旧版默认表单', () => {
    expect(caseWhenUnserializer()).toMatchObject({ whens: [{ when: { op: 'EQ' } }], as_label: false, float_epsilon: 10e-7 });
  });

  it('CalculateOpRules：newColNames 在表单中为逗号字符串，存储为数组', () => {
    const cls = 'calculate_rules_pb2.CalculateOpRules';
    const stored = { custom_value: { op: 'UNARY', operands: ['+', '/', '3'], newColNames: ['a_new', 'b_new'] }, custom_protobuf_cls: cls };
    const form = calculateUnserializer(stored);
    expect(form.newColNames).toBe('a_new,b_new');
    expect(calculateSerializer(form, cls)).toEqual(stored);
    expect(calculateUnserializer()).toEqual({ op: 'STANDARDIZE' });
  });

  it('GroupbyAggregationConfig', () => {
    const cls = 'groupby_aggregation_config_pb2.GroupbyAggregationConfig';
    const stored = { custom_value: { column_queries: [{ function: 'SUM', column_name: 'x' }, { function: 'MEAN', column_name: 'y' }] }, custom_protobuf_cls: cls };
    expect(groupbySerializer(groupbyUnserializer(stored), cls)).toEqual(stored);
    expect(groupbyUnserializer()).toEqual({ column_queries: [{}, {}] });
  });

  it('FeatureColumnConfig', () => {
    const cls = 'feature_column_config_pb2.FeatureColumnConfig';
    const stored = { custom_value: [{ tableName: 'alice_t1', tableFeatures: ['a', 'b'], nodeName: 'alice' }], custom_protobuf_cls: cls };
    expect(upstreamOutputFeatureSerializer(upstreamOutputFeatureUnserializer(stored), cls)).toEqual(stored);
    expect(upstreamOutputFeatureUnserializer()).toEqual([]);
  });

  it('Binning_modifications：WOE 分箱往返，区间标签与边界互转', () => {
    const stored = {
      modelHash: 'hash-1',
      variableBins: [
        {
          featureName: 'x1',
          validBinCount: 2,
          iv: 0.3,
          featureType: 'numeric',
          isWoe: true,
          partyName: 'alice',
          validBins: [
            { markForMerge: false, leftBound: '-Infinity', rightBound: 1.5, fillingValue: -0.2, totalCount: 10 },
            { markForMerge: true, leftBound: 1.5, rightBound: 'Infinity', fillingValue: 0.4, totalCount: 20 },
          ],
          elseBin: { fillingValue: 0, totalCount: 0, leftBound: 0, rightBound: 0, markForMerge: false },
        },
      ],
    };
    const form = binModificationsUnSerializer(stored)!;
    expect(form.variableBins[0].type).toBe('float');
    expect(form.variableBins[0].bins.map((b) => b.label)).toEqual(['(-Infinity, 1.5]', '(1.5, Infinity)', 'ELSE']);
    expect(binModificationsSerializer(form)).toEqual(stored);
    // 注册表以 { s: JSON } 形式保存（旧版 DefaultBinningModificationService）。
    const attr = customSerializerRegistry.Binning_modifications.serializer(form, 'Binning_modifications');
    expect(JSON.parse(attr.s as string)).toEqual(stored);
    expect(customSerializerRegistry.Binning_modifications.unserializer(attr)).toEqual(form);
  });

  it('getBinLabel / getBinBoundValue', () => {
    expect(getBinLabel(0, 0)).toBe('ELSE');
    expect(getBinLabel(1, 2)).toBe('(1, 2]');
    expect(getBinBoundValue('(1, 2]')).toEqual([1, 2]);
    expect(getBinBoundValue('ELSE')).toEqual([0, 0]);
  });

  it('linear_model_pb2：featureWeights 往返，key 仅存在于表单态', () => {
    const stored = { modelHash: 'm', featureWeights: [{ featureName: 'f1', party: 'alice', featureWeight: 0.5 }], bias: 0.1 };
    const form = modelModificationsUnSerializer(stored)!;
    expect(form.featureWeights[0].key).toBe('f1-alice');
    expect(modelModificationsSerializer(form)).toEqual(stored);
  });
});
