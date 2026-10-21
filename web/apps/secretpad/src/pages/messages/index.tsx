/**
 * Message center (legacy `modules/message-center`).
 *
 * Tabs 我处理的 / 我发起的 (isInitiator), status filter (isProcessed), type
 * filter, keyword search, server paging, per-type detail drawer, approve /
 * reject (reason ≤ 50 chars) and "enter project" for fully-approved P2P votes.
 */
import React, { useEffect, useState } from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Button, Card, Input, Pagination, RadioGroup, Select, Tabs } from '@secretpad/design-system';
import {
  getPendingMessageCountJava,
  listMessagesJava,
  type MessageVOJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { VoteStatusBadge } from '../../features/approval/participant-groups';
import { VoteReplyButtons } from '../../features/approval/vote-reply';
import { MessageDetailDrawer } from './message-detail';
import {
  buildMessageListRequest,
  canEnterProjectFromMessage,
  canReplyMessage,
  messageTypesFor,
  type MessageStateFilter,
  type MessageTab,
} from './helpers';

const PAGE_SIZE = 10;

export const MessagesPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { ownerId, isP2p } = usePlatform();

  const [tab, setTab] = useState<MessageTab>('process');
  const [state, setState] = useState<MessageStateFilter>('');
  const [type, setType] = useState('');
  const [keyword, setKeyword] = useState('');
  const [debouncedKeyword, setDebouncedKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [detail, setDetail] = useState<MessageVOJava | null>(null);

  useEffect(() => {
    const id = setTimeout(() => {
      setDebouncedKeyword(keyword);
      setPage(1);
    }, 300);
    return () => clearTimeout(id);
  }, [keyword]);

  const request = buildMessageListRequest({
    tab,
    state,
    type,
    keyword: debouncedKeyword,
    page,
    size: PAGE_SIZE,
    ownerId,
  });

  const listQuery = useQuery({
    queryKey: ['messages-java', request],
    queryFn: () => listMessagesJava(request),
    enabled: !!ownerId,
    placeholderData: keepPreviousData,
  });
  const messages = listQuery.data?.messages ?? [];
  const total = listQuery.data?.total ?? 0;

  const pendingQuery = useQuery({
    queryKey: ['messages-pending-java', ownerId],
    queryFn: () => getPendingMessageCountJava(ownerId),
    enabled: !!ownerId,
  });

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['messages-java'] });
    queryClient.invalidateQueries({ queryKey: ['messages-pending-java'] });
    queryClient.invalidateQueries({ queryKey: ['pending-message-count'] });
  };

  const changeTab = (key: string) => {
    setTab(key as MessageTab);
    setState('');
    setPage(1);
    setDetail(null);
  };

  const titleOf = (m: MessageVOJava) => {
    const suffix = m.type ? t(`msgCenter.suffix.${m.type}`) : '';
    const from =
      m.type === 'PROJECT_CREATE' && m.initiatingTypeMessage?.initiatorNodeName
        ? t('msgCenter.fromInst', { name: m.initiatingTypeMessage.initiatorNodeName })
        : '';
    return `${from}${m.messageName || ''}${suffix}`;
  };

  const enterProject = (m: MessageVOJava) =>
    navigate({ to: '/dag', search: { projectId: m.voteTypeMessage?.projectId } });

  const typeOptions = [
    { value: '', label: t('msgCenter.allTypes') },
    ...messageTypesFor(isP2p).map((v) => ({ value: v, label: t(`msgCenter.types.${v}`) })),
  ];

  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('messages.title')}</h2>
        <p className="text-xs text-gray-500">{t('messages.subtitle')}</p>
      </div>

      <Card>
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3 mb-4">
          <Tabs
            activeKey={tab}
            onChange={changeTab}
            items={[
              { key: 'process', label: t('msgCenter.tabProcess'), badge: pendingQuery.data || undefined },
              { key: 'apply', label: t('msgCenter.tabApply') },
            ]}
          />
          <div className="flex flex-wrap items-center gap-2">
            <RadioGroup
              name={t('msgCenter.state')}
              value={state}
              onChange={(v) => {
                setState(v as MessageStateFilter);
                setPage(1);
              }}
              options={[
                { value: '', label: t('msgCenter.stateAll') },
                {
                  value: 'PENDING',
                  label:
                    tab === 'process'
                      ? t('msgCenter.statePendingCount', { count: pendingQuery.data ?? 0 })
                      : t('msgCenter.statePending'),
                },
                { value: 'PROCESSED', label: t('msgCenter.stateProcessed') },
              ]}
            />
            <Select
              aria-label={t('msgCenter.type')}
              value={type}
              onChange={(v) => {
                setType(v);
                setPage(1);
              }}
              options={typeOptions}
            />
            <Input
              className="w-48"
              placeholder={t('msgCenter.searchPlaceholder')}
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
            />
          </div>
        </div>

        {!ownerId && <div className="text-xs text-gray-400">{t('msgCenter.noOwner')}</div>}
        {listQuery.error && (
          <div className="text-xs text-red-500 mb-2">{t('common.error', { message: listQuery.error.message })}</div>
        )}
        {listQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
        {!listQuery.isLoading && messages.length === 0 && ownerId && (
          <div className="text-xs text-gray-400 text-center py-8">{t('messages.noData')}</div>
        )}

        <ul className="divide-y divide-gray-100 dark:divide-gray-800">
          {messages.map((m) => {
            const parties = m.initiatingTypeMessage?.partyVoteStatuses || [];
            const showReply = canReplyMessage(m, tab, state);
            const showEnter = isP2p && canEnterProjectFromMessage(m);
            const rejectedReasons = parties.filter((p) => p.reason);
            return (
              <li key={m.voteID} className="py-3 flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="px-1.5 py-0.5 rounded border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300">
                      {m.type ? t(`msgCenter.types.${m.type}`) : '-'}
                    </span>
                    <button
                      type="button"
                      className="font-semibold text-gray-900 dark:text-gray-100 hover:text-blue-600 truncate text-left"
                      onClick={() => setDetail(m)}
                    >
                      {titleOf(m)}
                    </button>
                    {!(tab === 'process' && state === 'PENDING') && (
                      <span
                        className="inline-flex items-center gap-1"
                        title={
                          m.status === 'REJECTED' && rejectedReasons.length
                            ? rejectedReasons
                                .map((p) => `${p.participantName || p.nodeName}: ${p.reason}`)
                                .join('\n')
                            : undefined
                        }
                      >
                        <span className="text-gray-400">
                          {tab === 'process' ? t('msgCenter.myStatus') : t('msgCenter.currentStatus')}
                        </span>
                        <VoteStatusBadge action={m.status} />
                      </span>
                    )}
                  </div>
                  <div className="text-gray-400 flex flex-wrap gap-x-2">
                    <span>{m.createTime}</span>
                    {tab === 'apply' && m.type !== 'NODE_ROUTE' && parties.length > 0 && (
                      <span>
                        | {isP2p ? t('msgCenter.partnerInsts') : t('msgCenter.partnerNodes')}:{' '}
                        {parties
                          .map(
                            (p) =>
                              `${(isP2p ? p.participantName : p.nodeName) || p.participantID || p.nodeID} (${
                                p.action ? t(`approval.voteStatus.${p.action}`) : '-'
                              })`,
                          )
                          .join('、')}
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-3 flex-shrink-0">
                  {showEnter && (
                    <Button size="sm" variant="link" onClick={() => enterProject(m)}>
                      {t('msgCenter.enterProject')}
                    </Button>
                  )}
                  {showReply && <VoteReplyButtons voteId={m.voteID} participantId={ownerId} onDone={refresh} />}
                </div>
              </li>
            );
          })}
        </ul>

        {total > PAGE_SIZE && (
          <Pagination className="mt-3" page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
        )}
      </Card>

      {detail && (
        <MessageDetailDrawer
          message={detail}
          tab={tab}
          ownerId={ownerId}
          title={titleOf(detail)}
          onClose={() => setDetail(null)}
          onReplied={refresh}
        />
      )}
    </div>
  );
};
