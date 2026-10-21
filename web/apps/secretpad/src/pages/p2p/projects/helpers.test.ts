import { describe, expect, it } from 'vitest';
import type { P2pProjectVOJava } from '@secretpad/api-client';
import {
  archiveNeedsVote,
  buildProjectCreateVoteConfig,
  canArchiveProject,
  canEditProject,
  canEnterProject,
  filterProjects,
  hasSelfPendingVote,
  isInvited,
  validateNodeGroups,
  validateProjectDesc,
  validateProjectName,
  votersForNode,
} from './helpers';

const approved: P2pProjectVOJava = {
  projectId: 'p1',
  projectName: 'alpha',
  initiator: 'alice',
  status: 'APPROVED',
  computeMode: 'MPC',
  partyVoteInfos: [{ partyId: 'bob', action: 'APPROVED' }],
};
const reviewing: P2pProjectVOJava = {
  projectId: 'p2',
  projectName: 'beta',
  initiator: 'bob',
  status: 'REVIEWING',
  computeMode: 'TEE',
  partyVoteInfos: [
    { partyId: 'alice', action: 'REVIEWING' },
    { partyId: 'carol', action: 'APPROVED' },
  ],
};
const archived: P2pProjectVOJava = { ...approved, projectId: 'p3', projectName: 'gamma', status: 'ARCHIVED' };

describe('p2p permissions', () => {
  it('only lets the initiator edit non-archived projects', () => {
    expect(canEditProject(approved, 'alice')).toBe(true);
    expect(canEditProject(approved, 'bob')).toBe(false);
    expect(canEditProject(archived, 'alice')).toBe(false);
  });

  it('enables entering only after every party approved', () => {
    expect(canEnterProject(approved)).toBe(true);
    expect(canEnterProject(reviewing)).toBe(false);
    expect(canEnterProject({ ...approved, partyVoteInfos: [{ partyId: 'bob', action: 'REJECTED' }] })).toBe(false);
  });

  it('detects invited projects and my pending vote', () => {
    expect(isInvited(reviewing, 'alice')).toBe(true);
    expect(isInvited(reviewing, 'bob')).toBe(false); // initiator
    expect(isInvited(approved, 'carol')).toBe(false);
    expect(hasSelfPendingVote(reviewing, 'alice')).toBe(true);
    expect(hasSelfPendingVote(reviewing, 'carol')).toBe(false);
  });

  it('archive rules follow the legacy projectCanArchived', () => {
    expect(canArchiveProject(approved, 'bob')).toBe(true);
    expect(canArchiveProject(archived, 'alice')).toBe(false);
    expect(canArchiveProject(reviewing, 'bob')).toBe(true);
    expect(canArchiveProject(reviewing, 'alice')).toBe(false);
    expect(archiveNeedsVote(approved)).toBe(true);
    expect(archiveNeedsVote(reviewing)).toBe(false);
  });
});

describe('filterProjects', () => {
  const list = [approved, reviewing, archived];
  const f = { tab: '' as const, status: '', mode: '', keyword: '' };
  it('filters by owner tab, status, compute mode and keyword', () => {
    expect(filterProjects(list, { ...f, tab: 'mine' }, 'alice').map((p) => p.projectId)).toEqual(['p1', 'p3']);
    expect(filterProjects(list, { ...f, tab: 'invited' }, 'alice').map((p) => p.projectId)).toEqual(['p2']);
    expect(filterProjects(list, { ...f, status: 'ARCHIVED' }, 'alice').map((p) => p.projectId)).toEqual(['p3']);
    expect(filterProjects(list, { ...f, mode: 'TEE' }, 'alice').map((p) => p.projectId)).toEqual(['p2']);
    expect(filterProjects(list, { ...f, mode: 'MPC' }, 'alice').map((p) => p.projectId)).toEqual(['p1', 'p3']);
    expect(filterProjects(list, { ...f, keyword: 'et' }, 'alice').map((p) => p.projectId)).toEqual(['p2']);
  });
});

describe('create project', () => {
  it('validates name / description like the legacy form', () => {
    expect(validateProjectName('')).toBe('p2pProjects.errors.nameRequired');
    expect(validateProjectName('a'.repeat(33))).toBe('p2pProjects.errors.nameTooLong');
    expect(validateProjectName('bad name')).toBe('p2pProjects.errors.pattern');
    expect(validateProjectName('项目_a-1')).toBeUndefined();
    expect(validateProjectDesc('')).toBeUndefined();
    expect(validateProjectDesc('x'.repeat(129))).toBe('p2pProjects.errors.descTooLong');
  });

  it('validates node groups (≤5 groups, 1..9 invitees)', () => {
    expect(validateNodeGroups([])).toBe('p2pProjects.errors.groupsRequired');
    expect(validateNodeGroups([{ nodeId: '', invitees: ['x'] }])).toBe('p2pProjects.errors.nodeRequired');
    expect(validateNodeGroups([{ nodeId: 'n1', invitees: [] }])).toBe('p2pProjects.errors.inviteesRequired');
    expect(
      validateNodeGroups(Array.from({ length: 6 }, (_, i) => ({ nodeId: `n${i}`, invitees: ['x'] }))),
    ).toBe('p2pProjects.errors.groupsTooMany');
    expect(validateNodeGroups([{ nodeId: 'n1', invitees: Array.from({ length: 10 }, (_, i) => `x${i}`) }])).toBe(
      'p2pProjects.errors.inviteesTooMany',
    );
    expect(validateNodeGroups([{ nodeId: 'n1', invitees: ['x'] }])).toBeUndefined();
  });

  it('derives voters from succeeded routes and builds the PROJECT_CREATE voteConfig', () => {
    const routes = [
      { dstNodeId: 'alice-n1', status: 'Succeeded', srcNode: { nodeId: 'bob-n1', instId: 'bob' } },
      { dstNodeId: 'alice-n1', status: 'Failed', srcNode: { nodeId: 'carol-n1', instId: 'carol' } },
      { dstNodeId: 'alice-n2', status: 'Succeeded', srcNode: { nodeId: 'bob-n2', instId: 'bob' } },
    ];
    const voters = { 'alice-n1': votersForNode(routes, 'alice-n1'), 'alice-n2': votersForNode(routes, 'alice-n2') };
    expect(voters['alice-n1'].map((v) => v.nodeId)).toEqual(['bob-n1']);
    expect(
      buildProjectCreateVoteConfig(
        'p9',
        'alice',
        [
          { nodeId: 'alice-n1', invitees: ['bob-n1'] },
          { nodeId: 'alice-n2', invitees: ['bob-n2'] },
        ],
        voters,
      ),
    ).toEqual({
      projectId: 'p9',
      participants: ['alice', 'bob'],
      participantNodeInstVOS: [
        { initiatorNodeId: 'alice-n1', invitees: [{ inviteeId: 'bob-n1' }] },
        { initiatorNodeId: 'alice-n2', invitees: [{ inviteeId: 'bob-n2' }] },
      ],
    });
  });
});
