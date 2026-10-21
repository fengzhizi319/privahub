import { describe, expect, it } from 'vitest';
import type { MessageVOJava } from '@secretpad/api-client';
import {
  buildMessageListRequest,
  canEnterProjectFromMessage,
  canReplyMessage,
  isRejectReasonValid,
  messageTypesFor,
  type MessageFilters,
} from './helpers';

const filters = (patch: Partial<MessageFilters> = {}): MessageFilters => ({
  tab: 'process',
  state: '',
  type: '',
  keyword: '',
  page: 1,
  size: 10,
  ownerId: 'alice',
  ...patch,
});

describe('buildMessageListRequest', () => {
  it('maps tabs to isInitiator', () => {
    expect(buildMessageListRequest(filters()).isInitiator).toBe(false);
    expect(buildMessageListRequest(filters({ tab: 'apply' })).isInitiator).toBe(true);
  });

  it('maps the status filter to isProcessed', () => {
    expect(buildMessageListRequest(filters()).isProcessed).toBeUndefined();
    expect(buildMessageListRequest(filters({ state: 'PENDING' })).isProcessed).toBe(false);
    expect(buildMessageListRequest(filters({ state: 'PROCESSED' })).isProcessed).toBe(true);
  });

  it('maps type / keyword / paging / owner', () => {
    expect(buildMessageListRequest(filters({ type: 'NODE_ROUTE', keyword: ' abc ', page: 3, size: 20 }))).toEqual({
      page: 3,
      size: 20,
      isInitiator: false,
      ownerId: 'alice',
      isProcessed: undefined,
      type: 'NODE_ROUTE',
      keyWord: 'abc',
    });
    expect(buildMessageListRequest(filters({ type: 'ALL' })).type).toBeUndefined();
  });
});

describe('message permissions', () => {
  const pending: MessageVOJava = { status: 'REVIEWING', type: 'NODE_ROUTE', voteID: 'v1' };

  it('only allows replying to my pending votes', () => {
    expect(canReplyMessage(pending, 'process', '')).toBe(true);
    expect(canReplyMessage(pending, 'process', 'PENDING')).toBe(true);
    expect(canReplyMessage(pending, 'process', 'PROCESSED')).toBe(false);
    expect(canReplyMessage(pending, 'apply', '')).toBe(false);
    expect(canReplyMessage({ ...pending, status: 'APPROVED' }, 'process', '')).toBe(false);
  });

  it('allows entering a project once every party approved', () => {
    const base: MessageVOJava = {
      type: 'PROJECT_CREATE',
      voteTypeMessage: { projectId: 'p1' },
      initiatingTypeMessage: { partyVoteStatuses: [{ action: 'APPROVED' }, { action: 'APPROVED' }] },
    };
    expect(canEnterProjectFromMessage(base)).toBe(true);
    expect(
      canEnterProjectFromMessage({
        ...base,
        initiatingTypeMessage: { partyVoteStatuses: [{ action: 'APPROVED' }, { action: 'REVIEWING' }] },
      }),
    ).toBe(false);
    expect(
      canEnterProjectFromMessage({
        ...base,
        type: 'PROJECT_ARCHIVE',
        initiatingTypeMessage: { partyVoteStatuses: [{ action: 'REVIEWING', projectCreateVoteAction: 'APPROVED' }] },
      }),
    ).toBe(true);
    expect(canEnterProjectFromMessage({ ...base, type: 'NODE_ROUTE' })).toBe(false);
  });

  it('limits the reject reason to 50 chars and splits types by platform', () => {
    expect(isRejectReasonValid('x'.repeat(50))).toBe(true);
    expect(isRejectReasonValid('x'.repeat(51))).toBe(false);
    expect(messageTypesFor(true)).toEqual(['PROJECT_ARCHIVE', 'PROJECT_CREATE']);
    expect(messageTypesFor(false)).toEqual(['TEE_DOWNLOAD', 'NODE_ROUTE']);
  });
});
