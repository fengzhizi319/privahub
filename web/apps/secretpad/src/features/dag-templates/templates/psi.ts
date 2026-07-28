/**
 * PSI 隐私求交模板。
 *
 * 旧前端对应 `pipeline-template-psi.ts`：
 * - 接收方 read_data/datatable
 * - 发送方 read_data/datatable
 * - data_prep/psi 执行隐私求交
 *
 * 新前端原有 `pages/dag/index.tsx` 中的硬编码 PSI 模板逻辑已全部迁移至此。
 */
import type { TemplateBuildResult, TemplateContribution, TwoTableTemplateConfig } from '../types';
import { connect, createPsiNode, createReadDataNode } from '../builder';

export const psiTemplate: TemplateContribution<TwoTableTemplateConfig> = {
  metadata: {
    key: 'psi',
    nameKey: 'psi',
    descKey: 'psi',
    category: 'basic',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const receiverNode = createReadDataNode(graphId, 1, configs.receiverTableId, {
      x: -390,
      y: -210,
      partition: configs.receiverPartition,
      label: '接收方样本表',
    });
    const senderNode = createReadDataNode(graphId, 2, configs.senderTableId, {
      x: -150,
      y: -210,
      partition: configs.senderPartition,
      label: '发送方样本表',
    });

    const receiverOut = `${graphId}-node-1-output-0`;
    const senderOut = `${graphId}-node-2-output-0`;

    const psiNode = createPsiNode(graphId, 3, [receiverOut, senderOut], {
      receiverKey: configs.receiverKey || '',
      senderKey: configs.senderKey || '',
      receiverNodeId: configs.receiverNodeId || '',
      senderNodeId: configs.senderNodeId || '',
      x: -260,
      y: -100,
    });

    return {
      nodes: [receiverNode, senderNode, psiNode],
      edges: [
        connect(graphId, 1, 0, 3, 0),
        connect(graphId, 2, 0, 3, 1),
      ],
    };
  },
};
