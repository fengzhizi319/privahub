/**
 * 风险建模（二分类）模板。
 *
 * 旧前端对应 `pipeline-template-risk.ts`：
 * 1. read_data/datatable × 2（接收方 / 发送方样本表）
 * 2. data_prep/psi 隐私求交
 * 3. stats/table_statistics 全表统计
 * 4. data_prep/train_test_split 随机分割
 * 5. preprocessing/vert_woe_binning WOE 分箱
 * 6. preprocessing/substitution 特征工程应用
 * 7. stats/ss_pearsonr 相关系数矩阵
 * 8. stats/ss_vif VIF 指标计算
 * 9. ml.train/ss_sgd_train 逻辑回归训练
 * 10. ml.eval/ss_pvalue P-VALUE 评估
 * 11. ml.predict/ss_sgd_predict 逻辑回归预测
 * 12. ml.eval/biclassification_eval 二分类评估
 * 13. ml.eval/prediction_bias_eval 预测偏差评估
 *
 * 新前端保留相同拓扑，但把 nodeDef/edges 构造委托给 `builder.ts`，
 * 仅在此描述业务编排顺序与参数。
 */
import type {
  RiskTemplateConfig,
  TemplateBuildResult,
  TemplateContribution,
  TwoTableTemplateConfig,
} from '../types';
import {
  connect,
  createNode,
  createPsiNode,
  createReadDataNode,
  sAttr,
  ssAttr,
} from '../builder';

/**
 * 构造二分类建模所需的完整 DAG。
 *
 * 参数说明：
 * - receiver/sender 相关字段：双表输入，与 PSI 模板一致。
 * - featureSelects: 特征列列表（`ss` 数组）。
 * - labelSelects: 标签列（`s` 单个）。
 * - pred: 预测结果列名（`s` 单个）。
 */
export const riskTemplate: TemplateContribution<RiskTemplateConfig> = {
  metadata: {
    key: 'risk',
    nameKey: 'risk',
    descKey: 'risk',
    computeModes: ['MPC'],
    category: 'ml',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const { receiverNodeId, senderNodeId, receiverKey, senderKey } = configs as TwoTableTemplateConfig;

    // 1 ~ 2: 两表读取
    const receiverRead = createReadDataNode(graphId, 1, configs.receiverTableId, {
      x: -370,
      y: -250,
      partition: configs.receiverPartition,
      label: '样本表',
    });
    const senderRead = createReadDataNode(graphId, 2, configs.senderTableId, {
      x: -140,
      y: -250,
      partition: configs.senderPartition,
      label: '样本表',
    });

    // 3: PSI 求交
    const psi = createPsiNode(graphId, 3, [
      `${graphId}-node-1-output-0`,
      `${graphId}-node-2-output-0`,
    ], {
      receiverKey: receiverKey || '',
      senderKey: senderKey || '',
      receiverNodeId: receiverNodeId || '',
      senderNodeId: senderNodeId || '',
      x: -240,
      y: -160,
    });

    const featureSelects = configs.featureSelects?.ss ?? [];
    const labelName = configs.labelSelects?.s ?? '';
    const predName = configs.pred?.s ?? 'pred';
    const hasFeature = featureSelects.length > 0;
    const hasLabel = Boolean(labelName);

    // 4: 全表统计
    const tableStats = createNode(graphId, 4, 'stats/table_statistics', '全表统计', {
      x: -430,
      y: -90,
      inputs: [`${graphId}-node-3-output-0`],
      outputs: [`${graphId}-node-4-output-0`],
      nodeDef: {
        ...(hasFeature
          ? {
              attrPaths: ['input/input_ds/features'],
              attrs: [ssAttr(featureSelects)],
            }
          : {}),
        domain: 'stats',
        name: 'table_statistics',
        version: '1.0.0',
      },
    });

    // 5: 随机分割
    const split = createNode(graphId, 5, 'data_prep/train_test_split', '随机分割', {
      x: -120,
      y: -80,
      inputs: [`${graphId}-node-3-output-0`],
      outputs: [`${graphId}-node-5-output-0`, `${graphId}-node-5-output-1`],
      nodeDef: { domain: 'data_prep', name: 'train_test_split', version: '1.0.0' },
    });

    // 6: WOE 分箱
    const woeBinning = createNode(graphId, 6, 'preprocessing/vert_woe_binning', 'WOE分箱', {
      x: -140,
      y: 20,
      inputs: [`${graphId}-node-5-output-0`],
      outputs: [
        `${graphId}-node-6-output-0`,
        `${graphId}-node-6-output-1`,
        `${graphId}-node-6-output-2`,
      ],
      nodeDef: {
        ...(hasFeature && hasLabel
          ? {
              attrPaths: ['input/input_ds/feature_selects', 'input/input_ds/label'],
              attrs: [ssAttr(featureSelects), sAttr(labelName)],
            }
          : {}),
        domain: 'preprocessing',
        name: 'vert_woe_binning',
        version: '1.0.0',
      },
    });

    // 8: 特征工程应用（substitution）
    const substitution = createNode(graphId, 8, 'preprocessing/substitution', '特征工程应用', {
      x: -10,
      y: 100,
      inputs: [`${graphId}-node-5-output-1`, `${graphId}-node-6-output-1`],
      outputs: [`${graphId}-node-8-output-0`],
      nodeDef: { domain: 'preprocessing', name: 'substitution', version: '1.0.0' },
    });

    // 9: 相关系数矩阵
    const pearson = createNode(graphId, 9, 'stats/ss_pearsonr', '相关系数矩阵', {
      x: -450,
      y: 190,
      inputs: [`${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-9-output-0`],
      nodeDef: {
        ...(hasFeature
          ? {
              attrPaths: ['input/input_ds/feature_selects'],
              attrs: [ssAttr(featureSelects)],
            }
          : {}),
        domain: 'stats',
        name: 'ss_pearsonr',
        version: '1.0.0',
      },
    });

    // 10: VIF 指标计算
    const vif = createNode(graphId, 10, 'stats/ss_vif', 'VIF指标计算', {
      x: -240,
      y: 190,
      inputs: [`${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-10-output-0`],
      nodeDef: {
        ...(hasFeature
          ? {
              attrPaths: ['input/input_ds/feature_selects'],
              attrs: [ssAttr(featureSelects)],
            }
          : {}),
        domain: 'stats',
        name: 'ss_vif',
        version: '1.0.0',
      },
    });

    // 11: 逻辑回归训练
    const train = createNode(graphId, 11, 'ml.train/ss_sgd_train', '逻辑回归训练', {
      x: -40,
      y: 220,
      inputs: [`${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-11-output-0`, `${graphId}-node-11-output-1`],
      nodeDef: {
        ...(hasFeature && hasLabel
          ? {
              attrPaths: ['input/input_ds/label', 'input/input_ds/feature_selects'],
              attrs: [sAttr(labelName), ssAttr(featureSelects)],
            }
          : {}),
        domain: 'ml.train',
        name: 'ss_sgd_train',
        version: '1.0.0',
      },
    });

    // 12: P-VALUE 评估
    const pvalue = createNode(graphId, 12, 'ml.eval/ss_pvalue', 'P-VALUE评估', {
      x: -250,
      y: 310,
      inputs: [`${graphId}-node-11-output-0`, `${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-12-output-0`],
      nodeDef: { domain: 'ml.eval', name: 'ss_pvalue', version: '1.0.0' },
    });

    // 13: 逻辑回归预测
    const predict = createNode(graphId, 13, 'ml.predict/ss_sgd_predict', '逻辑回归预测', {
      x: 40,
      y: 330,
      inputs: [`${graphId}-node-11-output-0`, `${graphId}-node-8-output-0`],
      outputs: [`${graphId}-node-13-output-0`],
      nodeDef: {
        ...(receiverNodeId && predName
          ? {
              attrPaths: ['receiver', 'pred_name', 'save_label'],
              attrs: [sAttr(receiverNodeId), sAttr(predName), { b: true, is_na: false }],
            }
          : {}),
        domain: 'ml.predict',
        name: 'ss_sgd_predict',
        version: '1.0.0',
      },
    });

    // 14: 二分类评估
    const biEval = createNode(graphId, 14, 'ml.eval/biclassification_eval', '二分类评估', {
      x: 130,
      y: 450,
      inputs: [`${graphId}-node-13-output-0`],
      outputs: [`${graphId}-node-14-output-0`],
      nodeDef: {
        ...(hasLabel && predName
          ? {
              attrPaths: ['input/input_ds/label', 'input/input_ds/prediction'],
              attrs: [sAttr(labelName), ssAttr([predName])],
            }
          : {}),
        domain: 'ml.eval',
        name: 'biclassification_eval',
        version: '1.0.0',
      },
    });

    // 15: 预测偏差评估
    const biasEval = createNode(graphId, 15, 'ml.eval/prediction_bias_eval', '预测偏差评估', {
      x: -110,
      y: 540,
      inputs: [`${graphId}-node-13-output-0`],
      outputs: [`${graphId}-node-15-output-0`],
      nodeDef: {
        ...(hasLabel && predName
          ? {
              attrPaths: ['input/input_ds/label', 'input/input_ds/prediction'],
              attrs: [sAttr(labelName), ssAttr([predName])],
            }
          : {}),
        domain: 'ml.eval',
        name: 'prediction_bias_eval',
        version: '1.0.0',
      },
    });

    const nodes = [
      receiverRead,
      senderRead,
      psi,
      tableStats,
      split,
      woeBinning,
      substitution,
      pearson,
      vif,
      train,
      pvalue,
      predict,
      biEval,
      biasEval,
    ];

    const edges = [
      connect(graphId, 1, 0, 3, 0),
      connect(graphId, 2, 0, 3, 1),
      connect(graphId, 3, 0, 4, 0),
      connect(graphId, 3, 0, 5, 0),
      connect(graphId, 5, 0, 6, 0),
      connect(graphId, 5, 1, 8, 0),
      connect(graphId, 6, 1, 8, 1),
      connect(graphId, 6, 0, 9, 0),
      connect(graphId, 6, 0, 10, 0),
      connect(graphId, 6, 0, 11, 0),
      connect(graphId, 6, 0, 12, 1),
      connect(graphId, 11, 0, 12, 0),
      connect(graphId, 11, 0, 13, 0),
      connect(graphId, 8, 0, 13, 1),
      connect(graphId, 13, 0, 14, 0),
      connect(graphId, 13, 0, 15, 0),
    ];

    return { nodes, edges };
  },
};
