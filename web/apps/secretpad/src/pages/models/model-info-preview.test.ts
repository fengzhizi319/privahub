import { describe, expect, it } from 'vitest';
import { modelInfoPreview } from './model-info-preview';

const graph = {
  graphId: 'g1',
  name: 'train',
  nodes: [
    { graphNodeId: 'g1-node-1', codeName: 'read_data/datatable', label: 'read', x: 0, y: 0 },
    { graphNodeId: 'g1-node-2', codeName: 'ml.train/ss_sgd_train', label: 'sgd', x: 0, y: 100 },
    { graphNodeId: 'g1-node-3', codeName: 'stats/table_statistics', label: 'stats', x: 200, y: 100 },
  ],
  edges: [
    { edgeId: 'e1', source: 'g1-node-1', sourceAnchor: 'g1-node-1-output-0', target: 'g1-node-2', targetAnchor: 'g1-node-2-input-0' },
    { edgeId: 'e2', source: 'g1-node-1', sourceAnchor: 'g1-node-1-output-0', target: 'g1-node-3', targetAnchor: 'g1-node-3-input-0' },
  ],
};

describe('modelInfoPreview', () => {
  it('maps graphDetailVO and highlights modelGraphDetail nodes', () => {
    const p = modelInfoPreview({
      graphDetailVO: graph,
      modelGraphDetail: ['g1-node-1', 'g1-node-2'],
      servingDetails: [{ nodeId: 'alice', nodeName: 'Alice', sourcePath: '/models/a' }],
    });
    expect(p.graphName).toBe('train');
    expect(p.nodes.map((n) => n.id)).toEqual(['g1-node-1', 'g1-node-2', 'g1-node-3']);
    expect(p.edges).toHaveLength(2);
    expect([...p.highlight].sort()).toEqual(['g1-node-1', 'g1-node-2']);
    expect(p.modelPaths).toEqual([{ nodeId: 'alice', nodeName: 'Alice', sourcePath: '/models/a' }]);
  });

  it('highlights everything when modelGraphDetail misses and tolerates empty info', () => {
    const p = modelInfoPreview({ graphDetailVO: graph, modelGraphDetail: ['nope'] });
    expect(p.highlight.size).toBe(3);
    const empty = modelInfoPreview(null);
    expect(empty.nodes).toEqual([]);
    expect(empty.modelPaths).toEqual([]);
  });
});
