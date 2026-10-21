/**
 * Approve / reject a vote (`message/reply`), shared by the message center and
 * P2P project list. Reject asks for an optional reason of at most 50 chars
 * (legacy "请输50字符以内的理由").
 */
import React, { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, FormField, Modal, Textarea, toast } from '@secretpad/design-system';
import { replyMessageJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';

const REJECT_REASON_MAX = 50;

export interface VoteReplyButtonsProps {
  voteId?: string;
  /** replying party (my node / institution id) */
  participantId: string;
  onDone?: (action: 'APPROVED' | 'REJECTED') => void;
  size?: 'sm' | 'md';
}

export const VoteReplyButtons: React.FC<VoteReplyButtonsProps> = ({ voteId, participantId, onDone, size = 'sm' }) => {
  const { t } = useTranslation();
  const [rejectOpen, setRejectOpen] = useState(false);
  const [reason, setReason] = useState('');

  const mutation = useMutation({
    mutationFn: (action: 'APPROVED' | 'REJECTED') =>
      replyMessageJava({
        action,
        reason: action === 'REJECTED' ? reason.trim() : '',
        voteId: voteId || '',
        voteParticipantId: participantId,
      }),
    onSuccess: (_, action) => {
      toast.success(t('approval.replySuccess'));
      setRejectOpen(false);
      setReason('');
      onDone?.(action);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const disabled = !voteId || !participantId;
  const tooLong = reason.length > REJECT_REASON_MAX;

  return (
    <>
      <div className="flex items-center gap-2">
        <Button
          size={size}
          variant="primary"
          disabled={disabled || mutation.isPending}
          loading={mutation.isPending && mutation.variables === 'APPROVED'}
          onClick={() => mutation.mutate('APPROVED')}
        >
          {t('approval.agree')}
        </Button>
        <Button size={size} variant="outline" disabled={disabled || mutation.isPending} onClick={() => setRejectOpen(true)}>
          {t('approval.reject')}
        </Button>
      </div>
      <Modal
        isOpen={rejectOpen}
        onClose={() => setRejectOpen(false)}
        title={t('approval.rejectConfirm')}
        width="max-w-md"
        footer={
          <>
            <Button variant="ghost" onClick={() => setRejectOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              disabled={tooLong}
              loading={mutation.isPending && mutation.variables === 'REJECTED'}
              onClick={() => mutation.mutate('REJECTED')}
            >
              {t('approval.reject')}
            </Button>
          </>
        }
      >
        <FormField
          label={t('approval.rejectReason')}
          error={tooLong ? t('approval.rejectReasonTooLong', { max: REJECT_REASON_MAX }) : undefined}
          help={`${reason.length}/${REJECT_REASON_MAX}`}
        >
          <Textarea
            className="w-full"
            rows={3}
            maxLength={REJECT_REASON_MAX}
            placeholder={t('approval.rejectReasonPlaceholder', { max: REJECT_REASON_MAX })}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </FormField>
      </Modal>
    </>
  );
};
