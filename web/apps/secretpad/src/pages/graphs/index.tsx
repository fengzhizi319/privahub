/**
 * Graph 管理列表页。
 *
 * 对应旧前端 `/graphs` 占位/演示页面，在新前端中实现为可管理项目下图列表的独立页面：
 * - 选择项目后展示该项目下的所有 Graph；
 * - 支持创建空白 Graph、删除 Graph；
 * - 支持跳转到 DAG 画布继续编辑所选 Graph。
 */
import React, { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from '@tanstack/react-router';
import { Card, Button, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import { apiClient, type GraphMetaVO } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { dagEditPermissions } from '../dag/edit-permissions';

export const GraphsPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [selectedProjectId, setSelectedProjectId] = useState<string>('');
  const [createOpen, setCreateOpen] = useState(false);
  const [newGraphName, setNewGraphName] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<GraphMetaVO | null>(null);

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });

  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);

  React.useEffect(() => {
    if (!selectedProjectId && projects.length > 0) {
      setSelectedProjectId(projects[0].projectId);
    }
  }, [projects, selectedProjectId]);

  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: !!selectedProjectId,
  });

  const graphs = graphsQuery.data ?? [];

  // 与 DAG 页同一套细粒度权限（旧版 ProjectEditService.canEdit）：新建看项目级，
  // 删除看训练流级（归档 / 待审批项目、P2P 下非我方训练流不可删除）。
  const platform = usePlatform();
  const selectedProject = projects.find((p) => p.projectId === selectedProjectId);
  const permsFor = (graphOwnerId?: string) =>
    dagEditPermissions({
      platformType: platform.platformType,
      ownerId: platform.ownerId,
      project: selectedProject ? { status: selectedProject.status } : null,
      projectsLoaded: projectsQuery.isSuccess && !!selectedProjectId,
      graphOwnerId,
    });
  const canCreate = permsFor().project;

  const createMutation = useMutation({
    mutationFn: () =>
      apiClient.createGraph({
        projectId: selectedProjectId,
        name: newGraphName.trim() || t('graphs.defaultName'),
      }),
    onSuccess: (graphId) => {
      setCreateOpen(false);
      setNewGraphName('');
      queryClient.invalidateQueries({ queryKey: ['graphs', selectedProjectId] });
      toast.success(t('graphs.createSuccess'));
      navigate({
        to: '/dag',
        search: { projectId: selectedProjectId, graphId },
      });
    },
    onError: (e) => {
      toast.error(t('graphs.createError', { message: e instanceof Error ? e.message : String(e) }));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (graph: GraphMetaVO) =>
      apiClient.deleteGraph(selectedProjectId, graph.graphId!),
    onSuccess: () => {
      setDeleteTarget(null);
      queryClient.invalidateQueries({ queryKey: ['graphs', selectedProjectId] });
      toast.success(t('graphs.deleteSuccess'));
    },
    onError: (e) => {
      toast.error(t('graphs.deleteError', { message: e instanceof Error ? e.message : String(e) }));
    },
  });

  const loading = projectsQuery.isLoading || graphsQuery.isLoading;
  const error = projectsQuery.error?.message || graphsQuery.error?.message || null;

  return (
    <div className="space-y-6">
      {/* 页头 + 项目选择 + 新建 */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('graphs.title')}</h2>
          <p className="text-xs text-gray-500">{t('graphs.subtitle')}</p>
        </div>
        <div className="flex flex-col sm:flex-row gap-2 sm:gap-3 w-full sm:w-auto">
          <select
            value={selectedProjectId}
            onChange={(e) => setSelectedProjectId(e.target.value)}
            className="w-full sm:w-auto px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {projects.length === 0 && <option value="">{t('graphs.selectProject')}</option>}
            {projects.map((p) => (
              <option key={p.projectId} value={p.projectId}>
                {p.projectName || p.name}
              </option>
            ))}
          </select>
          {canCreate && (
            <Button variant="primary" className="w-full sm:w-auto" onClick={() => setCreateOpen(true)}>
              {t('graphs.create')}
            </Button>
          )}
        </div>
      </div>

      {error && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      {/* Graph 列表 */}
      {!loading && !error && (
        <Card bodyClassName="p-0">
          <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold uppercase border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('graphs.name')}</th>
                <th className="p-4">{t('graphs.graphId')}</th>
                <th className="p-4">{t('graphs.owner')}</th>
                <th className="p-4">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {graphs.length === 0 && (
                <tr>
                  <td colSpan={4} className="p-4 text-center text-gray-400">
                    {t('graphs.noData')}
                  </td>
                </tr>
              )}
              {graphs.map((graph) => (
                <tr key={graph.graphId}>
                  <td className="p-4 font-semibold text-gray-800 dark:text-gray-200">
                    {graph.name || '-'}
                  </td>
                  <td className="p-4 font-mono text-gray-500">{graph.graphId || '-'}</td>
                  <td className="p-4 text-gray-500">{graph.ownerId || '-'}</td>
                  <td className="p-4">
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="primary"
                        onClick={() =>
                          navigate({
                            to: '/dag',
                            search: {
                              projectId: selectedProjectId,
                              ...(graph.graphId ? { graphId: graph.graphId } : {}),
                            },
                          })
                        }
                      >
                        {t('graphs.openDag')}
                      </Button>
                      {permsFor(graph.ownerId).graph && (
                        <Button size="sm" variant="danger" onClick={() => setDeleteTarget(graph)}>
                          {t('common.delete')}
                        </Button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        </Card>
      )}

      {/* 创建 Graph */}
      <Modal
        isOpen={createOpen}
        onClose={() => {
          setCreateOpen(false);
          setNewGraphName('');
        }}
        title={t('graphs.createTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="primary"
              onClick={() => createMutation.mutate()}
              loading={createMutation.isPending}
            >
              {t('common.create')}
            </Button>
          </>
        }
      >
        <div className="text-xs space-y-3">
          <label className="block font-semibold text-gray-700 dark:text-gray-300">
            {t('graphs.nameLabel')}
          </label>
          <input
            type="text"
            value={newGraphName}
            onChange={(e) => setNewGraphName(e.target.value)}
            placeholder={t('graphs.namePlaceholder')}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          />
        </div>
      </Modal>

      {/* 删除确认 */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={t('graphs.delete')}
        message={t('graphs.deleteConfirm')}
        danger
        loading={deleteMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
