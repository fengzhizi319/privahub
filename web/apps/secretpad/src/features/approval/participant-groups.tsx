/**
 * Read-only view of project participants and their vote status, replacing the
 * legacy `VoteInstNodesGraph` (x6 canvas) with a grouped list:
 * initiator node → invited nodes, each with the institution's vote action.
 */
import React from 'react';
import { Badge } from '@secretpad/design-system';
import type { ParticipantNodeInstVOJava, PartyVoteStatusJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';

function voteBadge(action?: string): 'success' | 'processing' | 'error' | 'default' {
  if (action === 'APPROVED') return 'success';
  if (action === 'REJECTED') return 'error';
  if (action === 'REVIEWING') return 'processing';
  return 'default';
}

export const VoteStatusBadge: React.FC<{ action?: string }> = ({ action }) => {
  const { t } = useTranslation();
  if (!action) return <span className="text-gray-400">-</span>;
  return <Badge status={voteBadge(action)}>{t(`approval.voteStatus.${action}`)}</Badge>;
};

export interface ParticipantGroupsProps {
  initiatorId?: string;
  initiatorName?: string;
  groups?: ParticipantNodeInstVOJava[];
  partyVoteStatuses?: PartyVoteStatusJava[];
  /** my institution / node id (highlighted as "mine") */
  selfId?: string;
}

export const ParticipantGroups: React.FC<ParticipantGroupsProps> = ({
  initiatorId,
  initiatorName,
  groups = [],
  partyVoteStatuses = [],
  selfId,
}) => {
  const { t } = useTranslation();
  const statusOf = (instId?: string) => partyVoteStatuses.find((p) => p.participantID === instId);
  const mine = (id?: string) =>
    selfId && id === selfId ? <span className="text-blue-600 dark:text-blue-400">({t('approval.mine')})</span> : null;

  if (groups.length === 0 && partyVoteStatuses.length === 0) {
    return <div className="text-gray-400">{t('approval.noParties')}</div>;
  }

  return (
    <div className="space-y-3">
      {groups.map((g, gi) => (
        <div key={`${g.initiatorNodeId}-${gi}`} className="rounded-lg border border-gray-200 dark:border-gray-700 p-3 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300">
              {t('approval.initiatorTag')}
            </span>
            <span className="text-gray-800 dark:text-gray-200">
              {initiatorName || initiatorId} · {g.initiatorNodeName || g.initiatorNodeId}
            </span>
            {mine(initiatorId)}
          </div>
          {(g.invitees || []).map((inv) => {
            const st = statusOf(inv.instId);
            return (
              <div key={`${inv.inviteeId}-${inv.instId}`} className="flex flex-wrap items-center gap-2 pl-4">
                <span className="px-1.5 py-0.5 rounded bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300">
                  {t('approval.inviteeTag')}
                </span>
                <span className="text-gray-800 dark:text-gray-200">
                  {inv.instName || inv.instId} · {inv.inviteeName || inv.inviteeId}
                </span>
                <VoteStatusBadge action={st?.action} />
                {mine(inv.instId)}
                {st?.reason && (
                  <span className="text-gray-400">
                    {t('approval.rejectReason')}: {st.reason}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      ))}
      {groups.length === 0 &&
        partyVoteStatuses.map((p) => (
          <div key={p.participantID || p.nodeID} className="flex flex-wrap items-center gap-2">
            <span className="text-gray-800 dark:text-gray-200">{p.participantName || p.nodeName}</span>
            <VoteStatusBadge action={p.action} />
            {mine(p.participantID || p.nodeID)}
            {p.reason && (
              <span className="text-gray-400">
                {t('approval.rejectReason')}: {p.reason}
              </span>
            )}
          </div>
        ))}
    </div>
  );
};
