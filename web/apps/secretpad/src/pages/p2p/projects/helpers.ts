/**
 * Pure helpers for P2P projects (legacy `p2p-project-list/components/common.tsx`,
 * `p2p-project-list/index.tsx` and `create-project/p2p-create-project`).
 */
import type { P2pProjectVOJava, ProjectCreateVoteConfigJava } from '@secretpad/api-client';

export const PROJECT_STATUS = { REVIEWING: 'REVIEWING', APPROVED: 'APPROVED', ARCHIVED: 'ARCHIVED' } as const;
const VOTE = { REVIEWING: 'REVIEWING', APPROVED: 'APPROVED', REJECTED: 'REJECTED' } as const;

export const PROJECT_NAME_MAX = 32;
export const PROJECT_DESC_MAX = 128;
export const PROJECT_TEXT_PATTERN = /^[\u4E00-\u9FA5A-Za-z0-9\-_]+$/;
/** Max node groups ("最多可建5组") and invitees per group ("最多可选9个"). */
export const MAX_NODE_GROUPS = 5;
export const MAX_INVITEES_PER_GROUP = 9;

/** All invited parties approved (legacy `checkAllApproved`). */
export function checkAllApproved(project: P2pProjectVOJava): boolean {
  return (project.partyVoteInfos || []).every((p) => p.action === VOTE.APPROVED);
}

/** Some party still reviewing and none rejected (legacy `checkProjectIsReviewing`). */
export function checkProjectIsReviewing(project: P2pProjectVOJava): boolean {
  const parties = project.partyVoteInfos || [];
  return parties.some((p) => p.action === VOTE.REVIEWING) && !parties.some((p) => p.action === VOTE.REJECTED);
}

export function isInitiator(project: P2pProjectVOJava, ownerId: string): boolean {
  return !!ownerId && project.initiator === ownerId;
}

export function isInvited(project: P2pProjectVOJava, ownerId: string): boolean {
  return !!ownerId && !isInitiator(project, ownerId) && (project.partyVoteInfos || []).some((p) => p.partyId === ownerId);
}

/** Only the initiator may edit, and not once archived. */
export function canEditProject(project: P2pProjectVOJava, ownerId: string): boolean {
  return project.status !== PROJECT_STATUS.ARCHIVED && isInitiator(project, ownerId);
}

/** "进入项目" is enabled once cooperation is fully approved (archived projects stay viewable). */
export function canEnterProject(project: P2pProjectVOJava): boolean {
  return checkAllApproved(project);
}

/** Legacy `projectCanArchived`. */
export function canArchiveProject(project: P2pProjectVOJava, ownerId: string): boolean {
  if (project.status === PROJECT_STATUS.ARCHIVED) return false;
  if (checkAllApproved(project)) return true;
  if (checkProjectIsReviewing(project)) return isInitiator(project, ownerId);
  return false;
}

/** Archiving an approved project needs a PROJECT_ARCHIVE vote; otherwise it is archived directly. */
export function archiveNeedsVote(project: P2pProjectVOJava): boolean {
  return checkAllApproved(project);
}

/** My own pending vote on a project that is still under review. */
export function hasSelfPendingVote(project: P2pProjectVOJava, ownerId: string): boolean {
  return (
    project.status === PROJECT_STATUS.REVIEWING &&
    (project.partyVoteInfos || []).some((p) => p.partyId === ownerId && p.action === VOTE.REVIEWING)
  );
}

export type ProjectOwnerTab = '' | 'mine' | 'invited';

export interface ProjectFilters {
  tab: ProjectOwnerTab;
  /** '' | REVIEWING | APPROVED | ARCHIVED */
  status: string;
  /** '' | MPC | TEE */
  mode: string;
  keyword: string;
}

export function filterProjects(list: P2pProjectVOJava[], f: ProjectFilters, ownerId: string): P2pProjectVOJava[] {
  const kw = f.keyword.trim();
  return list.filter((p) => {
    if (f.tab === 'mine' && !isInitiator(p, ownerId)) return false;
    if (f.tab === 'invited' && !isInvited(p, ownerId)) return false;
    if (f.status && p.status !== f.status) return false;
    const tee = (p.computeMode || '').includes('TEE');
    if (f.mode === 'TEE' && !tee) return false;
    if (f.mode === 'MPC' && tee) return false;
    if (kw && !(p.projectName || '').includes(kw)) return false;
    return true;
  });
}

/** i18n error key or undefined. */
export function validateProjectName(name: string): string | undefined {
  const v = name.trim();
  if (!v) return 'p2pProjects.errors.nameRequired';
  if (v.length > PROJECT_NAME_MAX) return 'p2pProjects.errors.nameTooLong';
  if (!PROJECT_TEXT_PATTERN.test(v)) return 'p2pProjects.errors.pattern';
  return undefined;
}

export function validateProjectDesc(desc: string): string | undefined {
  const v = desc.trim();
  if (!v) return undefined;
  if (v.length > PROJECT_DESC_MAX) return 'p2pProjects.errors.descTooLong';
  if (!PROJECT_TEXT_PATTERN.test(v)) return 'p2pProjects.errors.pattern';
  return undefined;
}

export interface NodeGroup {
  /** my node */
  nodeId: string;
  /** invited node ids (authorised routes to my node) */
  invitees: string[];
}

export function validateNodeGroups(groups: NodeGroup[]): string | undefined {
  if (groups.length === 0) return 'p2pProjects.errors.groupsRequired';
  if (groups.length > MAX_NODE_GROUPS) return 'p2pProjects.errors.groupsTooMany';
  if (groups.some((g) => !g.nodeId)) return 'p2pProjects.errors.nodeRequired';
  if (groups.some((g) => g.invitees.length === 0)) return 'p2pProjects.errors.inviteesRequired';
  if (groups.some((g) => g.invitees.length > MAX_INVITEES_PER_GROUP)) return 'p2pProjects.errors.inviteesTooMany';
  return undefined;
}

export interface VoterNode {
  nodeId: string;
  nodeName: string;
  instId: string;
  instName: string;
}

/** Legacy `createProject` voteConfig: participants = [self, ...invitee institutions] (unique). */
export function buildProjectCreateVoteConfig(
  projectId: string,
  ownerId: string,
  groups: NodeGroup[],
  votersByNode: Record<string, VoterNode[]>,
): ProjectCreateVoteConfigJava {
  const participants = [ownerId];
  const participantNodeInstVOS = groups.map((g) => ({
    initiatorNodeId: g.nodeId,
    invitees: g.invitees.map((id) => {
      const instId = (votersByNode[g.nodeId] || []).find((n) => n.nodeId === id)?.instId;
      if (instId) participants.push(instId);
      return { inviteeId: id };
    }),
  }));
  return { projectId, participants: [...new Set(participants)], participantNodeInstVOS };
}

/** Authorised partner nodes for my node: routes with dst = my node and status Succeeded (src = partner). */
export function votersForNode(
  routes: { dstNodeId?: string; status?: string; srcNode?: { nodeId?: string; nodeName?: string; instId?: string; instName?: string } }[],
  nodeId: string,
): VoterNode[] {
  return routes
    .filter((r) => r.dstNodeId === nodeId && r.status === 'Succeeded')
    .map((r) => ({
      nodeId: r.srcNode?.nodeId || '',
      nodeName: r.srcNode?.nodeName || '',
      instId: r.srcNode?.instId || '',
      instName: r.srcNode?.instName || '',
    }))
    .filter((n) => n.nodeId);
}
