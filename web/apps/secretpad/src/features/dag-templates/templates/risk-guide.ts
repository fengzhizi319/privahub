/**
 * 风险建模引导式模板（MPC 二分类）。
 *
 * 旧前端对应 `pipeline-template-risk-guide.ts`：
 * 1. read_data/datatable × 2（占位 alice-table / bob-table）
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
 * 该引导式模板使用占位数据与默认参数，无需用户在向导中填写参数。
 */
import type { GuideTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import {
  bAttr,
  connect,
  createNode,
  createPsiNode,
  createReadDataNode,
  createTableStatisticsNode,
  createTrainTestSplitNode,
  fAttr,
  i64Attr,
  naAttr,
  sAttr,
  ssAttr,
} from '../builder';

export const riskGuideTemplate: TemplateContribution<GuideTemplateConfig> = {
  metadata: {
    key: 'riskGuide',
    nameKey: 'riskGuide',
    descKey: 'riskGuide',
    computeModes: ['MPC'],
    category: 'guide',
  },
  build({ graphId }): TemplateBuildResult {
    // 1 ~ 2: 两表读取
    const receiverRead = createReadDataNode(graphId, 1, 'alice-table', {
      x: -370,
      y: -250,
      label: '样本表',
    });
    const senderRead = createReadDataNode(graphId, 2, 'bob-table', {
      x: -140,
      y: -250,
      label: '样本表',
    });

    // 3: PSI 求交
    const receiverOut = `${graphId}-node-1-output-0`;
    const senderOut = `${graphId}-node-2-output-0`;
    const psi = createPsiNode(graphId, 3, [receiverOut, senderOut], {
      receiverKey: 'id1',
      senderKey: 'id2',
      receiverNodeId: 'alice',
      senderNodeId: 'bob',
      x: -240,
      y: -160,
    });

    // 4: 全表统计
    const psiOut = `${graphId}-node-3-output-0`;
    const tableStats = createTableStatisticsNode(graphId, 4, psiOut, {
      x: -430,
      y: -90,
      features: ['y', 'age', 'education', 'default'],
      version: '1.0.0',
    });

    // 5: 随机分割
    const split = createTrainTestSplitNode(graphId, 5, psiOut, {
      x: -120,
      y: -80,
      version: '1.0.0',
    });

    // 6: WOE 分箱
    const splitTrainOut = `${graphId}-node-5-output-0`;
    const splitTestOut = `${graphId}-node-5-output-1`;
    const woeBinning = createNode(graphId, 6, 'preprocessing/vert_woe_binning', 'WOE分箱', {
      x: -140,
      y: 20,
      inputs: [splitTrainOut],
      outputs: [
        `${graphId}-node-6-output-0`,
        `${graphId}-node-6-output-1`,
        `${graphId}-node-6-output-2`,
      ],
      nodeDef: {
        attrPaths: [
          'input/input_ds/feature_selects',
          'input/input_ds/label',
          'secure_device_type',
          'binning_method',
          'bin_num',
          'positive_label',
          'chimerge_init_bins',
          'chimerge_target_bins',
          'chimerge_target_pvalue',
          'report_rules',
        ],
        attrs: [
          ssAttr(['duration']),
          ssAttr(['y']),
          sAttr('spu'),
          sAttr('quantile'),
          i64Attr(10),
          sAttr('1'),
          i64Attr(100),
          i64Attr(10),
          fAttr(0.1),
          naAttr(),
        ],
        domain: 'preprocessing',
        name: 'vert_woe_binning',
        version: '1.0.0',
      },
    });

    // 8: 特征工程应用（substitution）
    // 注：旧模板节点编号跳过 7，此处保持编号一致。
    const woeBinningSubstOut = `${graphId}-node-6-output-1`;
    const substitution = createNode(graphId, 8, 'preprocessing/substitution', '特征工程应用', {
      x: -10,
      y: 100,
      inputs: [splitTestOut, woeBinningSubstOut],
      outputs: [`${graphId}-node-8-output-0`],
      nodeDef: { domain: 'preprocessing', name: 'substitution', version: '1.0.0' },
    });

    // 9: 相关系数矩阵
    const woeBinningOut = `${graphId}-node-6-output-0`;
    const pearson = createNode(graphId, 9, 'stats/ss_pearsonr', '相关系数矩阵', {
      x: -450,
      y: 190,
      inputs: [woeBinningOut],
      outputs: [`${graphId}-node-9-output-0`],
      nodeDef: {
        attrPaths: ['input/input_ds/feature_selects'],
        attrs: [ssAttr(['contact_cellular'])],
        domain: 'stats',
        name: 'ss_pearsonr',
        version: '1.0.0',
      },
    });

    // 10: VIF 指标计算
    const vif = createNode(graphId, 10, 'stats/ss_vif', 'VIF指标计算', {
      x: -240,
      y: 190,
      inputs: [woeBinningOut],
      outputs: [`${graphId}-node-10-output-0`],
      nodeDef: {
        attrPaths: ['input/input_ds/feature_selects'],
        attrs: [ssAttr(['duration'])],
        domain: 'stats',
        name: 'ss_vif',
        version: '1.0.0',
      },
    });

    // 11: 逻辑回归训练
    const train = createNode(graphId, 11, 'ml.train/ss_sgd_train', '逻辑回归训练', {
      x: -40,
      y: 220,
      inputs: [woeBinningOut],
      outputs: [`${graphId}-node-11-output-0`, `${graphId}-node-11-output-1`],
      nodeDef: {
        attrPaths: [
          'input/input_ds/feature_selects',
          'input/input_ds/label',
          'epochs',
          'learning_rate',
          'batch_size',
          'sig_type',
          'reg_type',
          'penalty',
          'l2_norm',
          'eps',
        ],
        attrs: [
          ssAttr(['duration']),
          ssAttr(['y']),
          i64Attr(10),
          fAttr(0.1),
          i64Attr(1024),
          sAttr('t1'),
          sAttr('logistic'),
          sAttr('None'),
          fAttr(0.5),
          fAttr(0.001),
        ],
        domain: 'ml.train',
        name: 'ss_sgd_train',
        version: '1.0.0',
      },
    });

    // 12: P-VALUE 评估
    const trainOut = `${graphId}-node-11-output-0`;
    const pvalue = createNode(graphId, 12, 'ml.eval/ss_pvalue', 'P-VALUE评估', {
      x: -250,
      y: 310,
      inputs: [trainOut, woeBinningOut],
      outputs: [`${graphId}-node-12-output-0`],
      nodeDef: { domain: 'ml.eval', name: 'ss_pvalue', version: '1.0.0' },
    });

    // 13: 逻辑回归预测
    const substitutionOut = `${graphId}-node-8-output-0`;
    const predict = createNode(graphId, 13, 'ml.predict/ss_sgd_predict', '逻辑回归预测', {
      x: 40,
      y: 330,
      inputs: [trainOut, substitutionOut],
      outputs: [`${graphId}-node-13-output-0`],
      nodeDef: {
        attrPaths: [
          'input/input_ds/saved_features',
          'batch_size',
          'receiver',
          'pred_name',
          'save_ids',
          'save_label',
        ],
        attrs: [ssAttr(['contact_cellular']), i64Attr(1024), ssAttr(['bob']), sAttr('pred'), bAttr(true), bAttr(true)],
        domain: 'ml.predict',
        name: 'ss_sgd_predict',
        version: '1.0.0',
      },
    });

    // 14: 二分类评估
    const predictOut = `${graphId}-node-13-output-0`;
    const biEval = createNode(graphId, 14, 'ml.eval/biclassification_eval', '二分类评估', {
      x: 130,
      y: 450,
      inputs: [predictOut],
      outputs: [`${graphId}-node-14-output-0`],
      nodeDef: {
        attrPaths: ['input/input_ds/label', 'input/input_ds/prediction'],
        attrs: [ssAttr(['y']), ssAttr(['pred'])],
        domain: 'ml.eval',
        name: 'biclassification_eval',
        version: '1.0.0',
      },
    });

    // 15: 预测偏差评估
    const biasEval = createNode(graphId, 15, 'ml.eval/prediction_bias_eval', '预测偏差评估', {
      x: -110,
      y: 540,
      inputs: [predictOut],
      outputs: [`${graphId}-node-15-output-0`],
      nodeDef: {
        attrPaths: ['input/input_ds/label', 'input/input_ds/prediction'],
        attrs: [ssAttr(['y']), ssAttr(['pred'])],
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
