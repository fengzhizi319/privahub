/**
 * P2P project detail drawer (legacy `p2p-project-detail/project-detail-drawer.tsx`):
 * parties (`p2p/project/participants`), graphs (`graph/list`) and jobs
 * (`project/job/list`). Under review only the parties are shown and my
 * pending vote can be answered.
 */
import React, { useEffect, useState } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Badge, Drawer, Pagination, Tabs } from '@secretpad/design-system';
import {
  apiClient,
  getP2pProjectParticipantsJava,
  listProjectJobSummariesJava,
  type P2pProjectVOJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../../shared/lib/i18n';
import { ParticipantGroups } from '../../../features/approval/participant-groups';
import { VoteReplyButtons } from '../../../features/approval/vote-reply';
import { jobStatusBadge } from '../../periodic-tasks/helpers';
import { PROJECT_STATUS, hasSelfPendingVote } from './helpers';

const JOB_PAGE_SIZE = 10;

export type DetailTab = 'parties' | 'graphs' | 'jobs';

export interface P2pProjectDetailDrawerProps {
  project: P2pProjectVOJava;
  initialTab?: DetailTab;
  ownerId: string;
  onClose: () => void;
  onChanged: () => void;
  /** archive / enter buttons for approved projects */
  actions?: React.ReactNode;
}

export const P2pProjectDetailDrawer: React.FC<P2pProjectDetailDrawerProps> = ({
  project,
  initialTab = 'parties',
  ownerId,
  onClose,
  onChanged,
  actions,
}) => {
  const { t } = useTranslation();
  const [tab, setTab] = useState<DetailTab>(initialTab);
  const [jobPage, setJobPage] = useState(1);
  const projectId = project.projectId || '';
  const reviewing = project.status === PROJECT_STATUS.REVIEWING;

  useEffect(() => setTab(initialTab), [initialTab, projectId]);

  const partiesQuery = useQuery({
    queryKey: ['p2p-participants', project.voteId],
    queryFn: () => getP2pProjectParticipantsJava(project.voteId || ''),
    enabled: !!project.voteId,
  });
  const graphsQuery = useQuery({
    queryKey: ['graphs', projectId],
    queryFn: () => apiClient.getGraphs(projectId),
    enabled: !!projectId && !reviewing,
  });
  const jobsQuery = useQuery({
    queryKey: ['p2p-project-jobs', projectId, jobPage],
    queryFn: () => listProjectJobSummariesJava({ projectId, pageNum: jobPage, pageSize: JOB_PAGE_SIZE }),
    enabled: !!projectId && !reviewing && tab === 'jobs',
    placeholderData: keepPreviousData,
  });

  const parties = partiesQuery.data;
  const partiesView = (
    <div className="text-xs">
      {partiesQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
      {partiesQuery.error && <div className="text-rose-500">{t('common.error', { message: partiesQuery.error.message })}</div>}
      {parties && (
        <ParticipantGroups
          initiatorId={parties.initiatorId || project.initiator}
          initiatorName={parties.initiatorName || project.initiatorName}
          groups={parties.participantNodeInstVOS}
          partyVoteStatuses={parties.partyVoteStatuses}
          selfId={ownerId}
        />
      )}
      {!project.voteId && (
        <div className="space-y-1">
          {(project.partyVoteInfos || []).map((p) => (
            <div key={p.partyId}>
              {p.partyName || p.partyId} · {p.action ? t(`approval.voteStatus.${p.action}`) : '-'}
            </div>
          ))}
        </div>
      )}
    </div>
  );

  const graphs = graphsQuery.data ?? [];
  const jobs = jobsQuery.data?.list ?? [];

  const selfPending = hasSelfPendingVote(project, ownerId);

  return (
    <Drawer
      isOpen
      onClose={onClose}
      width="max-w-2xl"
      title={
        <span className="flex items-center gap-2">
          <Badge>{t(`msgCenter.computeModes.${project.computeMode === 'TEE' ? 'TEE' : 'MPC'}`)}</Badge>
          <Badge>{t(`msgCenter.computeFuncs.${project.computeFunc || 'DAG'}`)}</Badge>
          <span className="truncate">{project.projectName}</span>
        </span>
      }
      footer={
        selfPending ? (
          <VoteReplyButtons
            size="md"
            voteId={project.voteId}
            participantId={ownerId}
            onDone={() => {
              onChanged();
              onClose();
            }}
          />
        ) : (
          actions
        )
      }
    >
      {reviewing ? (
        partiesView
      ) : (
        <div className="space-y-4">
          <Tabs
            activeKey={tab}
            onChange={(k) => setTab(k as DetailTab)}
            items={[
              {
                key: 'parties',
                label: t('p2pProjects.tabParties', { count: parties?.partyVoteStatuses?.length ?? (project.partyVoteInfos?.length ?? 0) + 1 }),
              },
              { key: 'graphs', label: t('p2pProjects.tabGraphs', { count: graphsQuery.data ? graphs.length : project.graphCount ?? 0 }) },
              { key: 'jobs', label: t('p2pProjects.tabJobs', { count: project.jobCount ?? 0 }) },
            ]}
          />
          {tab === 'parties' && partiesView}
          {tab === 'graphs' && (
            <div className="text-xs space-y-1">
              {graphsQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
              {!graphsQuery.isLoading && graphs.length === 0 && (
                <div className="text-gray-400 text-center py-6">{t('p2pProjects.noGraphs')}</div>
              )}
              {graphs.map((g) => (
                <div
                  key={g.graphId}
                  className="p-2 rounded-lg border border-gray-200 dark:border-gray-700 text-gray-800 dark:text-gray-200 truncate"
                >
                  {g.name || g.graphId}
                </div>
              ))}
            </div>
          )}
          {tab === 'jobs' && (
            <div className="text-xs space-y-2">
              {jobsQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
              {jobsQuery.error && <div className="text-rose-500">{t('common.error', { message: jobsQuery.error.message })}</div>}
              {!jobsQuery.isLoading && jobs.length === 0 && (
                <div className="text-gray-400 text-center py-6">{t('p2pProjects.noJobs')}</div>
              )}
              {jobs.map((j) => (
                <div
                  key={j.jobId}
                  className="p-2 rounded-lg border border-gray-200 dark:border-gray-700 flex items-center justify-between gap-2"
                >
                  <div className="min-w-0">
                    <div className="font-mono text-gray-800 dark:text-gray-200 truncate">{j.name || j.jobId}</div>
                    <div className="text-gray-400">
                      {j.gmtCreate} · {j.finishedTaskCount ?? 0}/{j.taskCount ?? 0}
                    </div>
                  </div>
                  <Badge status={jobStatusBadge(j.status)}>{j.status || '-'}</Badge>
                </div>
              ))}
              {(jobsQuery.data?.total ?? 0) > JOB_PAGE_SIZE && (
                <Pagination page={jobPage} pageSize={JOB_PAGE_SIZE} total={jobsQuery.data?.total ?? 0} onChange={setJobPage} />
              )}
            </div>
          )}
        </div>
      )}
    </Drawer>
  );
};
