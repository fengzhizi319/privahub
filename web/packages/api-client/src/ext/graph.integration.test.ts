import { describe, it, expect } from 'vitest';
import { apiClient } from '../client';
import {
  batchComponentsJava,
  getComponentI18nJava,
  getGraphDetailJava,
  getGraphNodeOutputJava,
  getProjectDatatableColumnsJava,
  getProjectDetailJava,
  listComponentsJava,
  listGraphJobsJava,
  listGraphNodeStatusJava,
  normalizeGraphStatus,
  refreshGraphNodeMaxIndexJava,
  startGraphJava,
} from './graph';

/** DAG 工作流适配器对 Java 契约形状（test/mocks/handlers.ts）的解析。 */
describe('graph ext（Java 契约）', () => {
  it('component/list：Map<app, CompListVO> → 带 app 的扁平组件列表', async () => {
    const list = await listComponentsJava();
    expect(list).toEqual([
      { app: 'secretflow', domain: 'data_prep', name: 'psi', version: '1.0.0', desc: 'PSI between two parties.' },
      { app: 'secretflow', domain: 'ml.train', name: 'ss_glm_train', version: '1.0.0', desc: 'SS-GLM training.' },
      { app: 'trustedflow', domain: 'data_prep', name: 'psi', version: '0.1.0', desc: 'TEE PSI.' },
    ]);
    // apiClient.getComponents 同样兼容 Map 形态（取 Object.values）。
    const legacy = await apiClient.getComponents();
    expect(legacy.map((c) => c.name)).toEqual(['secretflow', 'trustedflow']);
  });

  it('component/batch：有序数组按 domain/name 索引', async () => {
    const defs = await batchComponentsJava([{ app: 'secretflow', domain: 'data_prep', name: 'psi' }]);
    expect(Object.keys(defs)).toEqual(['data_prep/psi']);
    expect(defs['data_prep/psi'].inputs).toHaveLength(2);
  });

  it('component/i18n 原样返回嵌套 Map', async () => {
    const i18n = await getComponentI18nJava();
    expect((i18n.secretflow as Record<string, Record<string, string>>)['data_prep/psi:1.0.0'].psi).toBe('隐私求交');
  });

  it('graph/detail、graph/node/status、max_index、start', async () => {
    const g = await getGraphDetailJava('p1', 'g1');
    expect(g.nodes[0]).toMatchObject({ graphNodeId: 'g1-node-1', codeName: 'data_prep/psi', outputs: ['g1-node-1-output-0', 'g1-node-1-output-1'] });
    expect(g.dataSourceConfig?.[0].editEnable).toBe(true);
    const st = await listGraphNodeStatusJava('p1', 'g1');
    expect(st).toEqual({ finished: true, jobId: undefined, nodes: [expect.objectContaining({ graphNodeId: 'g1-node-1', status: 'STAGING' })] });
    expect(await refreshGraphNodeMaxIndexJava('p1', 'g1', 3)).toBe(32);
    expect(await startGraphJava('p1', 'g1', ['g1-node-1'])).toBe('job1');
  });

  it('旧 Go 形态的 status 仍可解析（snake_case、IDLE、无 finished）', () => {
    expect(normalizeGraphStatus({ status: 'RUNNING', nodes: [{ graph_node_id: 'n', status: 'IDLE' }] })).toEqual({
      finished: false,
      jobId: undefined,
      nodes: [expect.objectContaining({ graphNodeId: 'n', status: 'IDLE' })],
    });
  });

  it('graph/node/output 原样返回 Java GraphNodeOutputVO', async () => {
    const out = await getGraphNodeOutputJava({ projectId: 'p1', graphId: 'g1', graphNodeId: 'g1-node-1', outputId: 'g1-node-1-output-0' });
    expect((out?.meta as { rows: unknown[] }).rows).toHaveLength(1);
  });

  it('project/get 带 datatables；project/datatable/get 取顶层 configs', async () => {
    const p = await getProjectDetailJava('p1');
    expect(p.nodes[0].datatables).toEqual([{ datatableId: 't-alice', datatableName: 'alice.csv' }]);
    const cols = await getProjectDatatableColumnsJava({ projectId: 'p1', nodeId: 'alice', datatableId: 't-alice' });
    expect(cols.map((c) => c.colName)).toEqual(['id']);
  });

  it('project/job/list PageResponse', async () => {
    expect(await listGraphJobsJava({ projectId: 'p1', graphId: 'g1', pageNum: 1, pageSize: 5 })).toEqual({ list: [], total: 0 });
  });
});
