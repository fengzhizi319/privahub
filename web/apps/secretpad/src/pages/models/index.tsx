import { QueryFeedback } from '../../shared/ui/query-feedback';
import { FilterBar } from '../../shared/ui/filter-bar';
import { RowActions } from '../../shared/ui/row-actions';
import { TableView } from '../../shared/ui/table-view';
import React, { useEffect, useMemo, useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Button, Card, ConfirmDialog, Input, Pagination, Select, toast } from '@secretpad/design-system';
import {
  apiClient,
  deleteModelJava,
  deleteModelServingJava,
  discardModelJava,
  listP2pProjectsJava,
  pageModelsJava,
  type ModelPackJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { useDebounced } from '@/shared/lib/use-debounced';
import { errorText, formatTime } from '@/features/cooperative-node/format';
import { PublishModelDrawer } from './publish-drawer';
import { PackModelModal } from './pack-modal';
import { ServingDetailModal } from './serving-detail-modal';

/** Legacy model-manager status map (ModelStatus). */
const STATUS_BADGE: Record<string, 'warning' | 'success' | 'default' | 'error' | 'processing'> = {
  INIT: 'warning',
  PUBLISHED: 'success',
  OFFLINE: 'default',
  DISCARDED: 'default',
  PUBLISH_FAIL: 'error',
  PUBLISHING: 'processing',
};
const CAN_PUBLISH = ['INIT', 'OFFLINE', 'PUBLISH_FAIL'];
const CAN_DISCARD = ['INIT', 'OFFLINE', 'PUBLISH_FAIL'];
const CAN_OFFLINE = ['PUBLISHED', 'PUBLISHING'];
const CAN_DELETE = ['DISCARDED'];
const CAN_VIEW_SERVING = ['PUBLISHED', 'OFFLINE'];

type Confirm = { kind: 'discard' | 'offline' | 'delete'; model: ModelPackJava } | null;

/**
 * 模型管理（legacy model-manager）：model/page 服务端分页 + searchKey 搜索 +
 * modelStats 过滤 + 提交时间排序；发布（partyConfigs 特征映射 + 资源配置）、
 * 下线（model/serving/delete）、废弃、删除、查看服务详情；发布中每 10s 轮询。
 */
export const ModelsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const platform = usePlatform();

  const [projectId, setProjectId] = useState('');
  const [search, setSearch] = useState('');
  const debounced = useDebounced(search);
  const [stats, setStats] = useState('');
  const [sortOrder, setSortOrder] = useState<'ASC' | 'DESC' | null>(null);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  const [publish, setPublish] = useState<{ modelId?: string } | null>(null);
  const [packOpen, setPackOpen] = useState(false);
  const [detail, setDetail] = useState<ModelPackJava | null>(null);
  const [confirm, setConfirm] = useState<Confirm>(null);

  const projectsQuery = useQuery({
    queryKey: ['models-projects', platform.isP2p],
    queryFn: async () => {
      if (platform.isP2p) {
        const list = await listP2pProjectsJava();
        return list.map((p) => ({ projectId: p.projectId || '', projectName: p.projectName || p.projectId || '', status: p.status }));
      }
      const list = await apiClient.getProjects();
      return list.map((p) => ({
        projectId: p.projectId,
        projectName: p.projectName,
        status: (p as unknown as { status?: string }).status,
      }));
    },
  });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);
  useEffect(() => {
    if (!projectId && projects.length) setProjectId(projects[0].projectId);
  }, [projects, projectId]);
  const archived = projects.find((p) => p.projectId === projectId)?.status === 'ARCHIVED';
  const canWrite = !archived;

  useEffect(() => setPage(1), [debounced, stats, sortOrder, projectId]);

  const listQuery = useQuery({
    queryKey: ['models-page', projectId, page, size, debounced, stats, sortOrder],
    queryFn: () =>
      pageModelsJava({
        projectId,
        page,
        size,
        searchKey: debounced,
        modelStats: stats,
        sort: sortOrder ? { gmtCreate: sortOrder } : {},
      }),
    enabled: !!projectId,
    placeholderData: keepPreviousData,
    refetchInterval: (q) => (q.state.data?.modelPacks.some((m) => m.modelStats === 'PUBLISHING') ? 10_000 : false),
  });
  const models = listQuery.data?.modelPacks ?? [];
  const total = listQuery.data?.total ?? 0;
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['models-page'] });

  // Publishable models for the drawer (legacy getCanSubmitModelList; AUTONOMY: only own models).
  const publishableQuery = useQuery({
    queryKey: ['models-publishable', projectId],
    queryFn: () => pageModelsJava({ projectId, page: 1, size: 1000 }),
    enabled: !!publish && !!projectId,
  });
  const publishable = (publishableQuery.data?.modelPacks ?? []).filter(
    (m) => CAN_PUBLISH.includes(m.modelStats || '') && (!platform.isAutonomy || m.ownerId === platform.ownerId),
  );

  const action = useMutation({
    mutationFn: async (c: NonNullable<Confirm>) => {
      if (c.kind === 'discard') return discardModelJava(c.model.modelId!);
      if (c.kind === 'offline') return deleteModelServingJava(c.model.servingId!);
      return deleteModelJava(c.model.modelId!, c.model.ownerId);
    },
    onSuccess: (_, c) => {
      toast.success(t(`models.${c.kind}Success`));
      setConfirm(null);
      invalidate();
    },
    onError: (e) => {
      setConfirm(null);
      toast.error(errorText(e));
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('models.title')}</h2>
          <p className="text-xs text-gray-500">{t('models.subtitle')}</p>
        </div>
      <FilterBar onReset={()=>{setSearch('');setStats('');setPage(1);}}>
          <Select
            className="!w-48"
            value={projectId}
            onChange={setProjectId}
            options={projects.map((p) => ({ value: p.projectId, label: p.projectName }))}
          />
          <Input className="!w-48" value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('models.searchPlaceholder')} />
          <Select
            className="!w-36"
            value={stats}
            onChange={setStats}
            placeholder={t('models.allStatus')}
            options={Object.keys(STATUS_BADGE).map((s) => ({ value: s, label: t(`models.status.${s}`) }))}
          />
          {canWrite && (
            <>
              <Button variant="outline" disabled={!projectId} onClick={() => setPackOpen(true)}>
                {t('models.pack')}
              </Button>
              <Button variant="primary" disabled={!projectId} onClick={() => setPublish({})}>
                {t('models.publish')}
              </Button>
            </>
          )}
        </FilterBar>
      </div>

      {archived && <div className="text-xs text-amber-600">{t('models.archivedHint')}</div>}
      {(listQuery.error || projectsQuery.error) && (
        <QueryFeedback error={listQuery.error || projectsQuery.error} onRetry={()=>{void listQuery.refetch();void projectsQuery.refetch();}}/>
      )}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <TableView id="models"><table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-3">{t('models.name')}</th>
                <th className="p-3">{t('models.modelId')}</th>
                <th className="p-3">{t('models.desc')}</th>
                <th className="p-3">{t('models.publishStatus')}</th>
                <th
                  className="p-3 cursor-pointer select-none"
                  onClick={() => setSortOrder((s) => (s === null ? 'DESC' : s === 'DESC' ? 'ASC' : null))}
                >
                  {t('models.submitTime')} {sortOrder === 'ASC' ? '↑' : sortOrder === 'DESC' ? '↓' : '↕'}
                </th>
                <th className="p-3">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {listQuery.isLoading && (
                <tr>
                  <td colSpan={6} className="p-4 text-center text-gray-400">
                    <QueryFeedback loading/>
                  </td>
                </tr>
              )}
              {!listQuery.isLoading && !listQuery.error && models.length === 0 && (
                <tr>
                  <td colSpan={6} className="p-4 text-center text-gray-400">
                    <QueryFeedback empty label={t('models.noData')}/>
                  </td>
                </tr>
              )}
              {models.map((m) => {
                const s = m.modelStats || '';
                return (
                  <tr key={m.modelId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                    <td className="p-3 font-semibold">
                      <button type="button" className="text-blue-600 dark:text-blue-400 hover:underline" onClick={() => setDetail(m)}>
                        {m.modelName || m.modelId}
                      </button>
                    </td>
                    <td className="p-3 font-mono text-gray-500">{m.modelId}</td>
                    <td className="p-3 text-gray-500 max-w-xs truncate" title={m.modelDesc}>
                      {m.modelDesc || '-'}
                    </td>
                    <td className="p-3">
                      <Badge status={STATUS_BADGE[s] || 'default'}>{t(`models.status.${s || 'INIT'}`)}</Badge>
                    </td>
                    <td className="p-3 text-gray-500">{formatTime(m.gmtCreate)}</td>
                    <td className="p-3">
                      <RowActions>
                        {canWrite && CAN_PUBLISH.includes(s) && (
                          <Button size="sm" variant="ghost" onClick={() => setPublish({ modelId: m.modelId })}>
                            {t('models.publish')}
                          </Button>
                        )}
                        {canWrite && CAN_DISCARD.includes(s) && (
                          <Button size="sm" variant="ghost" onClick={() => setConfirm({ kind: 'discard', model: m })}>
                            {t('models.discard')}
                          </Button>
                        )}
                        {canWrite && CAN_OFFLINE.includes(s) && m.servingId && (
                          <Button size="sm" variant="ghost" onClick={() => setConfirm({ kind: 'offline', model: m })}>
                            {t('models.offline')}
                          </Button>
                        )}
                        {canWrite && CAN_DELETE.includes(s) && (
                          <Button size="sm" variant="ghost" className="text-rose-600" onClick={() => setConfirm({ kind: 'delete', model: m })}>
                            {t('common.delete')}
                          </Button>
                        )}
                        {CAN_VIEW_SERVING.includes(s) && (
                          <Button size="sm" variant="ghost" onClick={() => setDetail(m)}>
                            {t('models.viewServing')}
                          </Button>
                        )}
                      </RowActions>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table></TableView>
        </div>
        <div className="flex items-center justify-end gap-3 p-3">
          <Select
            className="!w-24"
            value={String(size)}
            onChange={(v) => {
              setSize(Number(v));
              setPage(1);
            }}
            options={[10, 20, 50].map((n) => ({ value: String(n), label: `${n}/page` }))}
          />
          <Pagination page={page} pageSize={size} total={total} onChange={setPage} />
        </div>
      </Card>

      <PublishModelDrawer
        open={!!publish}
        onClose={() => setPublish(null)}
        onOk={invalidate}
        projectId={projectId}
        modelId={publish?.modelId}
        models={publishable}
      />
      <PackModelModal open={packOpen} onClose={() => setPackOpen(false)} projectId={projectId} onPacked={invalidate} />
      <ServingDetailModal model={detail} projectId={projectId} onClose={() => setDetail(null)} />
      <ConfirmDialog
        isOpen={!!confirm}
        title={confirm ? t(`models.${confirm.kind}`) : ''}
        message={confirm ? t(`models.${confirm.kind}Confirm`, { name: confirm.model.modelName || '' }) : ''}
        danger={confirm?.kind !== 'offline'}
        loading={action.isPending}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        onConfirm={() => confirm && action.mutate(confirm)}
        onCancel={() => setConfirm(null)}
      />
    </div>
  );
};
