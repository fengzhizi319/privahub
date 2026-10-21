import { describe, expect, it } from 'vitest';
import { allTemplates } from './registry';

describe('independent predefined canvases', () => {
  it.each(allTemplates)('$metadata.key never shares nodes, definitions or coordinates with another canvas', template => {
    const configs = { receiverNodeId:'alice', senderNodeId:'bob', receiverTableId:'a', senderTableId:'b',
      receiverKey:'id_a', senderKey:'id_b', featureSelects:{ss:['a1','b1']}, labelSelects:{s:'y'}, pred:{s:'pred'},
      tableId:'a', qiCols:['age'], saCols:['diagnosis'], queryCol:'a1', sanitizationCols:['name'] };
    const first = template.build({ graphId:'first', configs });
    const second = template.build({ graphId:'second', configs });
    const snapshot = structuredClone(second);
    for (const node of first.nodes) {
      expect(node.graphNodeId).toMatch(/^first-/);
      for (const other of first.nodes) {
        if (other === node) continue;
        expect(Math.abs(node.x! - other.x!) >= 180 || Math.abs(node.y! - other.y!) >= 140).toBe(true);
      }
      node.x = 9999;
      node.nodeDef = { changed:true };
    }
    expect(second).toEqual(snapshot);
    expect(second.nodes.every(n => n.graphNodeId?.startsWith('second-'))).toBe(true);
    expect(second.edges.every(e => e.source?.startsWith('second-') && e.target?.startsWith('second-'))).toBe(true);
  });
});
