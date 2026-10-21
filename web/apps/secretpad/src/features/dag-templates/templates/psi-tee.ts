/**
 * TEE PSI 模板。
 *
 * 旧前端对应 `pipeline-template-psi-tee.ts`：
 * - 接收方 read_data/datatable
 * - 发送方 read_data/datatable
 * - preprocessing/psi 执行 TEE 隐私求交
 * - stats/table_statistics 全表统计
 */
import type { TemplateBuildResult, TemplateContribution, TwoTableTemplateConfig } from '../types';
import {
  connect,
  createReadDataNode,
  createTableStatisticsNode,
  createTeePsiNode,
} from '../builder';

export const psiTeeTemplate: TemplateContribution<TwoTableTemplateConfig> = {
  metadata: {
    key: 'psiTee',
    nameKey: 'psiTee',
    descKey: 'psiTee',
    computeModes: ['TEE'],
    category: 'basic',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const receiverNode = createReadDataNode(graphId, 1, configs.receiverTableId, {
      x: -390,
      y: -210,
      partition: configs.receiverPartition,
      label: '样本表',
    });
    const senderNode = createReadDataNode(graphId, 2, configs.senderTableId, {
      x: -150,
      y: -210,
      partition: configs.senderPartition,
      label: '样本表',
    });

    const receiverOut = `${graphId}-node-1-output-0`;
    const senderOut = `${graphId}-node-2-output-0`;

    const psiNode = createTeePsiNode(graphId, 3, [receiverOut, senderOut], {
      receiverKey: configs.receiverKey || '',
      senderKey: configs.senderKey || '',
      x: -260,
      y: -100,
    });

    const psiOut = `${graphId}-node-3-output-0`;
    const statsNode = createTableStatisticsNode(graphId, 4, psiOut, {
      x: -260,
      y: 20,
      version: '0.0.1',
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
