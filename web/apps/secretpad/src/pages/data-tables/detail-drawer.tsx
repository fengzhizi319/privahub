/**
 * 数据表详情抽屉（旧前端 `DataTableInfoDrawer` + `DataTableAuth`）。
 *
 * - 基本信息：数据源、类型、所属节点、数据地址、空缺值、描述，支持“刷新状态”（datatable/get）；
 * - 数据表结构；
 * - 授权项目：添加授权 / 配置授权（字段级关联键、分组列、标签列、保护开关）/ 取消授权；
 * - 授权血缘：数据源 → 数据表 → 已授权项目。
 */
import React, { useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Badge, Button, ConfirmDialog, Drawer, Tabs, toast } from '@secretpad/design-system';
import { cancelDatatableAuthJava, flattenDatatableNode, getDatatableJava } from '@secretpad/api-client';
import type { DatatableAuthProjectJava, DatatableRowJava } from '@secretpad/api-client';
import { formatNullStrs } from '@secretpad/utils';
import { useTranslation } from '@/shared/lib/i18n';
import { LineageTree } from '@/features/lineage/lineage-tree';
import type { LineageNode } from '@/features/lineage/lineage-tree';
import { AuthConfigModal } from './auth-config-modal';

export type DetailTab = 'schema' | 'auth' | 'lineage';

export interface DatatableDetailDrawerProps {
  row: DatatableRowJava | null;
  nodeId: string;
  initialTab?: DetailTab;
  canWrite: boolean;
  showNode: boolean;
  p2p: boolean;
  onClose: () => void;
  onChanged: () => void;
}

export const DatatableDetailDrawer: React.FC<DatatableDetailDrawerProps> = ({
  row,
  nodeId,
  initialTab = 'schema',
  canWrite,
  showNode,
  p2p,
  onClose,
  onChanged,
}) => {
  const { t } = useTranslation();
  const [tab, setTab] = useState<DetailTab>(initialTab);
  const [authModal, setAuthModal] = useState<{ mode: 'ADD' | 'EDIT'; project?: DatatableAuthProjectJava } | null>(null);
  const [cancelTarget, setCancelTarget] = useState<DatatableAuthProjectJava | null>(null);

  React.useEffect(() => setTab(initialTab), [initialTab, row?.datatableId]);

  const detailQuery = useQuery({
    queryKey: ['datatable-detail-java', nodeId, row?.datatableId],
    queryFn: () =>
      getDatatableJava({ nodeId, datatableId: row?.datatableId, type: row?.type, datasourceType: row?.datasourceType }),
    enabled: !!row && !!nodeId,
  });
  const info: DatatableRowJava = useMemo(
    () => ({ ...(row || {}), ...(detailQuery.data ? flattenDatatableNode(detailQuery.data) : {}) }),
    [row, detailQuery.data],
  );
  const authProjects = useMemo(() => [...(info.authProjects || [])].reverse(), [info.authProjects]);

  const cancelMutation = useMutation({
    mutationFn: (p: DatatableAuthProjectJava) =>
      cancelDatatableAuthJava({ projectId: p.projectId, nodeId, datatableId: info.datatableId, type: info.type }),
    onSuccess: () => {
      toast.success(t('dataTables.authCancel'));
      setCancelTarget(null);
      detailQuery.refetch();
      onChanged();
    },
    onError: (e) => {
      setCancelTarget(null);
      toast.error(e instanceof Error ? e.message : String(e));
    },
  });

  const lineageRoot = useMemo<LineageNode | null>(() => {
    if (!row) return null;
    return {
      id: 'datasource',
      icon: '🗄️',
      title: info.datasourceName || t('dataTables.lineageDatasource'),
      subtitle: info.datasourceId,
      badge: info.datasourceType,
      badgeTone: 'purple',
      children: [
        {
          id: 'table',
          icon: '📋',
          title: info.datatableName || '',
          subtitle: info.datatableId,
          badge: `${info.schema?.length ?? 0} ${t('dataTables.lineageColumns')}`,
          badgeTone: 'blue',
          details: [
            ...(info.relativeUri ? [{ label: 'URI', value: info.relativeUri }] : []),
            ...(info.status ? [{ label: t('dataTables.lineageStatus'), value: info.status }] : []),
          ],
          children: authProjects.map((auth, idx) => ({
            id: `project-${auth.projectId}-${idx}`,
            icon: '📁',
            title: auth.name || auth.projectId || '-',
            subtitle: auth.projectId,
            badge: auth.computeMode,
            badgeTone: 'green' as const,
            details: [
              ...(auth.associateKeys?.length ? [{ label: t('dataTables.lineageAssocKeys'), value: auth.associateKeys.join(', ') }] : []),
              ...(auth.labelKeys?.length ? [{ label: t('dataTables.lineageLabelKeys'), value: auth.labelKeys.join(', ') }] : []),
              ...(auth.gmtCreate ? [{ label: t('dataTables.lineageAuthTime'), value: auth.gmtCreate }] : []),
            ],
          })),
        },
      ],
    };
  }, [row, info, authProjects, t]);

  if (!row) return null;

  const kv = (label: string, value: React.ReactNode) => (
    <div className="flex gap-2">
      <span className="shrink-0 text-gray-500">{label}：</span>
      <span className="break-all">{value}</span>
    </div>
  );

  return (
    <>
      <Drawer
        isOpen={!!row}
        onClose={onClose}
        width="max-w-3xl"
        title={
          <div className="flex items-center gap-3">
            <span>{t('dtDetail.title', { name: info.datatableName || '' })}</span>
            <Badge status={info.status === 'Available' ? 'success' : 'error'}>
              {info.status === 'Available' ? t('dataTables.statusAvailable') : t('dataTables.statusUnavailable')}
            </Badge>
            <Button
              size="sm"
              variant="link"
              loading={detailQuery.isFetching}
              onClick={() =>
                detailQuery.refetch().then((r) => {
                  if (r.isSuccess) toast.success(t('dtDetail.refreshSuccess'));
                  else toast.error(t('dtDetail.refreshFailed'));
                })
              }
            >
              {t('dataTables.refreshStatus')}
            </Button>
          </div>
        }
      >
        <div className="space-y-4 text-xs">
          <div className="grid grid-cols-2 gap-2">
            {kv(t('dtDetail.datasource'), info.datasourceName || info.datasourceId || '-')}
            {kv(t('dtDetail.datasourceType'), info.datasourceType || '-')}
            {showNode && kv(t('dataTables.nodeBelongs'), info.nodeName || info.nodeId || '-')}
          </div>
          {kv(t('dtDetail.address'), <span className="font-mono">{info.relativeUri || '-'}</span>)}
          {kv(t('schemaEditor.nullStrs'), <span className="font-mono">{formatNullStrs(info.nullStrs) || '-'}</span>)}
          {kv(t('dataCommon.description'), info.description || '-')}
          {info.partition?.fields?.length ? kv(t('dtRegister.partition'), info.partition.fields.map((f) => f.name).join(', ')) : null}

          <Tabs
            items={[
              { key: 'schema', label: t('dtDetail.schema') },
              { key: 'auth', label: t('dataTables.auth'), badge: authProjects.length },
              { key: 'lineage', label: t('dataTables.lineage') },
            ]}
            activeKey={tab}
            onChange={(k) => setTab(k as DetailTab)}
          />

          {tab === 'schema' && (
            <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold">
                <tr>
                  <th className="p-2">{t('dataTables.columnName')}</th>
                  <th className="p-2">{t('dataTables.dataType')}</th>
                  <th className="p-2">{t('dataTables.columnDescription')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {(info.schema || []).map((c, i) => (
                  <tr key={`${c.colName}-${i}`}>
                    <td className="p-2 font-mono">{c.colName}</td>
                    <td className="p-2 font-mono text-gray-500">{c.colType}</td>
                    <td className="p-2 text-gray-500">{c.colComment || '-'}</td>
                  </tr>
                ))}
                {(info.schema || []).length === 0 && (
                  <tr>
                    <td colSpan={3} className="p-3 text-center text-gray-400">
                      {detailQuery.isLoading ? t('common.loading') : t('common.empty')}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            </div>
          )}

          {tab === 'auth' && (
            <div className="space-y-2">
              {canWrite && (
                <Button size="sm" variant="primary" onClick={() => setAuthModal({ mode: 'ADD' })}>
                  ＋ {t('dtAuth.addProject')}
                </Button>
              )}
              <div className="overflow-x-auto">
              <table className="w-full text-left">
                <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold">
                  <tr>
                    <th className="p-2">{t('dataTables.authProjects')}</th>
                    <th className="p-2">{t('dataTables.lineageAuthTime')}</th>
                    <th className="p-2">{t('common.action')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                  {authProjects.length === 0 && (
                    <tr>
                      <td colSpan={3} className="p-3 text-center text-gray-400">
                        {t('dataTables.noAuth')}
                      </td>
                    </tr>
                  )}
                  {authProjects.map((p) => (
                    <tr key={p.projectId}>
                      <td className="p-2">
                        <span className="mr-2 px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-950 text-blue-600 text-[10px] font-semibold">
                          {p.computeMode || 'MPC'}
                        </span>
                        {p.name || p.projectId}
                      </td>
                      <td className="p-2 text-gray-500">{p.gmtCreate || '-'}</td>
                      <td className="p-2">
                        {canWrite ? (
                          <div className="flex gap-2">
                            <Button size="sm" variant="link" className="!text-red-600" onClick={() => setCancelTarget(p)}>
                              {t('dataTables.removeAuth')}
                            </Button>
                            <Button size="sm" variant="link" onClick={() => setAuthModal({ mode: 'EDIT', project: p })}>
                              {t('dtAuth.configure')}
                            </Button>
                          </div>
                        ) : (
                          '-'
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            </div>
          )}

          {tab === 'lineage' &&
            (lineageRoot ? <LineageTree root={lineageRoot} /> : <div className="text-center text-gray-400 py-6">{t('dataTables.noLineage')}</div>)}
        </div>
      </Drawer>

      {authModal && (
        <AuthConfigModal
          isOpen
          mode={authModal.mode}
          project={authModal.project}
          table={info}
          nodeId={nodeId}
          p2p={p2p}
          onClose={() => setAuthModal(null)}
          onDone={() => {
            detailQuery.refetch();
            onChanged();
          }}
        />
      )}

      <ConfirmDialog
        isOpen={!!cancelTarget}
        title={t('dataTables.removeAuth')}
        message={t('dtAuth.cancelConfirm', { name: cancelTarget?.name || cancelTarget?.projectId || '' })}
        danger
        loading={cancelMutation.isPending}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        onConfirm={() => cancelTarget && cancelMutation.mutate(cancelTarget)}
        onCancel={() => setCancelTarget(null)}
      />
    </>
  );
};
