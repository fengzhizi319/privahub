import React, { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, FormField, Input, Modal, Pagination, RadioGroup, Select, toast } from '@secretpad/design-system';
import { createNodeJava, pageNodesJava, refreshNodeJava, type NodeDetailJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { errorText, formatTime } from '@/features/cooperative-node/format';
import { NodeStatusBadge } from '@/features/cooperative-node/ui-common';
import { useDebounced } from '@/shared/lib/use-debounced';
import { DeleteNodeDialog, NodeDetailDrawer } from './node-detail-drawer';
import { nodeDeleteBlockReason } from './node-helpers';

/** Java `CreateNodeRequest.mode`: 0 tee | 1 mpc | 2 tee&mpc (legacy register sent 1). */
const NODE_MODE = { TEE: 0, MPC: 1, BOTH: 2 } as const;

const NODE_NAME_RE = /^[一-龥A-Za-z0-9-_]+$/;

function validateNodeName(name: string): 'required' | 'length' | 'pattern' | null {
  if (!name.trim()) return 'required';
  if (name.length > 32) return 'length';
  if (!NODE_NAME_RE.test(name)) return 'pattern';
  return null;
}

/** Register node modal (legacy create-node.view). */
const RegisterNodeModal: React.FC<{ open: boolean; onClose: () => void; onOk: () => void }> = ({ open, onClose, onOk }) => {
  const { t } = useTranslation();
  const { supportsTee, supportsMpc } = usePlatform();
  const [name, setName] = useState('');
  const [mode, setMode] = useState<number>(NODE_MODE.MPC);
  const [touched, setTouched] = useState(false);
  useEffect(() => {
    if (open) {
      setName('');
      setMode(supportsMpc ? NODE_MODE.MPC : NODE_MODE.TEE);
      setTouched(false);
    }
  }, [open, supportsMpc]);
  const nameErr = validateNodeName(name);
  const mutation = useMutation({
    mutationFn: () => createNodeJava({ name, mode }),
    onSuccess: () => {
      toast.success(t('nodes.registerSuccess', { name }));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });
  const modeOptions = [
    ...(supportsMpc ? [{ value: String(NODE_MODE.MPC), label: t('nodes.modeMPC') }] : []),
    ...(supportsTee ? [{ value: String(NODE_MODE.TEE), label: t('nodes.modeTEE') }] : []),
    ...(supportsTee && supportsMpc ? [{ value: String(NODE_MODE.BOTH), label: t('nodes.modeBoth') }] : []),
  ];
  return (
    <Modal
      isOpen={open}
      onClose={onClose}
      title={t('nodes.modalRegisterTitle')}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={mutation.isPending}
            onClick={() => {
              setTouched(true);
              if (!nameErr) mutation.mutate();
            }}
          >
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <FormField label={t('nodes.nodeNameLabel')} required error={touched && nameErr ? t(`nodes.nameError.${nameErr}`) : undefined}>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('nodes.nodeNamePlaceholder')} />
        </FormField>
        {modeOptions.length > 1 && (
          <FormField label={t('nodes.modeLabel')}>
            <RadioGroup value={String(mode)} onChange={(v) => setMode(Number(v))} options={modeOptions} />
          </FormField>
        )}
      </div>
    </Modal>
  );
};

/**
 * 节点管理（legacy managed-node-list）：node/page 服务端分页、搜索、注册时间排序，
 * 详情抽屉（证书/公钥/认证码/部署令牌/实例/产物）、刷新状态、删除、进入节点。
 */
export const NodesPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const platform = usePlatform();

  const [search, setSearch] = useState('');
  const debounced = useDebounced(search);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  const [sortOrder, setSortOrder] = useState<'ASC' | 'DESC' | null>(null);
  const [registerOpen, setRegisterOpen] = useState(false);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<NodeDetailJava | null>(null);

  useEffect(() => setPage(1), [debounced, sortOrder]);

  // EDGE accounts on CENTER have no node management (route guard blocks it too).
  const hidden = platform.isEdgeAccountOnCenter;

  const listQuery = useQuery({
    queryKey: ['nodes-page', page, size, debounced, sortOrder],
    queryFn: () =>
      pageNodesJava({ page, size, search: debounced, sort: sortOrder ? { gmtCreate: sortOrder } : {} }),
    enabled: !hidden,
    placeholderData: keepPreviousData,
  });
  const nodes = listQuery.data?.list ?? [];
  const total = listQuery.data?.total ?? 0;
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['nodes-page'] });
    void queryClient.invalidateQueries({ queryKey: ['nodes'] });
  };

  const refresh = useMutation({
    mutationFn: (n: NodeDetailJava) => refreshNodeJava(n.nodeId!),
    onSuccess: (_, n) => {
      toast.success(t('nodes.refreshSuccess', { name: n.nodeName || '' }));
      invalidate();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  if (hidden) {
    return <div className="text-sm text-gray-400 p-8 text-center">{t('nodes.noPermission')}</div>;
  }

  const canManage = platform.isCenterAdmin || platform.isTest;

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('nodes.title')}</h2>
          <p className="text-xs text-gray-500">{t('nodes.subtitle')}</p>
        </div>
        <div className="flex flex-col sm:flex-row gap-2 sm:gap-3 w-full sm:w-auto">
          <Input className="!w-full sm:!w-60" value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('nodes.searchPlaceholder')} />
          {canManage && (
            <Button variant="primary" className="w-full sm:w-auto" icon={<span>＋</span>} onClick={() => setRegisterOpen(true)}>
              {t('nodes.register')}
            </Button>
          )}
        </div>
      </div>

      {listQuery.error && <div className="text-xs text-rose-500">{errorText(listQuery.error)}</div>}

      <Card bodyClassName="p-0">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-800 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('nodes.name')}</th>
                <th className="p-4">{t('nodes.status')}</th>
                <th className="p-4">{t('nodes.netAddress')}</th>
                <th
                  className="p-4 cursor-pointer select-none"
                  onClick={() => setSortOrder((s) => (s === null ? 'DESC' : s === 'DESC' ? 'ASC' : null))}
                >
                  {t('nodes.registerTime')} {sortOrder === 'ASC' ? '↑' : sortOrder === 'DESC' ? '↓' : '↕'}
                </th>
                <th className="p-4">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {listQuery.isLoading && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">
                    {t('common.loading')}
                  </td>
                </tr>
              )}
              {!listQuery.isLoading && nodes.length === 0 && (
                <tr>
                  <td colSpan={5} className="p-4 text-center text-gray-400">
                    {t('nodes.noNodes')}
                  </td>
                </tr>
              )}
              {nodes.map((node) => {
                const blockReason = nodeDeleteBlockReason(node, t);
                return (
                  <tr key={node.nodeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-800/50">
                    <td className="p-4">
                      <button
                        type="button"
                        onClick={() => setDetailId(node.nodeId || null)}
                        className="font-semibold text-blue-600 dark:text-blue-400 hover:underline text-left"
                      >
                        {node.type === 'embedded' && (
                          <span className="mr-1.5 px-1.5 py-0.5 rounded text-[10px] font-semibold bg-green-100 dark:bg-green-950 text-green-600 dark:text-green-400">
                            {t('nodes.embedded')}
                          </span>
                        )}
                        {node.nodeName}
                      </button>
                      <div className="font-mono text-[10px] text-gray-400">{node.nodeId}</div>
                    </td>
                    <td className="p-4">
                      <span className="inline-flex items-center gap-1">
                        <NodeStatusBadge status={node.nodeStatus} />
                        <Button
                          size="sm"
                          variant="link"
                          loading={refresh.isPending && refresh.variables?.nodeId === node.nodeId}
                          onClick={() => refresh.mutate(node)}
                        >
                          {t('common.refresh')}
                        </Button>
                      </span>
                    </td>
                    <td className="p-4 font-mono text-gray-500">{node.netAddress || '-'}</td>
                    <td className="p-4 text-gray-500">{formatTime(node.gmtCreate)}</td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        {platform.canEnterNode(node.nodeId) && (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => navigate({ to: '/node/$nodeId', params: { nodeId: node.nodeId! } })}
                          >
                            {t('nodes.enterNode')}
                          </Button>
                        )}
                        {canManage && node.type !== 'embedded' && (
                          <span title={blockReason || undefined}>
                            <Button size="sm" variant="ghost" disabled={!!blockReason} onClick={() => setDeleting(node)}>
                              {t('common.delete')}
                            </Button>
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
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

      <RegisterNodeModal open={registerOpen} onClose={() => setRegisterOpen(false)} onOk={invalidate} />
      <NodeDetailDrawer nodeId={detailId} onClose={() => setDetailId(null)} onChanged={invalidate} />
      <DeleteNodeDialog node={deleting} onClose={() => setDeleting(null)} onDeleted={invalidate} />
    </div>
  );
};
