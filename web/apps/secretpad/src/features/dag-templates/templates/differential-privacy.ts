/**
 * 差分隐私查询模板。
 *
 * 旧前端对应 `pipeline-template-privacy.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/differential_privacy 执行 count 差分隐私查询
 */
import type { SingleTableTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { buildSingleTablePrivacyTemplate, fAttr, i64Attr, sAttr } from '../builder';

export const differentialPrivacyTemplate: TemplateContribution<SingleTableTemplateConfig> = {
  metadata: {
    key: 'differentialPrivacy',
    nameKey: 'differentialPrivacy',
    descKey: 'differentialPrivacy',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/differential_privacy',
      label: '差分隐私',
      nodeDef: {
        domain: 'privacy',
        name: 'differential_privacy',
        version: '1.1.0',
        attrPaths: [
          'query_type',
          'epsilon_total',
          'epsilon_per_query',
          'mechanism',
          'random_state',
          'min_count',
          'mode',
        ],
        attrs: [
          sAttr('count'),
          fAttr(10.0),
          fAttr(1.0),
          sAttr('laplace'),
          i64Attr(42),
          fAttr(5.0),
          sAttr('use_column_sensitivity'),
        ],
      },
    });
  },
};
