/**
 * K-匿名模板。
 *
 * 旧前端对应 `pipeline-template-k-anonymity.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/k_anonymity 按准标识列与敏感列执行 K-匿名
 */
import type { KAnonymityTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { bAttr, buildSingleTablePrivacyTemplate, fAttr, i64Attr, jsonAttr } from '../builder';

export const kAnonymityTemplate: TemplateContribution<KAnonymityTemplateConfig> = {
  metadata: {
    key: 'kAnonymity',
    nameKey: 'kAnonymity',
    descKey: 'kAnonymity',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/k_anonymity',
      label: 'K-匿名',
      nodeDef: {
        domain: 'privacy',
        name: 'k_anonymity',
        version: '1.1.0',
        attrPaths: [
          'k',
          'qi_cols_json',
          'sa_cols_json',
          'suppression_rate',
          'report_result',
          'max_depth',
          'hierarchies_json',
        ],
        attrs: [
          i64Attr(2),
          jsonAttr(configs.qiCols || []),
          jsonAttr(configs.saCols || []),
          fAttr(0.05),
          bAttr(true),
          i64Attr(10),
          jsonAttr({}),
        ],
      },
    });
  },
};
