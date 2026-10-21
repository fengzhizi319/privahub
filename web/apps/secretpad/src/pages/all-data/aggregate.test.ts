import { describe, expect, it } from 'vitest';
import { aggregateDatasources, aggregateDatatables, filterAggregated, uniqueNodes } from './aggregate';

const nodes = [
  { nodeId: 'alice', nodeName: 'Alice' },
  { nodeId: 'bob', nodeName: 'Bob' },
];

describe('all-data aggregation', () => {
  it('dedupes nodes', () => {
    expect(uniqueNodes([{ nodeId: 'a' }, { nodeId: 'a' }, { nodeId: 'b' }])).toEqual([{ nodeId: 'a' }, { nodeId: 'b' }]);
  });

  it('aggregates datasources per node and records failures', () => {
    const r = aggregateDatasources(nodes, [
      {
        status: 'fulfilled',
        value: {
          total: 2,
          infos: [
            { datasourceId: 'ds1', name: 'oss', type: 'OSS', nodes: [{ nodeId: 'alice', status: 'Available' }] },
            { datasourceId: 'ds1', name: 'oss', type: 'OSS' },
          ],
        },
      },
      { status: 'rejected', reason: new Error('x') },
    ]);
    expect(r.items).toEqual([
      { datasourceId: 'ds1', name: 'oss', type: 'OSS', nodeId: 'alice', nodeName: 'Alice', status: 'Available', relatedDatas: undefined },
    ]);
    expect(r.failed).toEqual(['Bob']);
  });

  it('aggregates datatables by id + node + type', () => {
    const r = aggregateDatatables(nodes, [
      {
        status: 'fulfilled',
        value: [
          { datatableId: 't1', datatableName: 'a' },
          { datatableId: 't1', datatableName: 'a' },
          { datatableId: 't1', datatableName: 'a', datasourceType: 'HTTP' },
        ],
      },
      { status: 'fulfilled', value: [{ datatableId: 't1', datatableName: 'a', nodeId: 'bob' }] },
    ]);
    expect(r.items.map((i) => `${i.datatableId}/${i.nodeId}/${i.datasourceType || ''}/${i.nodeName}`)).toEqual([
      't1/alice//Alice',
      't1/alice/HTTP/Alice',
      't1/bob//Bob',
    ]);
  });

  it('filters by name and node', () => {
    const items = [
      { nodeId: 'alice', name: 'Credit' },
      { nodeId: 'bob', name: 'credit2' },
      { nodeId: 'bob', name: 'x' },
    ];
    expect(filterAggregated(items, 'CRED', '', (i) => i.name)).toHaveLength(2);
    expect(filterAggregated(items, 'cred', 'bob', (i) => i.name)).toEqual([{ nodeId: 'bob', name: 'credit2' }]);
  });
});
