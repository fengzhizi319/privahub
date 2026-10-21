/**
 * Java-contract extension APIs: P2P projects and approvals.
 *
 * Mirrors `P2PProjectController.ts`, `ApprovalController.ts`,
 * `InstController.listNode` and `NodeRouteController.page` of the legacy
 * Java SecretPad frontend.
 */
import { javaPost, toPage, type JavaPage } from './core';
import type { ParticipantNodeInstVOJava, PartyVoteStatusJava } from './message';

export interface PartyVoteInfoVOJava {
  partyId?: string;
  partyName?: string;
  /** REVIEWING | APPROVED | REJECTED */
  action?: string;
  reason?: string;
}

export interface P2pProjectVOJava {
  projectId?: string;
  projectName?: string;
  description?: string;
  partyVoteInfos?: PartyVoteInfoVOJava[];
  graphCount?: number;
  jobCount?: number;
  gmtCreate?: string;
  /** MPC | TEE */
  computeMode?: string;
  teeNodeId?: string;
  /** REVIEWING | APPROVED | ARCHIVED */
  status?: string;
  /** initiator institution id */
  initiator?: string;
  initiatorName?: string;
  /** DAG | PSI | ALL */
  computeFunc?: string;
  voteId?: string;
}

export interface ProjectParticipantsDetailVOJava {
  initiatorId?: string;
  initiatorName?: string;
  projectName?: string;
  partyVoteStatuses?: PartyVoteStatusJava[];
  computeMode?: string;
  computeFunc?: string;
  projectDesc?: string;
  gmtCreated?: string;
  participantNodeInstVOS?: ParticipantNodeInstVOJava[];
  status?: string;
}

/** voteConfig shapes per voteType (legacy frontend). */
export interface ProjectCreateVoteConfigJava {
  projectId: string;
  /** institution ids (self first), de-duplicated */
  participants: string[];
  participantNodeInstVOS: { initiatorNodeId: string; invitees: { inviteeId: string }[] }[];
}

export interface ProjectArchiveVoteConfigJava {
  projectId: string;
}

export type CreateApprovalRequestJava =
  | { initiatorId: string; voteType: 'PROJECT_CREATE'; voteConfig: ProjectCreateVoteConfigJava }
  | { initiatorId: string; voteType: 'PROJECT_ARCHIVE'; voteConfig: ProjectArchiveVoteConfigJava }
  | { initiatorId: string; voteType: 'NODE_ROUTE' | 'TEE_DOWNLOAD'; voteConfig: Record<string, unknown> };

export interface InstNodeVOJava {
  nodeId?: string;
  nodeName?: string;
  nodeStatus?: string;
  instId?: string;
  instName?: string;
}

export interface NodeRouterVOJava {
  routeId?: string;
  srcNodeId?: string;
  dstNodeId?: string;
  srcNode?: InstNodeVOJava;
  dstNode?: InstNodeVOJava;
  srcNetAddress?: string;
  dstNetAddress?: string;
  /** Pending | Succeeded | Failed | Unknown */
  status?: string;
}

export async function listP2pProjectsJava(): Promise<P2pProjectVOJava[]> {
  const data = await javaPost<P2pProjectVOJava[] | { list?: P2pProjectVOJava[] }>('p2p/project/list', {});
  return toPage<P2pProjectVOJava>(data, ['list']).list;
}

export async function createP2pProjectJava(req: {
  name: string;
  description?: string;
  computeMode: string;
  computeFunc: string;
}): Promise<{ projectId?: string }> {
  return (await javaPost<{ projectId?: string }>('p2p/project/create', req)) || {};
}

export async function updateP2pProjectJava(req: { projectId: string; name: string; description?: string }): Promise<void> {
  await javaPost('p2p/project/update', req);
}

/** Direct archive (legacy: only used when the project never reached full approval). */
export async function archiveP2pProjectJava(projectId: string): Promise<void> {
  await javaPost('p2p/project/archive', { projectId });
}

export async function getP2pProjectParticipantsJava(voteId: string): Promise<ProjectParticipantsDetailVOJava> {
  return (await javaPost<ProjectParticipantsDetailVOJava>('p2p/project/participants', { voteId })) || {};
}

export async function createApprovalJava(req: CreateApprovalRequestJava): Promise<unknown> {
  return javaPost('approval/create', req);
}

/** POST inst/node/list → all nodes of my institution. */
export async function listInstNodesJava(): Promise<InstNodeVOJava[]> {
  return toPage<InstNodeVOJava>(await javaPost('inst/node/list', {}), ['list', 'nodes']).list;
}

/** POST nodeRoute/page. */
export async function pageNodeRoutesJava(req: {
  page: number;
  size: number;
  search?: string;
  sort?: Record<string, string>;
  ownerId?: string;
}): Promise<JavaPage<NodeRouterVOJava>> {
  return toPage<NodeRouterVOJava>(await javaPost('nodeRoute/page', req), ['list']);
}
