import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Navigate, useSearch } from '@tanstack/react-router';
import { Button, FormField, Input, Modal, Select, Tabs, toast } from '@secretpad/design-system';
import { addInstNodeJava, deleteInstNodeJava, getNodeJava, listMyInstNodesJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { resolveMyNodeId, usePlatform, usePlatformContext } from '@/shared/lib/platform';
import { useNodeContext } from '@/shared/lib/use-node-context';
import { CooperativeNodeList } from '@/features/cooperative-node/cooperative-node-list';
import { errorText, nodeStatusMeta } from '@/features/cooperative-node/format';
import { DeleteNodeDialog } from '../../nodes/node-detail-drawer';
import { NodeInfoCard } from './node-info-card';

const MAX_INST_NODES = 10;
const NAME_RE = /^[一-龥A-Za-z0-9-_]+$/;

/** AUTONOMY: add a compute node (inst/node/add). Legacy my-node/add-node. */
const AddComputeNodeModal: React.FC<{ open: boolean; onClose: () => void; onOk: () => void }> = ({ open, onClose, onOk }) => {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  useEffect(() => {
    if (open) setName('');
  }, [open]);
  const err = !name ? undefined : name.length > 32 ? t('nodes.nameError.length') : !NAME_RE.test(name) ? t('nodes.nameError.pattern') : undefined;
  const mutation = useMutation({
    mutationFn: () => addInstNodeJava(name),
    onSuccess: () => {
      toast.success(t('p2p.addNodeSuccess', { name }));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });
  return (
    <Modal
      isOpen={open}
      onClose={onClose}
      title={t('p2p.addComputeNode')}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" disabled={!name || !!err} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <FormField label={t('nodes.nodeNameLabel')} required error={err}>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('nodes.nodeNamePlaceholder')} />
      </FormField>
    </Modal>
  );
};

/**
 * 我的节点 / 我的机构（legacy modules/my-node）。
 * - AUTONOMY：机构下多节点切换（inst/node/list）、新增/删除计算节点、机构 token（inst/node/token|newToken）。
 * - EDGE / P2P / CENTER 上的 EDGE 账号：展示自己的节点，可修改通讯地址、设置中心平台账号密码。
 * - CENTER 管理员：只能通过 `?ownerId=` 查看内置节点（alice/bob/tee），否则回到节点管理
 *   （旧版 edge-auth；不能用平台 id kuscia-system 调 node/get）。
 * - 第二个标签页为合作节点（nodeRoute / p2p node）。
 */
export const P2pMyNodePage: React.FC = () => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const queryClient = useQueryClient();
  const ctx = useNodeContext();
  const { ownerId: requestedId } = useSearch({ strict: false }) as { ownerId?: string };
  const ownNodeId = resolveMyNodeId(usePlatformContext(), ctx.nodeId || requestedId);
  const [tab, setTab] = useState('info');
  const [currentId, setCurrentId] = useState<string>('');
  const [addOpen, setAddOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const instNodes = useQuery({ queryKey: ['my-inst-nodes'], queryFn: listMyInstNodesJava, enabled: platform.isAutonomy });
  const list = useMemo(() => instNodes.data || [], [instNodes.data]);

  // Pick the main node by default (legacy setCurrentPageMainNodeId).
  useEffect(() => {
    if (!platform.isAutonomy) return;
    if (!currentId || !list.some((n) => n.nodeId === currentId)) {
      const main = list.find((n) => n.isMainNode) || list[0];
      if (main?.nodeId) setCurrentId(main.nodeId);
    }
  }, [platform.isAutonomy, list, currentId]);

  const nodeId = (platform.isAutonomy ? currentId : ownNodeId) || '';
  const detail = useQuery({ queryKey: ['my-node', nodeId], queryFn: () => getNodeJava(nodeId), enabled: !!nodeId });
  const node = detail.data;

  const refreshAll = () => {
    void queryClient.invalidateQueries({ queryKey: ['my-inst-nodes'] });
    void queryClient.invalidateQueries({ queryKey: ['my-node'] });
  };

  const deleteReason = node && !node.allowDeletion ? (node.isMainNode ? t('p2p.mainNodeNoDelete') : t('p2p.authorizedNoDelete')) : null;

  const autonomyActions = platform.isAutonomy ? (
    <>
      <Select
        className="!w-56"
        value={currentId}
        onChange={setCurrentId}
        options={list.map((n) => ({
          value: n.nodeId || '',
          label: `${n.isMainNode ? `[${t('p2p.mainNode')}] ` : ''}${n.nodeName || n.nodeId} · ${t(nodeStatusMeta(n.nodeStatus).key)}`,
        }))}
      />
      <span title={list.length >= MAX_INST_NODES ? t('p2p.maxNodes') : undefined}>
        <Button size="sm" variant="outline" disabled={list.length >= MAX_INST_NODES} onClick={() => setAddOpen(true)}>
          ＋ {t('p2p.addComputeNode')}
        </Button>
      </span>
      <span title={deleteReason || undefined}>
        <Button size="sm" variant="ghost" disabled={!!deleteReason} onClick={() => setDeleteOpen(true)}>
          {t('common.delete')}
        </Button>
      </span>
    </>
  ) : null;

  // Route guard normally redirects first; this covers a stale persisted user.
  if (ownNodeId === null) return <Navigate to="/nodes" replace />;

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">
          {platform.isAutonomy ? t('p2p.myInst') : t('p2p.myNodeTitle')}
        </h2>
        <p className="text-xs text-gray-500">{t('p2p.myNodeSubtitle')}</p>
        <Tabs
          className="mt-3"
          activeKey={tab}
          onChange={setTab}
          items={[
            { key: 'info', label: t('p2p.nodeInfo') },
            { key: 'coop', label: t('coop.title') },
          ]}
        />
      </div>

      {tab === 'info' && (
        <>
          {(detail.isLoading || instNodes.isLoading) && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
          {detail.error && <div className="text-xs text-rose-500">{errorText(detail.error)}</div>}
          {!nodeId && !instNodes.isLoading && <div className="text-xs text-gray-400">{t('p2p.noNodes')}</div>}
          {node && <NodeInfoCard node={node} headerExtra={autonomyActions} onChanged={refreshAll} />}
        </>
      )}
      {tab === 'coop' && <CooperativeNodeList ownerNodeId={platform.isAutonomy ? undefined : nodeId || undefined} compact />}

      <AddComputeNodeModal open={addOpen} onClose={() => setAddOpen(false)} onOk={refreshAll} />
      <DeleteNodeDialog
        node={deleteOpen ? node || null : null}
        onClose={() => setDeleteOpen(false)}
        deleteFn={deleteInstNodeJava}
        onDeleted={() => {
          setCurrentId('');
          refreshAll();
        }}
      />
    </div>
  );
};
