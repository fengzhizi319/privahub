import { describe, it, expect } from 'vitest';
import {
  buildPorts,
  buildOutputIds,
  buildResultOutputs,
  deriveInputs,
  isReachable,
  maxNodeIndex,
  nodeIdOf,
  parseAnchor,
  pickConnection,
  validateConnection,
  clearNodeDefAttrs,
  remapImportedGraph,
} from '../graph-model';
import type { PortDef, GraphEdgeLike } from '../graph-model';

const psiDef = {
  inputs: [
    { name: 'input_ds1', types: ['sf.table.individual'] },
    { name: 'input_ds2', types: ['sf.table.individual'] },
  ],
  outputs: [
    { name: 'psi_output', types: ['sf.table.vertical'] },
    { name: 'report', types: ['sf.report'] },
  ],
};

describe('端口模型', () => {
  it('按 IoDef 生成端口，report / read_data 输出不出端口', () => {
    const ports = buildPorts('g-node-2', psiDef);
    expect(ports.map((p) => p.id)).toEqual(['g-node-2-input-0', 'g-node-2-input-1', 'g-node-2-output-0']);
    expect(buildPorts('g-node-1', { outputs: [{ types: ['sf.read_data'] }] })).toEqual([]);
  });

  it('输出锚点使用 nodeId（而非 codeName），outputs 列表包含所有输出', () => {
    expect(buildOutputIds('g-node-2', psiDef)).toEqual(['g-node-2-output-0', 'g-node-2-output-1']);
    expect(buildResultOutputs('g-node-2', psiDef)).toEqual([
      { id: 'g-node-2-output-0', name: 'psi_output', type: 'table' },
      { id: 'g-node-2-output-1', name: 'report', type: 'report' },
    ]);
  });

  it('节点 ID 与 max_index 约定', () => {
    expect(nodeIdOf('abc-def', 33)).toBe('abc-def-node-33');
    expect(maxNodeIndex(['abc-def-node-3', 'abc-def-node-12', 'legacy-id'])).toBe(12);
    expect(parseAnchor('abc-def-node-3-output-1')).toEqual({ nodeId: 'abc-def-node-3', kind: 'output', index: 1 });
  });

  it('deriveInputs：inputs[i] 为连到第 i 个输入端口的上游锚点', () => {
    const edges: GraphEdgeLike[] = [
      { id: 'e1', source: 'a', target: 'c', sourceAnchor: 'a-output-0', targetAnchor: 'c-input-1' },
      { id: 'e2', source: 'b', target: 'c', sourceAnchor: 'b-output-0', targetAnchor: 'c-input-0' },
    ];
    expect(deriveInputs('c', edges)).toEqual(['b-output-0', 'a-output-0']);
  });

  it('clearNodeDefAttrs 保留 domain/name/version', () => {
    expect(clearNodeDefAttrs({ domain: 'feature', name: 'binning_modifications', version: '1', attrs: [{}], attrPaths: ['x'] })).toEqual({
      domain: 'feature',
      name: 'binning_modifications',
      version: '1',
    });
  });
});

describe('连线校验', () => {
  const ports: Record<string, PortDef[]> = {
    a: buildPorts('a', { outputs: [{ types: ['sf.table.individual'] }, { types: ['sf.model.ss_glm'] }] }),
    b: buildPorts('b', psiDef),
    c: buildPorts('c', { inputs: [{ types: ['sf.table.vertical'] }], outputs: [{ types: ['sf.table.vertical'] }] }),
  };
  const ctx = (edges: GraphEdgeLike[]) => ({ edges, portsOf: (id: string) => ports[id] });

  it('方向：只能从输出连到输入', () => {
    const r = validateConnection({ source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-output-0' }, ctx([]));
    expect(r).toEqual({ ok: false, reason: 'DIRECTION' });
    expect(validateConnection({ source: 'a', target: 'a', sourceAnchor: 'a-output-0', targetAnchor: 'a-input-0' }, ctx([]))).toEqual({ ok: false, reason: 'SELF' });
  });

  it('类型：端口类型需有交集', () => {
    expect(validateConnection({ source: 'a', target: 'b', sourceAnchor: 'a-output-1', targetAnchor: 'b-input-0' }, ctx([]))).toEqual({ ok: false, reason: 'TYPE' });
    expect(validateConnection({ source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-input-0' }, ctx([]))).toEqual({ ok: true });
  });

  it('重复与输入端口占用', () => {
    const edges = [{ id: 'e', source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-input-0' }];
    expect(validateConnection({ source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-input-0' }, ctx(edges))).toEqual({ ok: false, reason: 'DUPLICATE' });
    const other = [{ id: 'e', source: 'x', target: 'b', sourceAnchor: 'x-output-0', targetAnchor: 'b-input-0' }];
    expect(validateConnection({ source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-input-0' }, ctx(other))).toEqual({ ok: false, reason: 'OCCUPIED' });
  });

  it('成环检测', () => {
    const edges = [
      { id: 'e1', source: 'b', target: 'c', sourceAnchor: 'b-output-0', targetAnchor: 'c-input-0' },
      { id: 'e2', source: 'c', target: 'd', sourceAnchor: 'c-output-0', targetAnchor: 'd-input-0' },
    ];
    expect(isReachable('b', 'd', edges)).toBe(true);
    const cyclePorts = { ...ports, b: buildPorts('b', { inputs: [{ types: ['sf.table.vertical'] }], outputs: [{ types: ['sf.table.vertical'] }] }) };
    const r = validateConnection(
      { source: 'c', target: 'b', sourceAnchor: 'c-output-0', targetAnchor: 'b-input-0' },
      { edges, portsOf: (id) => cyclePorts[id as keyof typeof cyclePorts] },
    );
    expect(r).toEqual({ ok: false, reason: 'CYCLE' });
  });

  it('pickConnection 自动选择首个兼容的空闲端口', () => {
    const edges = [{ id: 'e', source: 'x', target: 'b', sourceAnchor: 'x-output-0', targetAnchor: 'b-input-0' }];
    const r = pickConnection('a', 'b', ctx(edges));
    expect(r).toEqual({ ok: true, candidate: { source: 'a', target: 'b', sourceAnchor: 'a-output-0', targetAnchor: 'b-input-1' } });
    expect(pickConnection('a', 'c', ctx([]))).toEqual({ ok: false, reason: 'TYPE' });
  });
});

describe('remapImportedGraph（导入 JSON 改写为当前图 ID）', () => {
  it('节点、锚点、端口、inputs 与连线全部改写，连线 ID 按新锚点重算，悬空连线丢弃', () => {
    const nodes = [
      { id: 'other-node-1', outputs: ['other-node-1-output-0'], ports: [{ id: 'other-node-1-output-0' }], resultOutputs: [{ id: 'other-node-1-output-0' }] },
      { id: 'other-node-2', inputs: ['other-node-1-output-0'], outputs: ['other-node-2-output-0'], ports: [{ id: 'other-node-2-input-0' }] },
    ];
    const edges = [
      { id: 'x', source: 'other-node-1', target: 'other-node-2', sourceAnchor: 'other-node-1-output-0', targetAnchor: 'other-node-2-input-0' },
      { id: 'dangling', source: 'other-node-1', target: 'ghost' },
    ];
    const out = remapImportedGraph(nodes, edges, ['g1-node-33', 'g1-node-34']);
    expect(out.nodes.map((n) => n.id)).toEqual(['g1-node-33', 'g1-node-34']);
    expect(out.nodes[0].outputs).toEqual(['g1-node-33-output-0']);
    expect(out.nodes[0].ports).toEqual([{ id: 'g1-node-33-output-0' }]);
    expect(out.nodes[0].resultOutputs).toEqual([{ id: 'g1-node-33-output-0' }]);
    expect(out.nodes[1].inputs).toEqual(['g1-node-33-output-0']);
    expect(out.nodes[1].ports).toEqual([{ id: 'g1-node-34-input-0' }]);
    expect(out.edges).toEqual([
      {
        id: 'g1-node-33-output-0__g1-node-34-input-0',
        source: 'g1-node-33',
        target: 'g1-node-34',
        sourceAnchor: 'g1-node-33-output-0',
        targetAnchor: 'g1-node-34-input-0',
      },
    ]);
    expect(out.idMap.get('other-node-2')).toBe('g1-node-34');
  });
});
