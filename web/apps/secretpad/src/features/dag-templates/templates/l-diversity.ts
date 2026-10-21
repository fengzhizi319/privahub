/**
 * L-多样性模板。
 *
 * 旧前端对应 `pipeline-template-l-diversity.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/l_diversity 按准标识列与敏感列执行 L-多样性
 */
import type { KAnonymityTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { bAttr, buildSingleTablePrivacyTemplate, fAttr, i64Attr, jsonAttr } from '../builder';

export const lDiversityTemplate: TemplateContribution<KAnonymityTemplateConfig> = {
  metadata: {
    key: 'lDiversity',
    nameKey: 'lDiversity',
    descKey: 'lDiversity',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/l_diversity',
      label: 'L-多样性',
      nodeDef: {
        domain: 'privacy',
        name: 'l_diversity',
        version: '1.0.0',
        attrPaths: ['k', 'l', 'qi_cols_json', 'sa_cols_json', 'suppression_rate', 'report_result'],
        attrs: [
          i64Attr(2),
          i64Attr(2),
          jsonAttr(configs.qiCols || []),
          jsonAttr(configs.saCols || []),
          fAttr(0.05),
          bAttr(true),
        ],
      },
    });
  },
};
