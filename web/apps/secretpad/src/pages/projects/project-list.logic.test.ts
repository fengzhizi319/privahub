import { describe, expect, it } from 'vitest';
import { filterProjects, isDeleteConfirmed, projectCounts, projectPermissions } from './project-list.logic';

const projects = [
  { projectName: 'Risk-A', computeMode: 'MPC' },
  { projectName: 'tee-b', computeMode: 'TEE' },
  { projectName: 'Other', computeMode: 'mpc' },
];

describe('project list logic', () => {
  it('filters by search and compute mode', () => {
    expect(filterProjects(projects, '', 'ALL')).toHaveLength(3);
    expect(filterProjects(projects, 'risk', 'ALL').map((p) => p.projectName)).toEqual(['Risk-A']);
    expect(filterProjects(projects, '', 'MPC').map((p) => p.projectName)).toEqual(['Risk-A', 'Other']);
    expect(filterProjects(projects, '', 'TEE').map((p) => p.projectName)).toEqual(['tee-b']);
  });
  it('reads graphCount / jobCount tolerant of missing values', () => {
    expect(projectCounts({ graphCount: 3, jobCount: 5 })).toEqual({ graphCount: 3, jobCount: 5 });
    expect(projectCounts({})).toEqual({ graphCount: 0, jobCount: 0 });
  });
  it('requires the exact project name to delete', () => {
    expect(isDeleteConfirmed({ projectName: 'abc' }, 'abc')).toBe(true);
    expect(isDeleteConfirmed({ projectName: 'abc' }, 'ABC')).toBe(false);
    expect(isDeleteConfirmed({ projectName: 'abc' }, 'abc ')).toBe(false);
    expect(isDeleteConfirmed({}, '')).toBe(false);
  });
  it('computes permissions', () => {
    expect(projectPermissions({ isCenter: true, isCenterAdmin: true, isAutonomy: false })).toEqual({ canCreate: true, canManage: true });
    expect(projectPermissions({ isCenter: true, isCenterAdmin: false, isAutonomy: false })).toEqual({ canCreate: true, canManage: false });
    expect(projectPermissions({ isCenter: false, isCenterAdmin: false, isAutonomy: true })).toEqual({ canCreate: true, canManage: true });
    expect(projectPermissions({ isCenter: false, isCenterAdmin: false, isAutonomy: false })).toEqual({ canCreate: false, canManage: false });
  });
});

it('searches IDs, descriptions and participants and groups explicit metadata only', async () => {
  const { groupProjects } = await import('./project-list.logic');
  const items = [
    { projectName: 'TEE-named MPC project', projectId: 'alpha', description: 'medical', computeMode: 'mpc', status: 'ACTIVE', nodes: [{nodeId:'alice',nodeName:'Hospital'}] },
    { projectName: 'Other', projectId: 'beta', computeMode: 'TEE', status: 'ARCHIVED' },
  ];
  for (const query of ['alpha','medical','alice','Hospital']) expect(filterProjects(items,query,'ALL')).toEqual([items[0]]);
  expect(groupProjects(items,'mode').map(group=>[group.key,group.projects.length])).toEqual([['MPC',1],['TEE',1]]);
  expect(groupProjects(items,'status').map(group=>group.key)).toEqual(['ACTIVE','ARCHIVED']);
  expect(groupProjects([{}],'status')[0].key).toBe('UNKNOWN');
  expect(groupProjects([],'mode')).toEqual([]);
});
