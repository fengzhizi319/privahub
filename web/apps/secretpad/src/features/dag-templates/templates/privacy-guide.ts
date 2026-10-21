/**
 * 隐私计算引导式模板（差分隐私示例）。
 *
 * 旧前端对应 `pipeline-template-privacy-guide.ts`：
 * - read_data/datatable（占位表 alice-table）
 * - privacy/differential_privacy 差分隐私查询节点
 *
 * 该模板为单节点引导示例，使用默认查询参数。
 */
import type { GuideTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { connect, createDifferentialPrivacyNode, createReadDataNode } from '../builder';

export const privacyGuideTemplate: TemplateContribution<GuideTemplateConfig> = {
  metadata: {
    key: 'privacyGuide',
    nameKey: 'privacyGuide',
    descKey: 'privacyGuide',
    computeModes: ['MPC', 'TEE'],
    category: 'guide',
  },
  build({ graphId }): TemplateBuildResult {
    const readNode = createReadDataNode(graphId, 1, 'alice-table', {
      x: -260,
      y: -210,
      label: '样本表',
    });

    const dpNode = createDifferentialPrivacyNode(graphId, 2, `${graphId}-node-1-output-0`, {
      x: -260,
      y: -80,
    });

    return {
      nodes: [readNode, dpNode],
      edges: [connect(graphId, 1, 0, 2, 0)],
    };
  },
};
