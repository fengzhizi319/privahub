/**
 * P2P projects (legacy `modules/p2p-project-list`).
 *
 * - filters: all / mine (initiator) / invited, status, compute mode, name search
 * - invited parties approve / reject the PROJECT_CREATE vote (`message/reply`)
 * - edit name / description (initiator, not archived) via `p2p/project/update`
 * - archive: approved projects → `approval/create PROJECT_ARCHIVE {projectId}`,
 *   never-approved projects → `p2p/project/archive` (legacy behaviour)
 * - create: `p2p/project/create` + `approval/create PROJECT_CREATE`
 * - detail drawer (parties / graphs / jobs); "enter project" once fully approved
 */
import React, { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  FormField,
  Input,
  Modal,
  Select,
  Tabs,
  Textarea,
  toast,
} from '@secretpad/design-system';
import {
  archiveP2pProjectJava,
  createApprovalJava,
  listP2pProjectsJava,
  updateP2pProjectJava,
  type P2pProjectVOJava,
} from '@secretpad/api-client';
import { useTranslation } from '../../../shared/lib/i18n';
import { usePlatform } from '../../../shared/lib/platform';
import { VoteStatusBadge } from '../../../features/approval/participant-groups';
import { VoteReplyButtons } from '../../../features/approval/vote-reply';
import { CreateP2pProjectDrawer } from './create-project-drawer';
import { P2pProjectDetailDrawer, type DetailTab } from './project-detail-drawer';
import {
  PROJECT_DESC_MAX,
  PROJECT_NAME_MAX,
  PROJECT_STATUS,
  archiveNeedsVote,
  canArchiveProject,
  canEditProject,
  canEnterProject,
  checkAllApproved,
  filterProjects,
  hasSelfPendingVote,
  isInitiator,
  validateProjectDesc,
  validateProjectName,
  type ProjectOwnerTab,
} from './helpers';

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export const P2pProjectsPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { ownerId, supportsMpc, supportsTee } = usePlatform();

  const [tab, setTab] = useState<ProjectOwnerTab>('');
  const [status, setStatus] = useState('');
  const [mode, setMode] = useState('');
  const [keyword, setKeyword] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<P2pProjectVOJava | null>(null);
  const [archiveTarget, setArchiveTarget] = useState<P2pProjectVOJava | null>(null);
  const [detail, setDetail] = useState<{ project: P2pProjectVOJava; tab: DetailTab } | null>(null);

  const listQuery = useQuery({ queryKey: ['p2p-projects-java'], queryFn: listP2pProjectsJava });
  const all = useMemo(() => listQuery.data ?? [], [listQuery.data]);
  const projects = useMemo(
    () => filterProjects(all, { tab, status, mode, keyword }, ownerId),
    [all, tab, status, mode, keyword, ownerId],
  );

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['p2p-projects-java'] });
    queryClient.invalidateQueries({ queryKey: ['messages-java'] });
  };

  const archiveMutation = useMutation({
    mutationFn: async (p: P2pProjectVOJava) => {
      if (archiveNeedsVote(p)) {
        await createApprovalJava({
          initiatorId: ownerId,
          voteType: 'PROJECT_ARCHIVE',
          voteConfig: { projectId: p.projectId || '' },
        });
      } else {
        await archiveP2pProjectJava(p.projectId || '');
      }
    },
    onSuccess: (_, p) => {
      toast.success(t('p2pProjects.archiveSuccess', { name: p.projectName || '' }));
      setArchiveTarget(null);
      setDetail(null);
      refresh();
    },
    onError: (e) => toast.error(errText(e)),
  });

  const enterProject = (p: P2pProjectVOJava) =>
    navigate({
      to: '/dag',
      search: { projectId: p.projectId, mode: p.computeMode || 'MPC', type: p.computeFunc || 'DAG' },
    });

  const projectActions = (p: P2pProjectVOJava, size: 'sm' | 'md' = 'sm') => (
    <div className="flex items-center gap-2">
      {canArchiveProject(p, ownerId) && (
        <Button size={size} variant="outline" onClick={() => setArchiveTarget(p)}>
          {t('p2p.archive')}
        </Button>
      )}
      <span title={canEnterProject(p) ? undefined : t('p2pProjects.enterDisabled')}>
        <Button size={size} variant="primary" disabled={!canEnterProject(p)} onClick={() => enterProject(p)}>
          {t('msgCenter.enterProject')}
        </Button>
      </span>
    </div>
  );

  const modeOptions = [
    { value: '', label: t('p2pProjects.allModes') },
    ...(supportsMpc ? [{ value: 'MPC', label: t('msgCenter.computeModes.MPC') }] : []),
    ...(supportsTee ? [{ value: 'TEE', label: t('msgCenter.computeModes.TEE') }] : []),
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('p2p.projectsTitle')}</h2>
          <p className="text-xs text-gray-500">{t('p2p.projectsSubtitle')}</p>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}>
          {t('p2p.createProject')}
        </Button>
      </div>

      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
        <Tabs
          activeKey={tab || 'all'}
          onChange={(k) => setTab(k === 'all' ? '' : (k as ProjectOwnerTab))}
          items={[
            { key: 'all', label: t('p2pProjects.tabAll') },
            { key: 'mine', label: t('p2pProjects.tabMine') },
            { key: 'invited', label: t('p2pProjects.tabInvited') },
          ]}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="w-48"
            placeholder={t('p2pProjects.searchPlaceholder')}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
          />
          <Select aria-label={t('p2p.computeMode')} value={mode} onChange={setMode} options={modeOptions} />
          <Select
            aria-label={t('p2p.status')}
            value={status}
            onChange={setStatus}
            options={[
              { value: '', label: t('p2pProjects.allStatus') },
              { value: 'REVIEWING', label: t('p2pProjects.status.REVIEWING') },
              { value: 'APPROVED', label: t('p2pProjects.status.APPROVED') },
              { value: 'ARCHIVED', label: t('p2pProjects.status.ARCHIVED') },
            ]}
          />
        </div>
      </div>

      {listQuery.error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: listQuery.error.message })}
        </div>
      )}
      {listQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
      {!listQuery.isLoading && projects.length === 0 && (
        <div className="text-xs text-gray-400 text-center py-10">{t('p2p.noProjects')}</div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {projects.map((p) => {
          const approved = checkAllApproved(p);
          const selfPending = hasSelfPendingVote(p, ownerId);
          return (
            <Card key={p.projectId} className="relative">
              <div className="space-y-3 text-xs">
                <div className="flex items-center gap-2">
                  <span title={p.computeMode === 'TEE' ? p.teeNodeId : undefined}>
                    <Badge>{t(`msgCenter.computeModes.${p.computeMode === 'TEE' ? 'TEE' : 'MPC'}`)}</Badge>
                  </span>
                  <Badge>{t(`msgCenter.computeFuncs.${p.computeFunc || 'DAG'}`)}</Badge>
                  {p.status === PROJECT_STATUS.ARCHIVED && <Badge status="default">{t('p2pProjects.status.ARCHIVED')}</Badge>}
                  <button
                    type="button"
                    className="flex-1 min-w-0 text-left font-semibold text-sm text-gray-900 dark:text-gray-100 truncate hover:text-blue-600"
                    title={p.projectName}
                    onClick={() => setDetail({ project: p, tab: 'parties' })}
                  >
                    {p.projectName}
                  </button>
                  {canEditProject(p, ownerId) && (
                    <Button size="sm" variant="ghost" onClick={() => setEditing(p)}>
                      {t('p2p.editProject')}
                    </Button>
                  )}
                </div>
                <p className="text-gray-500 truncate">{p.description || t('p2pProjects.noDesc')}</p>

                {approved ? (
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {(
                      [
                        ['parties', t('p2pProjects.parties'), (p.partyVoteInfos?.length ?? 0) + 1],
                        ['graphs', t('p2pProjects.graphs'), p.graphCount ?? 0],
                        ['jobs', t('p2pProjects.jobs'), p.jobCount ?? 0],
                      ] as const
                    ).map(([key, label, count]) => (
                      <button
                        type="button"
                        key={key}
                        className="rounded-lg bg-gray-50 dark:bg-gray-800 p-2 hover:bg-gray-100 dark:hover:bg-gray-700"
                        onClick={() => setDetail({ project: p, tab: key })}
                      >
                        <div className="text-gray-400">{label}</div>
                        <div className="text-base font-semibold text-blue-600 dark:text-blue-400">{count}</div>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="space-y-1.5 rounded-lg bg-gray-50 dark:bg-gray-800 p-2">
                    <div className="flex items-center gap-2">
                      <span className="px-1.5 rounded bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300">
                        {t('approval.initiatorTag')}
                      </span>
                      <span className="truncate">{p.initiatorName || p.initiator}</span>
                      {isInitiator(p, ownerId) && <span className="text-blue-600">({t('approval.mine')})</span>}
                    </div>
                    {(p.partyVoteInfos || []).map((party) => (
                      <div key={party.partyId} className="flex flex-wrap items-center gap-2">
                        <span className="px-1.5 rounded bg-gray-200 dark:bg-gray-700">{t('approval.inviteeTag')}</span>
                        <span className="truncate">{party.partyName || party.partyId}</span>
                        <span title={party.action === 'REJECTED' ? party.reason || t('p2pProjects.noReason') : undefined}>
                          <VoteStatusBadge action={party.action} />
                        </span>
                        {party.partyId === ownerId && <span className="text-blue-600">({t('approval.mine')})</span>}
                      </div>
                    ))}
                    {selfPending && (
                      <div className="pt-1">
                        <VoteReplyButtons voteId={p.voteId} participantId={ownerId} onDone={refresh} />
                      </div>
                    )}
                  </div>
                )}

                <div className="text-gray-400">{t('p2pProjects.createdAt', { time: p.gmtCreate || '-' })}</div>
                <div className="flex justify-end">{projectActions(p)}</div>
              </div>
            </Card>
          );
        })}
      </div>

      <CreateP2pProjectDrawer isOpen={createOpen} onClose={() => setCreateOpen(false)} onCreated={refresh} />

      {editing && <EditProjectModal project={editing} onClose={() => setEditing(null)} onSaved={refresh} />}

      {detail && (
        <P2pProjectDetailDrawer
          project={detail.project}
          initialTab={detail.tab}
          ownerId={ownerId}
          onClose={() => setDetail(null)}
          onChanged={refresh}
          actions={projectActions(detail.project, 'md')}
        />
      )}

      <ConfirmDialog
        isOpen={!!archiveTarget}
        title={t('p2pProjects.archiveTitle', { name: archiveTarget?.projectName || '' })}
        message={archiveTarget && archiveNeedsVote(archiveTarget) ? t('p2pProjects.archiveVoteDesc') : t('p2p.archiveConfirm')}
        danger
        loading={archiveMutation.isPending}
        confirmText={t('p2p.archive')}
        cancelText={t('common.cancel')}
        onConfirm={() => archiveTarget && archiveMutation.mutate(archiveTarget)}
        onCancel={() => setArchiveTarget(null)}
      />
    </div>
  );
};

const EditProjectModal: React.FC<{ project: P2pProjectVOJava; onClose: () => void; onSaved: () => void }> = ({
  project,
  onClose,
  onSaved,
}) => {
  const { t } = useTranslation();
  const [name, setName] = useState(project.projectName || '');
  const [description, setDescription] = useState(project.description || '');
  const [submitted, setSubmitted] = useState(false);
  const nameError = validateProjectName(name);
  const descError = validateProjectDesc(description);

  const mutation = useMutation({
    mutationFn: () =>
      updateP2pProjectJava({ projectId: project.projectId || '', name: name.trim(), description: description.trim() }),
    onSuccess: () => {
      toast.success(t('p2pProjects.updateSuccess'));
      onSaved();
      onClose();
    },
    onError: (e) => toast.error(errText(e)),
  });

  return (
    <Modal
      isOpen
      onClose={onClose}
      title={t('p2p.editProjectTitle')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={mutation.isPending}
            onClick={() => {
              setSubmitted(true);
              if (!nameError && !descError) mutation.mutate();
            }}
          >
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <FormField label={t('p2p.name')} required error={submitted && nameError ? t(nameError, { max: PROJECT_NAME_MAX }) : undefined}>
          <Input className="w-full" value={name} maxLength={PROJECT_NAME_MAX} onChange={(e) => setName(e.target.value)} />
        </FormField>
        <FormField label={t('p2p.description')} error={submitted && descError ? t(descError, { max: PROJECT_DESC_MAX }) : undefined}>
          <Textarea
            className="w-full"
            rows={3}
            maxLength={PROJECT_DESC_MAX}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </FormField>
      </div>
    </Modal>
  );
};
