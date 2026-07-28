import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { ProjectVO, ProjectParticipantsDetailVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../../shared/lib/i18n';

export const P2pProjectsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [error, setError] = useState<string | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editing, setEditing] = useState<ProjectVO | null>(null);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [computeMode, setComputeMode] = useState('MPC');
  const [computeFunc, setComputeFunc] = useState('');
  const [archiveTarget, setArchiveTarget] = useState<ProjectVO | null>(null);
  const [participants, setParticipants] = useState<ProjectParticipantsDetailVO | null>(null);

  const projectsQuery = useQuery({
    queryKey: ['p2p-projects'],
    queryFn: () => apiClient.listP2pProjects(),
  });
  const projects = projectsQuery.data ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['p2p-projects'] });

  const saveMutation = useMutation({
    mutationFn: async (input: { editing: ProjectVO | null }): Promise<void> => {
      if (input.editing) {
        await apiClient.updateP2pProject({ projectId: input.editing.projectId, name, description });
        return;
      }
      await apiClient.createP2pProject({ name, description, computeMode, computeFunc: computeFunc || undefined });
    },
    onSuccess: () => {
      setIsModalOpen(false);
      resetForm();
      invalidate();
      toast.success(t('common.save'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const archiveMutation = useMutation({
    mutationFn: (projectId: string) => apiClient.archiveP2pProject(projectId),
    onSuccess: () => {
      setArchiveTarget(null);
      invalidate();
      toast.success(t('common.save'));
    },
    onError: (e) => {
      setArchiveTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const resetForm = () => {
    setName('');
    setDescription('');
    setComputeMode('MPC');
    setComputeFunc('');
    setEditing(null);
  };

  const openCreate = () => {
    resetForm();
    setIsModalOpen(true);
  };

  const openEdit = (project: ProjectVO) => {
    setEditing(project);
    setName(project.projectName || '');
    setDescription(project.description || '');
    setComputeMode(project.computeMode || 'MPC');
    setComputeFunc(project.computeFunc || '');
    setIsModalOpen(true);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    saveMutation.mutate({ editing });
  };

  const handleViewParticipants = async (project: ProjectVO) => {
    if (!project.voteId) return;
    try {
      const res = await apiClient.getP2pParticipants(project.voteId);
      setParticipants(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('p2p.projectsTitle')}</h2>
          <p className="text-xs text-gray-500">{t('p2p.projectsSubtitle')}</p>
        </div>
        <Button variant="primary" icon={<span>＋</span>} onClick={openCreate}>{t('p2p.createProject')}</Button>
      </div>

      {(error || projectsQuery.error) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || projectsQuery.error?.message || '' })}
        </div>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 uppercase tracking-wider font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('p2p.name')}</th>
                <th className="p-4">{t('p2p.computeMode')}</th>
                <th className="p-4">{t('p2p.status')}</th>
                <th className="p-4">{t('p2p.initiator')}</th>
                <th className="p-4">{t('common.action') || 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {projects.length === 0 && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">{t('p2p.noProjects')}</td>
                </tr>
              )}
              {projects.map((project) => (
                <tr key={project.projectId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                  <td className="p-4">
                    <div className="font-semibold text-blue-600 dark:text-blue-400">{project.projectName || '-'}</div>
                    <div className="text-gray-400 font-mono text-[10px]">{project.projectId}</div>
                  </td>
                  <td className="p-4">
                    <span className="px-2 py-0.5 rounded font-mono bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300">
                      {project.computeMode || '-'}
                    </span>
                  </td>
                  <td className="p-4">
                    <Badge status={project.status === 'APPROVED' || project.status === 'AGREE' ? 'success' : 'default'}>
                      {project.status || '-'}
                    </Badge>
                  </td>
                  <td className="p-4 text-gray-500">{project.initiatorName || project.initiator || '-'}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      {project.voteId && (
                        <Button size="sm" variant="ghost" onClick={() => handleViewParticipants(project)}>{t('p2p.participants')}</Button>
                      )}
                      <Button size="sm" variant="ghost" onClick={() => openEdit(project)}>{t('p2p.editProject')}</Button>
                      <Button size="sm" variant="danger" onClick={() => setArchiveTarget(project)}>{t('p2p.archive')}</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Create / Edit Modal */}
      <Modal
        isOpen={isModalOpen}
        onClose={() => { setIsModalOpen(false); resetForm(); }}
        title={editing ? t('p2p.editProjectTitle') : t('p2p.createProjectTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setIsModalOpen(false); resetForm(); }}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={handleSubmit} loading={saveMutation.isPending}>{t('common.confirm')}</Button>
          </>
        }
      >
        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('p2p.name')}</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('p2p.description')}</label>
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            />
          </div>
          {!editing && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('p2p.computeMode')}</label>
              <select
                value={computeMode}
                onChange={(e) => setComputeMode(e.target.value)}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              >
                <option value="MPC">MPC</option>
                <option value="TEE">TEE</option>
              </select>
            </div>
          )}
          {!editing && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('p2p.computeFunc')}</label>
              <input
                type="text"
                value={computeFunc}
                onChange={(e) => setComputeFunc(e.target.value)}
                className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              />
            </div>
          )}
        </form>
      </Modal>

      {/* Participants Modal */}
      <Modal
        isOpen={!!participants}
        onClose={() => setParticipants(null)}
        title={t('p2p.participantsTitle')}
        footer={<Button variant="primary" onClick={() => setParticipants(null)}>{t('common.close')}</Button>}
      >
        {participants && (
          <div className="text-xs space-y-3">
            <div>
              <div className="text-gray-400 mb-1">{t('p2p.initiator')}</div>
              <div className="font-semibold text-gray-800 dark:text-gray-200">
                {participants.initiatorName || participants.initiatorId || '-'}
              </div>
            </div>
            <div>
              <div className="text-gray-400 mb-1">{t('p2p.invitees')}</div>
              <div className="space-y-1">
                {(participants.invitees ?? []).length === 0 && <div className="text-gray-400">-</div>}
                {(participants.invitees ?? []).map((inv, idx) => (
                  <div key={idx} className="font-mono text-gray-600 dark:text-gray-300">
                    {inv.inviteeName || inv.inviteeId || '-'}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </Modal>

      {/* Archive Confirm */}
      <ConfirmDialog
        isOpen={!!archiveTarget}
        title={t('p2p.archive')}
        message={t('p2p.archiveConfirm')}
        danger
        loading={archiveMutation.isPending}
        confirmText={t('p2p.archive')}
        cancelText={t('common.cancel')}
        onConfirm={() => archiveTarget && archiveMutation.mutate(archiveTarget.projectId)}
        onCancel={() => setArchiveTarget(null)}
      />
    </div>
  );
};
