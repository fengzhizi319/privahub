/**
 * 查询混淆模板。
 *
 * 旧前端对应 `pipeline-template-query-obfuscation.ts`：
 * 单节点直接生成 privacy/query_obfuscation，无需数据表输入。
 * 该模板 argsFilled=true，用户只需确认创建即可。
 */
import type { TemplateBuildResult, TemplateContribution } from '../types';
import { createNode, i64Attr, jsonAttr, sAttr } from '../builder';

export const queryObfuscationTemplate: TemplateContribution = {
  metadata: {
    key: 'queryObfuscation',
    nameKey: 'queryObfuscation',
    descKey: 'queryObfuscation',
    computeModes: ['MPC', 'TEE'],
    category: 'privacy',
  },
  build({ graphId }): TemplateBuildResult {
    const node = createNode(graphId, 1, 'privacy/query_obfuscation', '查询混淆', {
      x: -260,
      y: -80,
      outputs: [`${graphId}-node-1-output-0`],
      nodeDef: {
        domain: 'privacy',
        name: 'query_obfuscation',
        version: '1.1.0',
        attrPaths: [
          'op',
          'query',
          'queries_json',
          'synonym_map_json',
          'num_dummies',
          'random_state',
          'domain',
          'medical_pool_json',
          'generic_pool_json',
        ],
        attrs: [
          sAttr('batch'),
          { s: '', is_na: true },
          jsonAttr([
            '患者张三患有艾滋病，如何查询相关诊疗方案',
            '患者李四患有高血压，如何查询相关诊疗方案',
          ]),
          jsonAttr({}),
          i64Attr(3),
          i64Attr(42),
          sAttr('medical'),
          jsonAttr([]),
          jsonAttr([]),
        ],
      },
    });
    return { nodes: [node], edges: [] };
  },
};
