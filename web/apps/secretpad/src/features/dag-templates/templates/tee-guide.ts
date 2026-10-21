/**
 * TEE 建模引导式模板（二分类）。
 *
 * 旧前端对应 `pipeline-template-tee-guide.ts`：
 * 1. read_data/datatable × 2（占位 alice-table / bob-table）
 * 2. preprocessing/psi 隐私求交
 * 3. stats/table_statistics 全表统计
 * 4. preprocessing/train_test_split 随机分割
 * 5. feature/vert_woe_binning WOE 分箱
 * 6. feature/vert_woe_substitution WOE 转换（训练集）
 * 7. stats/pearsonr 相关系数矩阵
 * 8. stats/vif VIF 指标计算
 * 9. ml.train/lr_train 逻辑回归训练
 * 10. feature/vert_woe_substitution WOE 转换（测试集）
 * 11. ml.predict/lr_predict 逻辑回归预测
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
  createReadDataNode,
  createTableStatisticsNode,
  createTeePsiNode,
  createTrainTestSplitNode,
  fAttr,
  i64Attr,
  sAttr,
  ssAttr,
} from '../builder';

const TEE_WOE_FEATURES = [
  'contact_cellular',
  'contact_telephone',
  'contact_unknown',
  'month_apr',
  'month_aug',
  'month_dec',
  'month_feb',
  'month_jan',
  'month_jul',
  'month_jun',
  'month_mar',
  'month_may',
  'month_nov',
  'month_oct',
  'month_sep',
  'poutcome_failure',
  'poutcome_other',
  'poutcome_success',
  'poutcome_unknown',
  'age',
  'education',
  'default',
  'balance',
  'housing',
  'loan',
  'day',
  'duration',
  'campaign',
  'pdays',
  'previous',
  'job_blue-collar',
  'job_entrepreneur',
  'job_housemaid',
  'job_management',
  'job_retired',
  'job_self-employed',
  'job_services',
  'job_student',
  'job_technician',
  'job_unemployed',
  'marital_divorced',
  'marital_married',
  'marital_single',
];

export const teeGuideTemplate: TemplateContribution<GuideTemplateConfig> = {
  metadata: {
    key: 'teeGuide',
    nameKey: 'teeGuide',
    descKey: 'teeGuide',
    computeModes: ['TEE'],
    category: 'guide',
  },
  build({ graphId }): TemplateBuildResult {
    // 1 ~ 2: 两表读取
    const receiverRead = createReadDataNode(graphId, 1, 'alice-table', {
      x: -380,
      y: -180,
      label: '样本表',
    });
    const senderRead = createReadDataNode(graphId, 2, 'bob-table', {
      x: -160,
      y: -180,
      label: '样本表',
    });

    // 3: TEE PSI 求交
    const receiverOut = `${graphId}-node-1-output-0`;
    const senderOut = `${graphId}-node-2-output-0`;
    const psi = createTeePsiNode(graphId, 3, [receiverOut, senderOut], {
      receiverKey: 'id1',
      senderKey: 'id2',
      x: -270,
      y: -90,
    });

    // 4: 全表统计
    const psiOut = `${graphId}-node-3-output-0`;
    const tableStats = createTableStatisticsNode(graphId, 4, psiOut, {
      x: -470,
      y: 10,
      version: '0.0.1',
    });

    // 5: 随机分割
    const split = createTrainTestSplitNode(graphId, 5, psiOut, {
      x: -160,
      y: 10,
      domain: 'preprocessing',
      version: '0.0.1',
    });

    // 6: WOE 分箱
    const splitTrainOut = `${graphId}-node-5-output-0`;
    const splitTestOut = `${graphId}-node-5-output-1`;
    const woeBinning = createNode(graphId, 6, 'feature/vert_woe_binning', 'WOE分箱', {
      x: -140,
      y: 120,
      inputs: [splitTrainOut],
      outputs: [`${graphId}-node-6-output-0`],
      nodeDef: {
        attrPaths: [
          'input/input_data/feature_selects',
          'input/input_data/label',
          'binning_method',
          'positive_label',
          'bin_num',
        ],
        attrs: [ssAttr(TEE_WOE_FEATURES), ssAttr(['y']), sAttr('quantile'), sAttr('1'), i64Attr(10)],
        domain: 'feature',
        name: 'vert_woe_binning',
        version: '0.0.1',
      },
    });

    // 7: WOE 转换（训练集）
    const woeBinningOut = `${graphId}-node-6-output-0`;
    const woeSubstTrain = createNode(graphId, 7, 'feature/vert_woe_substitution', 'WOE转换', {
      x: -410,
      y: 200,
      inputs: [splitTrainOut, woeBinningOut],
      outputs: [`${graphId}-node-7-output-0`],
      nodeDef: { domain: 'feature', name: 'vert_woe_substitution', version: '0.0.1' },
    });

    // 8: 相关系数矩阵
    const woeTrainOut = `${graphId}-node-7-output-0`;
    const pearson = createNode(graphId, 8, 'stats/pearsonr', '相关系数矩阵', {
      x: -540,
      y: 320,
      inputs: [woeTrainOut],
      outputs: [`${graphId}-node-8-output-0`],
      nodeDef: { domain: 'stats', name: 'pearsonr', version: '0.0.1' },
    });

    // 9: VIF 指标计算
    const vif = createNode(graphId, 9, 'stats/vif', 'VIF指标计算', {
      x: -280,
      y: 320,
      inputs: [woeTrainOut],
      outputs: [`${graphId}-node-9-output-0`],
      nodeDef: { domain: 'stats', name: 'vif', version: '0.0.1' },
    });

    // 10: LR 训练
    const train = createNode(graphId, 10, 'ml.train/lr_train', 'LR训练', {
      x: -60,
      y: 320,
      inputs: [woeTrainOut],
      outputs: [`${graphId}-node-10-output-0`],
      nodeDef: {
        attrPaths: [
          'input/train_dataset/ids',
          'input/train_dataset/label',
          'max_iter',
          'reg_type',
          'l2_norm',
          'tol',
          'penalty',
        ],
        attrs: [ssAttr(['id2', 'id1']), ssAttr(['y']), i64Attr(10), sAttr('logistic'), fAttr(1), fAttr(0.0001), sAttr('l2')],
        domain: 'ml.train',
        name: 'lr_train',
        version: '0.0.1',
      },
    });

    // 12: WOE 转换（测试集）
    const woeSubstTest = createNode(graphId, 12, 'feature/vert_woe_substitution', 'WOE转换', {
      x: -60,
      y: 200,
      inputs: [splitTestOut, woeBinningOut],
      outputs: [`${graphId}-node-12-output-0`],
      nodeDef: { domain: 'feature', name: 'vert_woe_substitution', version: '0.0.1' },
    });

    // 11: LR 预测（编号保持与旧模板一致）
    const woeTestOut = `${graphId}-node-12-output-0`;
    const trainOut = `${graphId}-node-10-output-0`;
    const predict = createNode(graphId, 11, 'ml.predict/lr_predict', 'LR预测', {
      x: -40,
      y: 390,
      inputs: [woeTestOut, trainOut],
      outputs: [`${graphId}-node-11-output-0`],
      nodeDef: {
        attrPaths: [
          'input/feature_dataset/ids',
          'input/feature_dataset/label',
          'pred_name',
          'save_label',
          'label_name',
          'save_id',
          'id_name',
          'col_names',
        ],
        attrs: [
          ssAttr(['id1']),
          ssAttr(['y']),
          sAttr('pred'),
          bAttr(true),
          sAttr('label'),
          bAttr(true),
          sAttr('id'),
          ssAttr([]),
        ],
        domain: 'ml.predict',
        name: 'lr_predict',
        version: '0.0.1',
      },
    });

    // 13: 二分类评估
    const predictOut = `${graphId}-node-11-output-0`;
    const biEval = createNode(graphId, 13, 'ml.eval/biclassification_eval', '二分类评估', {
      x: -40,
      y: 490,
      inputs: [predictOut],
      outputs: [`${graphId}-node-13-output-0`],
      nodeDef: {
        attrPaths: [
          'input/predictions/label',
          'input/predictions/score',
          'bucket_num',
          'min_item_cnt_per_bucket',
        ],
        attrs: [ssAttr(['label']), ssAttr(['pred']), i64Attr(10), i64Attr(2)],
        domain: 'ml.eval',
        name: 'biclassification_eval',
        version: '0.0.1',
      },
    });

    // 14: 预测偏差评估
    const biasEval = createNode(graphId, 14, 'ml.eval/prediction_bias_eval', '预测偏差评估', {
      x: -270,
      y: 490,
      inputs: [predictOut],
      outputs: [`${graphId}-node-14-output-0`],
      nodeDef: {
        attrPaths: [
          'input/predictions/label',
          'input/predictions/score',
          'bucket_num',
          'min_item_cnt_per_bucket',
          'bucket_method',
        ],
        attrs: [ssAttr(['label']), ssAttr(['pred']), i64Attr(10), i64Attr(2), sAttr('equal_width')],
        domain: 'ml.eval',
        name: 'prediction_bias_eval',
        version: '0.0.1',
      },
    });

    const nodes = [
      receiverRead,
      senderRead,
      psi,
      tableStats,
      split,
      woeBinning,
      woeSubstTrain,
      pearson,
      vif,
      train,
      woeSubstTest,
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
      connect(graphId, 5, 0, 7, 0),
      connect(graphId, 6, 0, 7, 1),
      connect(graphId, 7, 0, 8, 0),
      connect(graphId, 7, 0, 9, 0),
      connect(graphId, 7, 0, 10, 0),
      connect(graphId, 10, 0, 11, 1),
      connect(graphId, 6, 0, 12, 1),
      connect(graphId, 5, 1, 12, 0),
      connect(graphId, 12, 0, 11, 0),
      connect(graphId, 11, 0, 13, 0),
      connect(graphId, 11, 0, 14, 0),
    ];

    return { nodes, edges };
  },
};
