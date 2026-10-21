import { describe, expect, it } from 'vitest';
import { authorizedNodeIds, guideNodes } from './guide-node.logic';

describe('guide node logic', () => {
  it('counts distinct authorized destination nodes', () => {
    const routes = [
      { srcNodeId: 'alice', dstNodeId: 'bob' },
      { srcNodeId: 'alice', dstNodeId: 'bob' },
      { srcNodeId: 'alice', dstNodeId: 'carol' },
      { srcNodeId: 'bob', dstNodeId: 'alice' },
    ];
    expect(authorizedNodeIds(routes, 'alice')).toEqual(['bob', 'carol']);
    expect(authorizedNodeIds(undefined, 'alice')).toEqual([]);
  });
  it('prefers embedded nodes and excludes tee', () => {
    const nodes = [
      { nodeId: 'alice', type: 'embedded' },
      { nodeId: 'tee', type: 'embedded' },
      { nodeId: 'n1', type: 'normal' },
    ];
    expect(guideNodes(nodes).map((n) => n.nodeId)).toEqual(['alice']);
    expect(guideNodes([{ nodeId: 'n1' }, { nodeId: 'tee' }]).map((n) => n.nodeId)).toEqual(['n1']);
  });
});
