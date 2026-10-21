/**
 * 数据脱敏模板。
 *
 * 旧前端对应 `pipeline-template-sanitization.ts`：
 * - read_data/datatable 读取样本表
 * - privacy/sanitization 按用户选择列执行 auto_mask 脱敏
 */
import type { SanitizationTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { buildSingleTablePrivacyTemplate, jsonAttr } from '../builder';

export const sanitizationTemplate: TemplateContribution<SanitizationTemplateConfig> = {
  metadata: {
    key: 'sanitization',
    nameKey: 'sanitization',
    descKey: 'sanitization',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const rules = (configs.sanitizationCols || []).map((col) => ({ column: col, method: 'auto_mask' }));
    return buildSingleTablePrivacyTemplate(graphId, configs.tableId, {
      codeName: 'privacy/sanitization',
      label: '数据脱敏',
      nodeDef: {
        domain: 'privacy',
        name: 'sanitization',
        version: '1.1.0',
        attrPaths: ['rules_json'],
        attrs: [jsonAttr(rules)],
      },
    });
  },
};
