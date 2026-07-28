/**
 * 本地差分隐私模板。
 *
 * 旧前端对应 `pipeline-template-local-differential-privacy.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/local_differential_privacy 对指定列做扰动
 */
import type { LocalDifferentialPrivacyTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { buildSingleTablePrivacyTemplate, fAttr, i64Attr, jsonAttr, sAttr } from '../builder';

export const localDifferentialPrivacyTemplate: TemplateContribution<LocalDifferentialPrivacyTemplateConfig> = {
  metadata: {
    key: 'localDifferentialPrivacy',
    nameKey: 'localDifferentialPrivacy',
    descKey: 'localDifferentialPrivacy',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const queryCol = configs.queryCol || '';
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/local_differential_privacy',
      label: '本地差分隐私',
      nodeDef: {
        domain: 'privacy',
        name: 'local_differential_privacy',
        version: '1.0.0',
        attrPaths: ['op', 'mechanism', 'query_col', 'epsilon', 'categories_json', 'random_state'],
        attrs: [
          sAttr('perturb'),
          sAttr('binary_rr'),
          { s: queryCol, is_na: !queryCol },
          fAttr(1.0),
          jsonAttr([]),
          i64Attr(42),
        ],
      },
    });
  },
};
