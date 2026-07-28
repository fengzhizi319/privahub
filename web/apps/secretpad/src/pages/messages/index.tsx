import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Badge, Button, Modal, toast } from '@secretpad/design-system';
import type { User, MessageVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { useAuthStore } from '../../features/auth/model/auth-store';
import { CreateApprovalModal } from '../../features/approval/create-approval-modal';
import { ApprovalStatusPoller } from '../../features/approval/approval-status-poller';

const PENDING_STATUSES = ['PENDING', 'WAITING', 'REVIEWING'];

export const MessagesPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { user } = useAuthStore();

  const ownerId = (user as User | null)?.ownerId || '';

  const [replyTarget, setReplyTarget] = useState<MessageVO | null>(null);
  const [replyReason, setReplyReason] = useState('');
  const [detailTarget, setDetailTarget] = useState<MessageVO | null>(null);
  // 审批主动创建 Modal 的开关状态。
  const [createOpen, setCreateOpen] = useState(false);

  const messagesQuery = useQuery({
    queryKey: ['messages', ownerId],
    queryFn: () => apiClient.getMessages(ownerId, 1, 100),
    enabled: !!ownerId,
  });
  const messages = messagesQuery.data ?? [];

  const pendingCountQuery = useQuery({
    queryKey: ['pending-message-count', ownerId],
    queryFn: () => apiClient.getPendingMessageCount(ownerId),
    enabled: !!ownerId,
  });
  const pendingCount = pendingCountQuery.data ?? 0;

  const detailQuery = useQuery({
    queryKey: ['message-detail', detailTarget?.voteID],
    queryFn: () =>
      apiClient.getMessageDetail({
        ownerId,
        voteId: detailTarget!.voteID!,
        isInitiator: false,
        voteType: detailTarget!.type || '',
      }),
    enabled: !!detailTarget && !!detailTarget.voteID,
  });

  const invalidateMessages = () => {
    queryClient.invalidateQueries({ queryKey: ['messages', ownerId] });
    queryClient.invalidateQueries({ queryKey: ['pending-message-count', ownerId] });
  };

  const replyMutation = useMutation({
    mutationFn: (action: string) =>
      apiClient.replyMessage({
        voteId: replyTarget!.voteID!,
        voteParticipantId: ownerId,
        action,
        reason: replyReason || undefined,
      }),
    onSuccess: () => {
      setReplyTarget(null);
      setReplyReason('');
      invalidateMessages();
      toast.success(t('messages.replySuccess'));
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const handleRefresh = () => invalidateMessages();

  const isPending = (msg: MessageVO) =>
    PENDING_STATUSES.includes((msg.status || '').toUpperCase());

  const statusBadge = (status?: string) => {
    switch (status?.toUpperCase()) {
      case 'PENDING':
      case 'WAITING':
      case 'REVIEWING':
        return 'warning';
      case 'AGREE':
      case 'APPROVED':
      case 'SUCCEED':
        return 'success';
      case 'REJECT':
      case 'REJECTED':
      case 'FAILED':
        return 'error';
      default:
        return 'default';
    }
  };

  const loading = messagesQuery.isLoading;
  const error = messagesQuery.error?.message || pendingCountQuery.error?.message || null;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('messages.title')}</h2>
          <p className="text-xs text-gray-500">{t('messages.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <Badge status={pendingCount > 0 ? 'warning' : 'success'}>
            {t('messages.pendingCount', { count: pendingCount })}
          </Badge>
          <Button size="sm" variant="primary" onClick={() => setCreateOpen(true)}>{t('approval.create')}</Button>
          <Button size="sm" variant="outline" onClick={handleRefresh}>{t('common.refresh')}</Button>
        </div>
      </div>

      {/* 审批状态轮询：填写资源定位字段后按间隔轮询各参与方投票状态 */}
      <ApprovalStatusPoller />

      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <Card bodyClassName="p-0">
        <div className="divide-y divide-gray-100 dark:divide-gray-800 text-xs">
          {messages.length === 0 && !loading && !error && (
            <div className="p-4 text-center text-gray-400">{t('messages.noData')}</div>
          )}
          {messages.map((msg) => (
            <div key={msg.voteID || msg.messageName} className="p-4 flex items-center justify-between hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
              <div className="min-w-0">
                <div className="font-semibold text-gray-900 dark:text-gray-100 truncate">{msg.messageName}</div>
                <div className="text-gray-500 mt-1">
                  {t('messages.type')}: {msg.type} • {t('messages.createTime')}: {msg.createTime}
                </div>
              </div>
              <div className="flex items-center gap-2 flex-shrink-0">
                <Badge status={statusBadge(msg.status)}>
                  {msg.status || t('messages.statusUnknown')}
                </Badge>
                <Button size="sm" variant="ghost" onClick={() => setDetailTarget(msg)}>{t('messages.detail')}</Button>
                {isPending(msg) && (
                  <Button size="sm" variant="primary" onClick={() => { setReplyReason(''); setReplyTarget(msg); }}>
                    {t('messages.reply')}
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      </Card>

      {/* Reply Modal */}
      <Modal
        isOpen={!!replyTarget}
        onClose={() => setReplyTarget(null)}
        title={t('messages.replyTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setReplyTarget(null)}>{t('common.cancel')}</Button>
            <Button
              variant="danger"
              loading={replyMutation.isPending && replyMutation.variables === 'REJECTED'}
              onClick={() => replyMutation.mutate('REJECTED')}
            >
              {t('messages.reject')}
            </Button>
            <Button
              variant="primary"
              loading={replyMutation.isPending && replyMutation.variables === 'APPROVED'}
              onClick={() => replyMutation.mutate('APPROVED')}
            >
              {t('messages.agree')}
            </Button>
          </>
        }
      >
        <div className="text-xs space-y-3">
          <div className="font-semibold text-gray-800 dark:text-gray-200">{replyTarget?.messageName}</div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('messages.reason')}</label>
            <textarea
              value={replyReason}
              onChange={(e) => setReplyReason(e.target.value)}
              rows={3}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>
      </Modal>

      {/* Create Approval Modal */}
      <CreateApprovalModal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        defaultInitiatorId={ownerId}
        onCreated={invalidateMessages}
      />

      {/* Detail Modal */}
      <Modal
        isOpen={!!detailTarget}
        onClose={() => setDetailTarget(null)}
        title={t('messages.detail')}
        footer={<Button variant="primary" onClick={() => setDetailTarget(null)}>{t('common.close')}</Button>}
      >
        <div className="text-xs space-y-2">
          <div><span className="text-gray-500">{t('messages.type')}: </span><span className="font-semibold">{detailTarget?.type}</span></div>
          <div><span className="text-gray-500">{t('messages.createTime')}: </span><span>{detailTarget?.createTime}</span></div>
          <div>
            <span className="text-gray-500">{t('common.statusActive')}: </span>
            <Badge status={statusBadge(detailQuery.data?.status || detailTarget?.status)}>
              {detailQuery.data?.status || detailTarget?.status || t('messages.statusUnknown')}
            </Badge>
          </div>
          {detailQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
        </div>
      </Modal>
    </div>
  );
};
