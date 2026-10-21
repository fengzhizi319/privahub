import { describe, expect, it, vi } from 'vitest';

vi.mock('@secretpad/api-client', () => ({
  apiClient: { createGraph: vi.fn(), updateGraph: vi.fn() },
  createProjectJava: vi.fn(),
  addProjectNodeJava: vi.fn(),
  addProjectInstJava: vi.fn(),
  addProjectDatatableJava: vi.fn(),
}));

import * as api from '@secretpad/api-client';
import { createProjectWithSetup, resolveInstIds, resolveNodeIds } from './service';
import type { CreateProjectDeps, CreateProjectInput } from './service';

const centerAdmin = { platformType: 'CENTER', ownerType: 'CENTER', ownerId: '' };
const edgeOnCenter = { platformType: 'CENTER', ownerType: 'EDGE', ownerId: 'alice' };
const autonomy = { platformType: 'AUTONOMY', ownerType: 'EDGE', ownerId: 'alice' };

function makeDeps(overrides: Partial<CreateProjectDeps> = {}): CreateProjectDeps {
  return {
    createProject: vi.fn().mockResolvedValue({ projectId: 'p1' }),
    addNode: vi.fn().mockResolvedValue(undefined),
    addInst: vi.fn().mockResolvedValue(undefined),
    addDatatable: vi.fn().mockResolvedValue(undefined),
    createGraph: vi.fn().mockResolvedValue('g1'),
    updateGraph: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

const baseInput: CreateProjectInput = {
  name: 'demo',
  description: 'desc',
  computeMode: 'MPC',
  computeFunc: 'DAG',
  templateKey: 'psi',
  participants: [
    { nodeId: 'alice', datatableId: 'alice_t' },
    { nodeId: 'bob', datatableId: 'bob_t' },
  ],
};

describe('resolveNodeIds / resolveInstIds', () => {
  it('skips the EDGE account own node on CENTER', () => {
    expect(resolveNodeIds(baseInput, edgeOnCenter)).toEqual(['bob']);
    expect(resolveNodeIds(baseInput, centerAdmin)).toEqual(['alice', 'bob']);
  });
  it('adds institutions only on CENTER', () => {
    expect(resolveInstIds(baseInput, centerAdmin)).toEqual(['alice', 'bob']);
    expect(resolveInstIds(baseInput, autonomy)).toEqual([]);
    const withInst = { ...baseInput, participants: [{ nodeId: 'n1', instId: 'i1' }, { nodeId: 'n2', instId: 'i1' }] };
    expect(resolveInstIds(withInst, centerAdmin)).toEqual(['i1']);
  });
});

describe('createProjectWithSetup', () => {
  it('orchestrates create → nodes → insts → datatables → graph', async () => {
    const deps = makeDeps();
    const result = await createProjectWithSetup(baseInput, centerAdmin, deps);
    expect(deps.createProject).toHaveBeenCalledWith({
      name: 'demo',
      description: 'desc',
      computeMode: 'MPC',
      teeNodeId: undefined,
      computeFunc: 'DAG',
    });
    expect(deps.addNode).toHaveBeenCalledTimes(2);
    expect(deps.addInst).toHaveBeenCalledTimes(2);
    expect(deps.addDatatable).toHaveBeenCalledWith(
      expect.objectContaining({ projectId: 'p1', nodeId: 'alice', datatableId: 'alice_t', configs: expect.any(Array) })
    );
    expect(deps.createGraph).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'p1' }));
    expect(deps.updateGraph).toHaveBeenCalledWith('p1', 'g1', expect.any(Array), expect.any(Array));
    expect(result).toEqual({ projectId: 'p1', graphId: 'g1', warnings: [] });
  });

  it('passes teeNodeId only in TEE mode', async () => {
    const deps = makeDeps();
    await createProjectWithSetup({ ...baseInput, computeMode: 'TEE', teeNodeId: 'tee', templateKey: 'tee' }, centerAdmin, deps);
    expect(deps.createProject).toHaveBeenCalledWith(expect.objectContaining({ computeMode: 'TEE', teeNodeId: 'tee' }));
    expect(deps.addDatatable).toHaveBeenCalledWith(expect.objectContaining({ teeNodeId: 'tee' }));
  });

  it('aborts when project/create fails', async () => {
    const deps = makeDeps({ createProject: vi.fn().mockRejectedValue(new Error('dup name')) });
    await expect(createProjectWithSetup(baseInput, centerAdmin, deps)).rejects.toThrow('dup name');
    expect(deps.addNode).not.toHaveBeenCalled();
  });

  it('collects partial failures as warnings', async () => {
    const deps = makeDeps({
      addNode: vi.fn().mockImplementation((_p: string, n: string) => (n === 'bob' ? Promise.reject(new Error('no route')) : Promise.resolve())),
      createGraph: vi.fn().mockRejectedValue(new Error('graph failed')),
    });
    const result = await createProjectWithSetup(baseInput, centerAdmin, deps);
    expect(result.projectId).toBe('p1');
    expect(result.graphId).toBeUndefined();
    expect(result.warnings).toEqual([
      { step: 'node', target: 'bob', message: 'no route' },
      { step: 'graph', target: 'psi', message: 'graph failed' },
    ]);
  });

  it('creates no graph without a template and skips unselected tables', async () => {
    const deps = makeDeps();
    await createProjectWithSetup(
      { ...baseInput, templateKey: undefined, participants: [{ nodeId: 'alice' }, { nodeId: 'bob' }] },
      autonomy,
      deps
    );
    expect(deps.addInst).not.toHaveBeenCalled();
    expect(deps.addDatatable).not.toHaveBeenCalled();
    expect(deps.createGraph).not.toHaveBeenCalled();
  });

  it('uses the Java ext api by default', async () => {
    vi.mocked(api.createProjectJava).mockResolvedValue({ projectId: 'p9' });
    vi.mocked(api.addProjectNodeJava).mockResolvedValue(undefined);
    vi.mocked(api.addProjectInstJava).mockResolvedValue(undefined);
    const result = await createProjectWithSetup(
      { ...baseInput, templateKey: undefined, participants: [{ nodeId: 'alice' }, { nodeId: 'bob' }] },
      centerAdmin
    );
    expect(result.projectId).toBe('p9');
    expect(api.addProjectNodeJava).toHaveBeenCalledWith('p9', 'alice');
    expect(api.addProjectInstJava).toHaveBeenCalledWith('p9', 'bob');
  });
});
