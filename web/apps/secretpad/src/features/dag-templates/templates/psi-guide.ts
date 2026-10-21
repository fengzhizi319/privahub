/**
 * PSI 引导式模板。
 *
 * 旧前端对应 `pipeline-template-psi-guide.ts`：
 * - 接收方 read_data/datatable（占位表 alice-table）
 * - 发送方 read_data/datatable（占位表 bob-table）
 * - data_prep/psi 执行隐私求交
 * - stats/table_statistics 全表统计
 *
 * 与 `psi` 模板的差异：使用占位表与默认关联键，无需用户在向导中填写参数。
 */
import type { GuideTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import {
  connect,
  createPsiNode,
  createReadDataNode,
  createTableStatisticsNode,
} from '../builder';

export const psiGuideTemplate: TemplateContribution<GuideTemplateConfig> = {
  metadata: {
    key: 'psiGuide',
    nameKey: 'psiGuide',
    descKey: 'psiGuide',
    computeModes: ['MPC'],
    category: 'guide',
  },
  build({ graphId }): TemplateBuildResult {
    const receiverNode = createReadDataNode(graphId, 1, 'alice-table', {
      x: -390,
      y: -210,
      label: '样本表',
    });
    const senderNode = createReadDataNode(graphId, 2, 'bob-table', {
      x: -150,
      y: -210,
      label: '样本表',
    });

    const receiverOut = `${graphId}-node-1-output-0`;
    const senderOut = `${graphId}-node-2-output-0`;

    const psiNode = createPsiNode(graphId, 3, [receiverOut, senderOut], {
      receiverKey: 'id1',
      senderKey: 'id2',
      receiverNodeId: 'alice',
      senderNodeId: 'bob',
      x: -260,
      y: -100,
    });

    const psiOut = `${graphId}-node-3-output-0`;
    const statsNode = createTableStatisticsNode(graphId, 4, psiOut, {
      x: -260,
      y: 20,
      features: ['y', 'age', 'education', 'default'],
      version: '1.0.0',
    });

    return {
      nodes: [receiverNode, senderNode, psiNode, statsNode],
      edges: [
        connect(graphId, 1, 0, 3, 0),
        connect(graphId, 2, 0, 3, 1),
        connect(graphId, 3, 0, 4, 0),
      ],
    };
  },
};
