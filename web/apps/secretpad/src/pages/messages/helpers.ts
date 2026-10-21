/**
 * Pure helpers for the message center (legacy `message-center/message.service.ts`
 * and `message-center/index.tsx`).
 */
import type { MessageListRequestJava, MessageVOJava, PartyVoteStatusJava } from '@secretpad/api-client';

/** Tabs: `process` = 我处理的 (isInitiator=false), `apply` = 我发起的 (isInitiator=true). */
export type MessageTab = 'process' | 'apply';

/** Status filter: '' = all, PENDING = isProcessed:false, PROCESSED = isProcessed:true. */
export type MessageStateFilter = '' | 'PENDING' | 'PROCESSED';

export const MESSAGE_TYPES = ['TEE_DOWNLOAD', 'NODE_ROUTE', 'PROJECT_ARCHIVE', 'PROJECT_CREATE'] as const;
export type MessageType = (typeof MESSAGE_TYPES)[number];

export const VOTE_STATUS = { REVIEWING: 'REVIEWING', APPROVED: 'APPROVED', REJECTED: 'REJECTED' } as const;

export const REJECT_REASON_MAX = 50;

export interface MessageFilters {
  tab: MessageTab;
  state: MessageStateFilter;
  /** '' = all types */
  type: string;
  keyword: string;
  page: number;
  size: number;
  ownerId: string;
}

export function buildMessageListRequest(f: MessageFilters): MessageListRequestJava {
  return {
    page: f.page,
    size: f.size,
    isInitiator: f.tab === 'apply',
    ownerId: f.ownerId,
    isProcessed: f.state === '' ? undefined : f.state === 'PROCESSED',
    type: f.type && f.type !== 'ALL' ? f.type : undefined,
    keyWord: f.keyword.trim(),
  };
}

/** Legacy: P2P/AUTONOMY platforms show project votes; others node-route / TEE download. */
export function messageTypesFor(isP2p: boolean): MessageType[] {
  return isP2p ? ['PROJECT_ARCHIVE', 'PROJECT_CREATE'] : ['TEE_DOWNLOAD', 'NODE_ROUTE'];
}

/** Approve/reject buttons: only on my pending votes. */
export function canReplyMessage(item: MessageVOJava, tab: MessageTab, state: MessageStateFilter): boolean {
  return tab === 'process' && (state === '' || state === 'PENDING') && item.status === VOTE_STATUS.REVIEWING;
}

/**
 * Legacy `showEnterProjectButton` (P2P only):
 * - PROJECT_CREATE: every party approved the invitation;
 * - PROJECT_ARCHIVE: every party had approved the original project creation.
 */
export function canEnterProjectFromMessage(item: MessageVOJava): boolean {
  const parties: PartyVoteStatusJava[] = item.initiatingTypeMessage?.partyVoteStatuses || [];
  if (!item.voteTypeMessage?.projectId || parties.length === 0) return false;
  if (item.type === 'PROJECT_CREATE') return parties.every((p) => p.action === VOTE_STATUS.APPROVED);
  if (item.type === 'PROJECT_ARCHIVE') return parties.every((p) => p.projectCreateVoteAction === VOTE_STATUS.APPROVED);
  return false;
}

export function isRejectReasonValid(reason: string): boolean {
  return reason.length <= REJECT_REASON_MAX;
}

export function voteStatusBadge(status?: string): 'success' | 'processing' | 'error' | 'default' {
  if (status === VOTE_STATUS.APPROVED) return 'success';
  if (status === VOTE_STATUS.REJECTED) return 'error';
  if (status === VOTE_STATUS.REVIEWING) return 'processing';
  return 'default';
}
