/** Two-party vertical modeling presets. Keep algorithm differences here rather
 * than duplicating graph wiring. Data stays with its owner; only predictions are
 * revealed to the selected receiver. Training options remain editable on canvas.
 */
import type { RiskTemplateConfig, TemplateContribution } from '../types';
import { labelOf, toList } from '../types';
import { connect, createNode, createReadDataNode, outputAnchor } from '../builder';

type Preset = {
  key: string;
  algorithm: string;
  version?: string;
  trainOutputs: number;
  attrs: Record<string, string | number>;
};
export const modelingPresets: Preset[] = [
  { key: 'logisticRegression', algorithm: 'ss_sgd', trainOutputs: 2, attrs: { epochs: 10, batch_size: 128, reg_type: 'logistic' } },
  { key: 'ssGlm', algorithm: 'ss_glm', version: '1.1.0', trainOutputs: 2, attrs: { epochs: 10, batch_size: 128, link_type: 'Logit', label_dist_type: 'Bernoulli', optimizer: 'SGD' } },
  { key: 'secureBoost', algorithm: 'sgb', version: '1.1.0', trainOutputs: 2, attrs: { num_boost_round: 10, max_depth: 3 } },
  { key: 'ssXgb', algorithm: 'ss_xgb', trainOutputs: 1, attrs: { num_boost_round: 10, max_depth: 3 } },
  { key: 'naiveBayes', algorithm: 'gnb', trainOutputs: 1, attrs: {} },
  { key: 'gaussianProcess', algorithm: 'gpc', trainOutputs: 1, attrs: { max_iter_predict: 3 } },
  { key: 'knn', algorithm: 'knn', trainOutputs: 1, attrs: { weights: 'uniform', n_neighbors: 5 } },
  { key: 'kmeans', algorithm: 'kmeans', trainOutputs: 1, attrs: { n_clusters: 2, max_iter: 10, n_init: 1, init_method: 'random' } },
];
export const isModelingKey = (key: string): boolean => modelingPresets.some(p => p.key === key);

/** Canonical Attribute one-of representation; column selectors use string lists
 * even when their cardinality is one (the current SecretFlow component API).
 */
function attribute(value: string | number | boolean | string[]): Record<string, unknown> {
  if (Array.isArray(value)) return { ss: value, is_na: false };
  if (typeof value === 'boolean') return { b: value, is_na: false };
  if (typeof value === 'number') return { i64: value, is_na: false };
  return { s: value, is_na: false };
}

export const modelingTemplates: TemplateContribution<RiskTemplateConfig>[] = modelingPresets.map(preset => ({
  metadata: { key: preset.key, nameKey: preset.key, descKey: preset.key, category: 'ml', computeModes: ['MPC'] },
  build({ graphId, configs }) {
    const out = (idx: number, port = 0) => outputAnchor(graphId, idx, port);
    const clustering = preset.key === 'kmeans';
    const label = labelOf(configs.labelSelects);
    // Never let selected labels or join identifiers silently leak into features.
    const identifiers = [...toList(configs.receiverKey), ...toList(configs.senderKey)];
    const features = (configs.featureSelects?.ss ?? []).filter(c => c !== label && !identifiers.includes(c));
    const pred = configs.pred?.s || 'pred';
    const make = (idx: number, domain: string, name: string, inputs: string[], count: number,
      attrs: Record<string, string | number | boolean | string[]>, version = '1.0.0') => createNode(graphId, idx, `${domain}/${name}`, name, {
      x: 280, y: (idx - 2) * 160, inputs, outputs: Array.from({ length: count }, (_, i) => out(idx, i)),
      nodeDef: { domain, name, version, attrPaths: Object.keys(attrs), attrs: Object.values(attrs).map(attribute) },
    });
    const nodes = [
      createReadDataNode(graphId, 1, configs.receiverTableId, { x: 80, y: 0, partition: configs.receiverPartition }),
      createReadDataNode(graphId, 2, configs.senderTableId, { x: 480, y: 0, partition: configs.senderPartition }),
      make(3, 'data_prep', 'psi', [out(1), out(2)], 2, {
        'input/input_ds1/input_ds1_keys': toList(configs.receiverKey),
        'input/input_ds2/input_ds2_keys': toList(configs.senderKey), protocol: 'PROTOCOL_RR22', join_type: 'inner_join',
      }),
      // Split after alignment to prevent evaluation on the training samples.
      make(4, 'data_prep', 'train_test_split', [out(3)], 2, {}),
      make(5, 'ml.train', `${preset.algorithm}_train`, [out(4)], preset.trainOutputs, {
        ...preset.attrs, ...(features.length ? { 'input/input_ds/feature_selects': features } : {}),
        ...(!clustering && label ? { 'input/input_ds/label': [label] } : {}),
      }, preset.version),
      make(6, 'ml.predict', `${preset.algorithm}_predict`, [out(5), out(4, 1)], 1, {
        receiver: [configs.predictReceiver || configs.receiverNodeId || ''].filter(Boolean), pred_name: pred,
        ...(!clustering ? { save_label: true } : {}),
      }, preset.algorithm === 'ss_glm' ? '1.1.0' : '1.0.0'),
    ];
    const edges = [connect(graphId, 1, 0, 3, 0), connect(graphId, 2, 0, 3, 1), connect(graphId, 3, 0, 4, 0),
      connect(graphId, 4, 0, 5, 0), connect(graphId, 5, 0, 6, 0), connect(graphId, 4, 1, 6, 1)];
    if (!clustering) {
      nodes.push(make(7, 'ml.eval', 'biclassification_eval', [out(6)], 1, {
        ...(label ? { 'input/input_ds/label': [label] } : {}), 'input/input_ds/prediction': [pred],
        ...(preset.algorithm === 'gpc' ? { bucket_size: 2 } : {}),
      }));
      edges.push(connect(graphId, 6, 0, 7, 0));
    }
    return { nodes, edges };
  },
}));
