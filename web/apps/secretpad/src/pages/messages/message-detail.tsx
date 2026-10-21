/**
 * Message detail drawer (legacy `message-info/message-info.view.tsx` +
 * `info-content.tsx`): `message/detail` rendered per vote type.
 */
import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { Drawer, Empty } from '@secretpad/design-system';
import { getMessageDetailJava, isMessageVoteType, type MessageDetailVOJava, type MessageVOJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { ParticipantGroups, VoteStatusBadge } from '../../features/approval/participant-groups';
import { VoteReplyButtons } from '../../features/approval/vote-reply';
import type { MessageTab } from './helpers';

const Row: React.FC<{ label: React.ReactNode; children: React.ReactNode }> = ({ label, children }) => (
  <div className="grid grid-cols-[7rem_1fr] gap-2 py-1.5">
    <div className="text-gray-400">{label}</div>
    <div className="text-gray-800 dark:text-gray-200 break-all">{children || '-'}</div>
  </div>
);

const NodeRouteInfo: React.FC<{ info: MessageDetailVOJava }> = ({ info }) => {
  const { t } = useTranslation();
  return (
    <div>
      {info.status === 'REJECTED' && <Row label={t('msgCenter.rejectReason')}>{info.reason}</Row>}
      <Row label={t('msgCenter.initiatorNode')}>{info.initiatorNodeName}</Row>
      <Row label={t('msgCenter.nodeName')}>{info.nodeName}</Row>
      <Row label={t('msgCenter.nodeId')}>
        <span className="font-mono">{info.nodeID}</span>
      </Row>
      <Row label={t('msgCenter.nodeAddress')}>{info.url}</Row>
      <Row label={t('msgCenter.routeMode')}>
        {info.isSingle ? t('msgCenter.routeSingle') : t('msgCenter.routeDouble')}
      </Row>
    </div>
  );
};

const TeeDownloadInfo: React.FC<{ info: MessageDetailVOJava; selfId: string }> = ({ info, selfId }) => {
  const { t } = useTranslation();
  const mode = info.project?.computeMode || 'MPC';
  return (
    <div className="space-y-3">
      <div>
        <Row label={t('msgCenter.resultTable')}>{info.messageName}</Row>
        <Row label={t('msgCenter.sourceProject')}>{info.project?.projectName}</Row>
        <Row label={t('msgCenter.graphName')}>{info.graphName}</Row>
        <Row label={t('msgCenter.generatedAt')}>{info.project?.gmtCreated}</Row>
        <Row label={t('msgCenter.computeMode')}>{t(`msgCenter.computeModes.${mode}`)}</Row>
        <Row label={t('msgCenter.taskId')}>
          <span className="font-mono">{info.taskID}</span>
        </Row>
      </div>
      <div>
        <div className="font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('msgCenter.parties')}</div>
        <ParticipantGroups partyVoteStatuses={info.partyVoteStatuses} selfId={selfId} />
      </div>
      {(info.tableColumns || []).length > 0 && (
        <div>
          <div className="font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('msgCenter.tableColumns')}</div>
          <div className="overflow-x-auto">
          <table className="w-full text-left border border-gray-200 dark:border-gray-700">
            <thead className="text-gray-500 bg-gray-50 dark:bg-gray-800">
              <tr>
                <th className="p-1.5">{t('msgCenter.colName')}</th>
                <th className="p-1.5">{t('msgCenter.colType')}</th>
                <th className="p-1.5">{t('msgCenter.colComment')}</th>
              </tr>
            </thead>
            <tbody>
              {(info.tableColumns || []).map((c) => (
                <tr key={c.colName} className="border-t border-gray-100 dark:border-gray-800">
                  <td className="p-1.5 font-mono">{c.colName}</td>
                  <td className="p-1.5">{c.colType}</td>
                  <td className="p-1.5">{c.colComment || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        </div>
      )}
    </div>
  );
};

const ProjectInviteInfo: React.FC<{ info: MessageDetailVOJava; selfId: string }> = ({ info, selfId }) => {
  const { t } = useTranslation();
  const func = info.computeFunc || 'DAG';
  const mode = info.computeMode || 'MPC';
  return (
    <div className="space-y-3">
      <div>
        <Row label={t('msgCenter.projectName')}>{info.projectName}</Row>
        <Row label={t('msgCenter.initiatorInst')}>{info.initiatorName}</Row>
        <Row label={t('msgCenter.initiatorNode')}>
          {(info.participantNodeInstVOS || []).map((i) => i.initiatorNodeName || i.initiatorNodeId).join('、')}
        </Row>
        <Row label={t('msgCenter.createdAt')}>{info.gmtCreated}</Row>
        <Row label={t('msgCenter.computeFunc')}>{t(`msgCenter.computeFuncs.${func}`)}</Row>
        <Row label={t('msgCenter.computeMode')}>{t(`msgCenter.computeModes.${mode}`)}</Row>
        <Row label={t('msgCenter.projectDesc')}>{info.projectDesc}</Row>
      </div>
      <div>
        <div className="font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('msgCenter.invitedNodes')}</div>
        <ParticipantGroups
          initiatorId={info.initiatorId}
          initiatorName={info.initiatorName}
          groups={info.participantNodeInstVOS}
          partyVoteStatuses={info.partyVoteStatuses}
          selfId={selfId}
        />
      </div>
    </div>
  );
};

export const MessageDetailContent: React.FC<{ info: MessageDetailVOJava; selfId: string }> = ({ info, selfId }) => {
  switch (info.type) {
    case 'NODE_ROUTE':
      return <NodeRouteInfo info={info} />;
    case 'TEE_DOWNLOAD':
      return <TeeDownloadInfo info={info} selfId={selfId} />;
    case 'PROJECT_CREATE':
    case 'PROJECT_ARCHIVE':
      return <ProjectInviteInfo info={info} selfId={selfId} />;
    default:
      return <Empty>-</Empty>;
  }
};

export interface MessageDetailDrawerProps {
  message: MessageVOJava;
  tab: MessageTab;
  ownerId: string;
  onClose: () => void;
  onReplied: () => void;
  title: React.ReactNode;
}

export const MessageDetailDrawer: React.FC<MessageDetailDrawerProps> = ({
  message,
  tab,
  ownerId,
  onClose,
  onReplied,
  title,
}) => {
  const { t } = useTranslation();
  const detailQuery = useQuery({
    queryKey: ['message-detail-java', message.voteID, tab, ownerId],
    queryFn: () =>
      getMessageDetailJava({
        ownerId,
        voteId: message.voteID || '',
        isInitiator: tab === 'apply',
        voteType: message.type,
      }),
    // 后端要求 voteType ∈ TEE_DOWNLOAD / NODE_ROUTE / PROJECT_CREATE / PROJECT_ARCHIVE。
    enabled: !!message.voteID && !!ownerId && isMessageVoteType(message.type),
  });
  const detail = detailQuery.data;
  const status = detail?.status || message.status;

  return (
    <Drawer
      isOpen
      onClose={onClose}
      width="max-w-xl"
      title={
        <span className="flex items-center gap-2">
          <span className="truncate">{title}</span>
          <span className="text-xs font-normal text-gray-400">
            {tab === 'process' ? t('msgCenter.myStatus') : t('msgCenter.currentStatus')}
          </span>
          <VoteStatusBadge action={status} />
        </span>
      }
      footer={
        tab === 'process' && status === 'REVIEWING' ? (
          <VoteReplyButtons
            size="md"
            voteId={message.voteID}
            participantId={ownerId}
            onDone={() => {
              onReplied();
              onClose();
            }}
          />
        ) : undefined
      }
    >
      <div className="text-xs">
        {detailQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
        {detailQuery.error && (
          <div className="text-rose-500">{t('common.error', { message: detailQuery.error.message })}</div>
        )}
        {detail && <MessageDetailContent info={{ ...detail, type: detail.type || message.type }} selfId={ownerId} />}
      </div>
    </Drawer>
  );
};
