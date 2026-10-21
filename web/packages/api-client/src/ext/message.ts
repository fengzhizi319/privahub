/**
 * Java-contract extension APIs: message center (votes / approvals).
 *
 * Mirrors `MessageController.ts` of the legacy Java SecretPad frontend.
 */
import { javaPost } from './core';

export type VoteTypeJava = 'TEE_DOWNLOAD' | 'NODE_ROUTE' | 'PROJECT_ARCHIVE' | 'PROJECT_CREATE';

/** Vote status (`VoteStatusEnum`). */
export type VoteActionJava = 'REVIEWING' | 'APPROVED' | 'REJECTED';

export interface MessageListRequestJava {
  page?: number;
  size?: number;
  sort?: Record<string, string>;
  /** true: messages I initiated; false: messages I have to process */
  isInitiator?: boolean;
  /** requester node / institution id */
  ownerId?: string;
  /** undefined: all; false: pending; true: processed */
  isProcessed?: boolean;
  /** VoteTypeJava, undefined for all */
  type?: string;
  keyWord?: string;
}

export interface PartyVoteStatusJava {
  nodeID?: string;
  nodeName?: string;
  participantID?: string;
  participantName?: string;
  action?: string;
  reason?: string;
  /** PROJECT_ARCHIVE: the party's vote on the original project creation */
  projectCreateVoteAction?: string;
}

export interface MessageVOJava {
  type?: string;
  /** initiator: overall status; processor: own status */
  status?: string;
  initiatingTypeMessage?: {
    initiatorNodeName?: string;
    partyVoteStatuses?: PartyVoteStatusJava[];
    [key: string]: unknown;
  };
  voteTypeMessage?: {
    projectId?: string;
    computeMode?: string;
    computeFunc?: string;
    [key: string]: unknown;
  };
  messageName?: string;
  createTime?: string;
  voteID?: string;
}

export interface MessageListVOJava {
  messages: MessageVOJava[];
  total: number;
}

export interface MessageDetailRequestJava {
  ownerId: string;
  voteId: string;
  isInitiator: boolean;
  voteType?: string;
  projectId?: string;
}

/**
 * Java `MessageDetailVO` subclasses (fields depend on `type`):
 * - NODE_ROUTE: initiatorNodeName, nodeName, nodeID, url, isSingle, reason
 * - TEE_DOWNLOAD: messageName, project{projectName,gmtCreated,computeMode}, graphName,
 *   partyVoteStatuses, taskID, tableColumns
 * - PROJECT_CREATE / PROJECT_ARCHIVE: projectName, initiatorName, gmtCreated,
 *   participantNodeInstVOS, partyVoteStatuses, computeFunc, computeMode, projectDesc
 */
export interface MessageDetailVOJava {
  messageName?: string;
  type?: string;
  status?: string;
  reason?: string;
  // NODE_ROUTE
  initiatorNodeName?: string;
  nodeName?: string;
  nodeID?: string;
  url?: string;
  isSingle?: boolean;
  // TEE_DOWNLOAD
  project?: { projectName?: string; gmtCreated?: string; computeMode?: string; projectId?: string };
  graphName?: string;
  taskID?: string;
  tableColumns?: { colName?: string; colType?: string; colComment?: string }[];
  // PROJECT_*
  projectName?: string;
  initiatorId?: string;
  initiatorName?: string;
  gmtCreated?: string;
  projectDesc?: string;
  computeFunc?: string;
  computeMode?: string;
  participantNodeInstVOS?: ParticipantNodeInstVOJava[];
  partyVoteStatuses?: PartyVoteStatusJava[];
  [key: string]: unknown;
}

export interface ParticipantNodeInstVOJava {
  initiatorNodeId?: string;
  initiatorNodeName?: string;
  invitees?: { inviteeId?: string; inviteeName?: string; instId?: string; instName?: string }[];
}

export interface VoteReplyRequestJava {
  action: 'APPROVED' | 'REJECTED';
  reason?: string;
  voteId: string;
  voteParticipantId: string;
}

export async function listMessagesJava(req: MessageListRequestJava): Promise<MessageListVOJava> {
  const data = (await javaPost<{ messages?: MessageVOJava[]; total?: number }>('message/list', req)) || {};
  const messages = Array.isArray(data.messages) ? data.messages : [];
  return { messages, total: Number(data.total ?? messages.length) || 0 };
}

export async function getPendingMessageCountJava(ownerId: string): Promise<number> {
  return Number(await javaPost<number>('message/pending', { ownerId })) || 0;
}

/** message/detail 的 voteType 只能取这些值（后端校验，否则 202011100）。 */
export const MESSAGE_VOTE_TYPES = ['TEE_DOWNLOAD', 'NODE_ROUTE', 'PROJECT_CREATE', 'PROJECT_ARCHIVE'] as const;
export type MessageVoteType = (typeof MESSAGE_VOTE_TYPES)[number];
export const isMessageVoteType = (v?: string | null): v is MessageVoteType => !!v && (MESSAGE_VOTE_TYPES as readonly string[]).includes(v);

export async function getMessageDetailJava(req: MessageDetailRequestJava): Promise<MessageDetailVOJava> {
  if (!isMessageVoteType(req.voteType)) throw new Error(`unsupported voteType: ${req.voteType ?? ''}`);
  return (await javaPost<MessageDetailVOJava>('message/detail', req)) || {};
}

export async function replyMessageJava(req: VoteReplyRequestJava): Promise<void> {
  await javaPost('message/reply', req);
}
