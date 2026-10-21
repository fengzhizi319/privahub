import { describe, expect, it } from 'vitest';
import {
  autoMatchFeatures,
  availableOnlineFeatures,
  buildPartyConfigs,
  canPublish,
  matchStatus,
  mockMatchFeatures,
  setFeatureMapping,
  validateResource,
} from './feature-mapping';

describe('feature auto mapping', () => {
  it('matches same-name online features and leaves the rest unmatched', () => {
    expect(autoMatchFeatures(['age', 'income', 'x1'], ['income', 'age', 'other'])).toEqual([
      { into: 'age', online: 'age' },
      { into: 'income', online: 'income' },
      { into: 'x1', online: undefined },
    ]);
  });

  it('mock service maps every feature to itself', () => {
    expect(mockMatchFeatures(['a', 'b'])).toEqual([
      { into: 'a', online: 'a' },
      { into: 'b', online: 'b' },
    ]);
  });

  it('match status and manual mapping', () => {
    let items = autoMatchFeatures(['a', 'b'], ['a', 'c']);
    expect(matchStatus('n1', 't1', items)).toBe('error');
    expect(matchStatus(undefined, 't1', items)).toBe('default');
    expect(availableOnlineFeatures(['a', 'c'], items)).toEqual(['c']);
    items = setFeatureMapping(items, 'b', 'c');
    expect(matchStatus('n1', 't1', items)).toBe('success');
  });

  it('builds legacy partyConfigs', () => {
    const rows = [
      { nodeId: 'alice', featureTableId: 'ft1', items: [{ into: 'a', online: 'oa' }] },
      { nodeId: 'bob', featureTableId: 'mock', items: [{ into: 'b', online: 'b' }] },
    ];
    expect(canPublish(rows)).toBe(true);
    expect(
      buildPartyConfigs(rows, { alice: { minCpu: 1, maxCpu: 2, minMemory: 1, maxMemory: 4 } }),
    ).toEqual([
      {
        nodeId: 'alice',
        featureTableId: 'ft1',
        isMock: false,
        features: [{ offlineName: 'a', onlineName: 'oa' }],
        resources: [{ minCPU: '1', maxCPU: '2', minMemory: '1Gi', maxMemory: '4Gi' }],
      },
      {
        nodeId: 'bob',
        featureTableId: 'mock',
        isMock: true,
        features: [{ offlineName: 'b', onlineName: 'b' }],
        resources: [{ minCPU: '0', maxCPU: '0', minMemory: '0Gi', maxMemory: '0Gi' }],
      },
    ]);
  });

  it('cannot publish with unmatched rows or no rows', () => {
    expect(canPublish([])).toBe(false);
    expect(canPublish([{ nodeId: 'a', featureTableId: 't', items: [{ into: 'x' }] }])).toBe(false);
  });

  it('validates resources', () => {
    expect(validateResource({ minCpu: 1, maxCpu: 2, minMemory: 1, maxMemory: 2 })).toBeNull();
    expect(validateResource({ minCpu: 3, maxCpu: 2, minMemory: 1, maxMemory: 2 })).toBe('cpu');
    expect(validateResource({ minCpu: 1, maxCpu: 2, minMemory: 1, maxMemory: 20000 })).toBe('memory');
  });
});
