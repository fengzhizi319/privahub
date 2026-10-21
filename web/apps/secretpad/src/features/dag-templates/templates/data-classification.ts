/**
 * 数据分类分级模板。
 *
 * 旧前端对应 `pipeline-template-data-classification.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/data_classification 执行自动分类分级
 */
import type { SingleTableTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { buildSingleTablePrivacyTemplate, sAttr } from '../builder';

export const dataClassificationTemplate: TemplateContribution<SingleTableTemplateConfig> = {
  metadata: {
    key: 'dataClassification',
    nameKey: 'dataClassification',
    descKey: 'dataClassification',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/data_classification',
      label: '数据分类分级',
      nodeDef: {
        domain: 'privacy',
        name: 'data_classification',
        version: '1.1.0',
        attrPaths: ['mode', 'mode/auto/default_level'],
        attrs: [sAttr('auto'), sAttr('L3')],
      },
    });
  },
};
