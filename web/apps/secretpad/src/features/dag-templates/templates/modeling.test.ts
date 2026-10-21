import { describe, expect, it } from 'vitest';
import { modelingPresets, modelingTemplates } from './modeling';
import { isTwoTableTemplate, needsFeatureColumns, needsLabelColumn, needsPredictionName } from '../registry';
import { buildTemplateConfigs, templatesForMode } from '../../create-project/templates';

describe('predefined secure modeling canvases', () => {
  it.each(modelingPresets)('$key has a connected, held-out prediction graph with correct schema', preset => {
    const template = modelingTemplates.find(t => t.metadata.key === preset.key)!;
    const { nodes, edges } = template.build({ graphId: 'g', configs: {
      receiverNodeId: 'alice', senderNodeId: 'bob', receiverTableId: 'a', senderTableId: 'b',
      receiverKey: 'id_a', senderKey: 'id_b', featureSelects: { ss: ['id_a', 'a1', 'b1', 'y'] },
      labelSelects: { s: 'y' }, pred: { s: 'score' },
    } });
    const train = nodes.find(n => n.codeName === `ml.train/${preset.algorithm}_train`)!;
    const predict = nodes.find(n => n.codeName === `ml.predict/${preset.algorithm}_predict`)!;
    const def = train.nodeDef as { attrPaths: string[]; attrs: { ss?: string[] }[] };
    expect(def.attrs[def.attrPaths.indexOf('input/input_ds/feature_selects')]).toEqual({ ss: ['a1','b1'], is_na: false });
    expect(train.inputs).toEqual(['g-node-4-output-0']);
    expect(predict.inputs).toEqual(['g-node-5-output-0','g-node-4-output-1']);
    expect(train.outputs).toHaveLength(preset.trainOutputs);
    for (const edge of edges) {
      expect(nodes.find(n => n.graphNodeId === edge.source)?.outputs).toContain(edge.sourceAnchor);
      expect(nodes.some(n => n.graphNodeId === edge.target)).toBe(true);
    }
    const evalNodes = nodes.filter(n => n.codeName?.startsWith('ml.eval/'));
    expect(evalNodes).toHaveLength(preset.key === 'kmeans' ? 0 : 1);
    expect(def.attrPaths.includes('input/input_ds/label')).toBe(preset.key !== 'kmeans');
  });
  it.each(modelingPresets)('$key is available in both wizards, with label-free clustering', preset => {
    const template = modelingTemplates.find(t => t.metadata.key === preset.key)!;
    expect(isTwoTableTemplate(template)).toBe(true);
    expect(needsFeatureColumns(template)).toBe(true);
    expect(needsPredictionName(template)).toBe(true);
    expect(needsLabelColumn(template)).toBe(preset.key !== 'kmeans');
    expect(templatesForMode('MPC').some(t => t.metadata.key === preset.key)).toBe(true);
    expect(templatesForMode('TEE').some(t => t.metadata.key === preset.key)).toBe(false);
    const configs = buildTemplateConfigs(preset.key, [{ nodeId: 'alice', datatableId: 'a' }, { nodeId: 'bob', datatableId: 'b' }]);
    expect(configs).toMatchObject({ receiverTableId: 'a', senderTableId: 'b', pred: { s: 'pred' } });
    if (preset.key === 'kmeans') expect(configs.labelSelects).toBeUndefined();
    else expect(configs.labelSelects).toEqual({ s: 'y' });
  });
});
