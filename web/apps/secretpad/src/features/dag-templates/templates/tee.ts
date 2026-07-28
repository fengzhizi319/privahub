/**
 * TEE 二分类建模模板。
 *
 * 旧前端对应 `pipeline-template-tee.ts`：
 * 1. read_data/datatable × 2（接收方 / 发送方样本表）
 * 2. preprocessing/psi 隐私求交（TEE 版本使用 `preprocessing/psi`）
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
 * 与 MPC Risk 模板的关键差异：
 * - PSI 组件域为 `preprocessing/psi`，版本 `0.0.1`。
 * - WOE 分箱/转换使用 `feature/vert_woe_*` 而非 `preprocessing/vert_woe_*`。
 * - LR 训练/预测使用 `ml.train/lr_train` 与 `ml.predict/lr_predict`。
 * - 需要单独的 WOE 转换节点分别处理训练集与测试集。
 */
import type { TeeTemplateConfig, TemplateBuildResult, TemplateContribution } from '../types';
import { connect, createNode, createReadDataNode, sAttr, ssAttr } from '../builder';

/**
 * 构造 TEE 二分类建模 DAG。
 *
 * 参数说明：
 * - receiver/sender 相关字段：双表输入。
 * - featureSelects: 特征列列表。
 * - labelSelect: 单个标签列。
 */
export const teeTemplate: TemplateContribution<TeeTemplateConfig> = {
  metadata: {
    key: 'tee',
    nameKey: 'tee',
    descKey: 'tee',
    computeModes: ['TEE'],
    category: 'ml',
  },
  build({ graphId, configs }): TemplateBuildResult {
    const featureSelects = configs.featureSelects?.ss ?? [];
    const labelName = configs.labelSelects?.s ?? '';
    const hasFeature = featureSelects.length > 0;
    const hasLabel = Boolean(labelName);

    // 1 ~ 2: 两表读取
    const receiverRead = createReadDataNode(graphId, 1, configs.receiverTableId, {
      x: -380,
      y: -180,
      label: '样本表',
    });
    const senderRead = createReadDataNode(graphId, 2, configs.senderTableId, {
      x: -160,
      y: -180,
      label: '样本表',
    });

    // 3: TEE PSI 求交（preprocessing/psi）
    const psi = createNode(graphId, 3, 'preprocessing/psi', '隐私求交', {
      x: -270,
      y: -90,
      inputs: [`${graphId}-node-1-output-0`, `${graphId}-node-2-output-0`],
      outputs: [`${graphId}-node-3-output-0`],
      nodeDef: {
        ...(configs.receiverKey && configs.senderKey
          ? {
              attrPaths: ['input/input1/key', 'input/input2/key'],
              attrs: [sAttr(configs.receiverKey), sAttr(configs.senderKey)],
            }
          : {}),
        domain: 'preprocessing',
        name: 'psi',
        version: '0.0.1',
      },
    });

    // 4: 全表统计
    const tableStats = createNode(graphId, 4, 'stats/table_statistics', '全表统计', {
      x: -470,
      y: 10,
      inputs: [`${graphId}-node-3-output-0`],
      outputs: [`${graphId}-node-4-output-0`],
      nodeDef: { domain: 'stats', name: 'table_statistics', version: '0.0.1' },
    });

    // 5: 随机分割
    const split = createNode(graphId, 5, 'preprocessing/train_test_split', '随机分割', {
      x: -160,
      y: 10,
      inputs: [`${graphId}-node-3-output-0`],
      outputs: [`${graphId}-node-5-output-0`, `${graphId}-node-5-output-1`],
      nodeDef: { domain: 'preprocessing', name: 'train_test_split', version: '0.0.1' },
    });

    // 6: WOE 分箱
    const woeBinning = createNode(graphId, 6, 'feature/vert_woe_binning', 'WOE分箱', {
      x: -140,
      y: 120,
      inputs: [`${graphId}-node-5-output-0`],
      outputs: [`${graphId}-node-6-output-0`],
      nodeDef: {
        ...(hasFeature && hasLabel
          ? {
              attrPaths: ['input/input_data/feature_selects', 'input/input_data/label'],
              attrs: [ssAttr(featureSelects), sAttr(labelName)],
            }
          : {}),
        domain: 'feature',
        name: 'vert_woe_binning',
        version: '0.0.1',
      },
    });

    // 7: WOE 转换（训练集）
    const woeSubstTrain = createNode(graphId, 7, 'feature/vert_woe_substitution', 'WOE转换', {
      x: -410,
      y: 200,
      inputs: [`${graphId}-node-5-output-0`, `${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-7-output-0`],
      nodeDef: { domain: 'feature', name: 'vert_woe_substitution', version: '0.0.1' },
    });

    // 8: 相关系数矩阵
    const pearson = createNode(graphId, 8, 'stats/pearsonr', '相关系数矩阵', {
      x: -540,
      y: 320,
      inputs: [`${graphId}-node-7-output-0`],
      outputs: [`${graphId}-node-8-output-0`],
      nodeDef: {
        ...(hasFeature
          ? {
              attrPaths: ['input/input_data/feature_selects'],
              attrs: [ssAttr(featureSelects)],
            }
          : {}),
        domain: 'stats',
        name: 'pearsonr',
        version: '0.0.1',
      },
    });

    // 9: VIF 指标计算
    const vif = createNode(graphId, 9, 'stats/vif', 'VIF指标计算', {
      x: -280,
      y: 320,
      inputs: [`${graphId}-node-7-output-0`],
      outputs: [`${graphId}-node-9-output-0`],
      nodeDef: {
        ...(hasFeature
          ? {
              attrPaths: ['input/input_data/feature_selects'],
              attrs: [ssAttr(featureSelects)],
            }
          : {}),
        domain: 'stats',
        name: 'vif',
        version: '0.0.1',
      },
    });

    // 10: LR 训练
    const train = createNode(graphId, 10, 'ml.train/lr_train', 'LR训练', {
      x: -60,
      y: 320,
      inputs: [`${graphId}-node-7-output-0`],
      outputs: [`${graphId}-node-10-output-0`],
      nodeDef: {
        ...(hasLabel
          ? {
              attrPaths: ['input/train_dataset/ids', 'input/train_dataset/label'],
              attrs: [{ ss: [], is_na: true }, sAttr(labelName)],
            }
          : {}),
        domain: 'ml.train',
        name: 'lr_train',
        version: '0.0.1',
      },
    });

    // 12: WOE 转换（测试集）
    const woeSubstTest = createNode(graphId, 12, 'feature/vert_woe_substitution', 'WOE转换', {
      x: -60,
      y: 200,
      inputs: [`${graphId}-node-5-output-1`, `${graphId}-node-6-output-0`],
      outputs: [`${graphId}-node-12-output-0`],
      nodeDef: { domain: 'feature', name: 'vert_woe_substitution', version: '0.0.1' },
    });

    // 11: LR 预测（编号保持与旧模板一致，位于训练节点之后）
    const predict = createNode(graphId, 11, 'ml.predict/lr_predict', 'LR预测', {
      x: -40,
      y: 390,
      inputs: [`${graphId}-node-12-output-0`, `${graphId}-node-10-output-0`],
      outputs: [`${graphId}-node-11-output-0`],
      nodeDef: {
        ...(hasLabel
          ? {
              attrPaths: [
                'input/feature_dataset/ids',
                'input/feature_dataset/label',
                'save_id',
                'save_label',
              ],
              attrs: [
                { ss: [], is_na: true },
                sAttr(labelName),
                { b: true, is_na: false },
                { b: true, is_na: false },
              ],
            }
          : {}),
        domain: 'ml.predict',
        name: 'lr_predict',
        version: '0.0.1',
      },
    });

    // 13: 二分类评估
    const biEval = createNode(graphId, 13, 'ml.eval/biclassification_eval', '二分类评估', {
      x: -40,
      y: 490,
      inputs: [`${graphId}-node-11-output-0`],
      outputs: [`${graphId}-node-13-output-0`],
      nodeDef: {
        ...(hasLabel
          ? {
              attrPaths: ['input/predictions/label', 'input/predictions/score'],
              attrs: [sAttr(labelName), sAttr(labelName)],
            }
          : {}),
        domain: 'ml.eval',
        name: 'biclassification_eval',
        version: '0.0.1',
      },
    });

    // 14: 预测偏差评估
    const biasEval = createNode(graphId, 14, 'ml.eval/prediction_bias_eval', '预测偏差评估', {
      x: -270,
      y: 490,
      inputs: [`${graphId}-node-11-output-0`],
      outputs: [`${graphId}-node-14-output-0`],
      nodeDef: {
        ...(hasLabel
          ? {
              attrPaths: ['input/predictions/label', 'input/predictions/score'],
              attrs: [sAttr(labelName), sAttr(labelName)],
            }
          : {}),
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
      connect(graphId, 5, 1, 7, 0),
      connect(graphId, 6, 0, 7, 1),
      connect(graphId, 7, 0, 8, 0),
      connect(graphId, 7, 0, 9, 0),
      connect(graphId, 7, 0, 10, 0),
      connect(graphId, 6, 0, 12, 1),
      connect(graphId, 5, 1, 12, 0),
      connect(graphId, 12, 0, 11, 0),
      connect(graphId, 10, 0, 11, 1),
      connect(graphId, 11, 0, 13, 0),
      connect(graphId, 11, 0, 14, 0),
    ];

    return { nodes, edges };
  },
};
