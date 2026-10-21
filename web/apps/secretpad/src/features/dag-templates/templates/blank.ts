/**
 * 空白 DAG 模板。
 *
 * 不生成任何节点或边，仅创建一个空图，让用户在画布上从零编排。
 */
import type { TemplateBuildResult, TemplateContribution } from '../types';

export const blankTemplate: TemplateContribution = {
  metadata: {
    key: 'blank',
    nameKey: 'blank',
    descKey: 'blank',
    category: 'basic',
  },
  build(): TemplateBuildResult {
    return { nodes: [], edges: [] };
  },
};
