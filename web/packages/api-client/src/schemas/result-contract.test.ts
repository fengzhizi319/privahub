import { describe, expect, it } from 'vitest';
import { NodeResultDetailVOSchema } from './index';

// Real PSI output shape: source nodes have nil Go input slices and sf.report
// carries an array, whereas older graph endpoints may wrap tabs in an object.
const wireResult = (tabs: unknown) => ({
  graphDetailVO: { graphId: 'g1', nodes: [
    { graphNodeId: 'read-alice', inputs: null, outputs: ['alice-table'] },
    { graphNodeId: 'psi', inputs: ['alice-table', 'bob-table'], outputs: null },
  ] },
  output: { type: 'report', codeName: 'data_prep/psi', tabs },
});
const tabs = [{ name: 'intersection', divs: [] }];

describe('real node/result/detail contract', () => {
  it('normalizes nil source inputs and retains array report tabs', () => {
    const detail = NodeResultDetailVOSchema.parse(wireResult(tabs));
    expect(detail.graphDetailVO?.nodes?.[0].inputs).toEqual([]);
    expect(detail.graphDetailVO?.nodes?.[1].outputs).toEqual([]);
    expect(detail.output?.tabs).toEqual(tabs);
  });
  it('keeps the legacy object report representation compatible', () => {
    expect(NodeResultDetailVOSchema.parse(wireResult({ tabs })).output?.tabs).toEqual({ tabs });
  });
  it('rejects malformed graph inputs and scalar report payloads', () => {
    expect(NodeResultDetailVOSchema.safeParse(wireResult('broken')).success).toBe(false);
    const malformed = wireResult(tabs);
    malformed.graphDetailVO.nodes[0].inputs = [42] as unknown as string[];
    expect(NodeResultDetailVOSchema.safeParse(malformed).success).toBe(false);
  });
});
