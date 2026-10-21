import React, { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Drawer, Input, Modal, Tabs, toast } from '@secretpad/design-system';
import {
  apiClient,
  deleteNodeJava,
  getNodeJava,
  refreshNodeJava,
  updateNodeJava,
  type NodeDetailJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { errorText, formatTime } from '@/features/cooperative-node/format';
import { InfoRow, NodeStatusBadge, SecretText } from '@/features/cooperative-node/ui-common';
import { EdgeAccountPasswordDialog } from '@/features/edge-account/edge-account-dialog';
import { TokenPanel } from './token-panel';
import { NodeInstancesSection } from './node-instances';
import { nodeDeleteBlockReason } from './node-helpers';

/** Delete confirmation that requires typing the node name (legacy confirmDeleteInput). */
export const DeleteNodeDialog: React.FC<{
  node: NodeDetailJava | null;
  onClose: () => void;
  onDeleted: () => void;
  deleteFn?: (nodeId: string) => Promise<unknown>;
}> = ({ node, onClose, onDeleted, deleteFn = deleteNodeJava }) => {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  useEffect(() => setName(''), [node]);
  const mutation = useMutation({
    mutationFn: () => deleteFn(node!.nodeId!),
    onSuccess: () => {
      toast.success(t('nodes.deleteSuccess', { name: node?.nodeName || '' }));
      onDeleted();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });
  return (
    <Modal
      isOpen={!!node}
      onClose={onClose}
      title={t('nodes.delete')}
      width="max-w-md"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" disabled={name !== (node?.nodeName || '')} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            {t('common.delete')}
          </Button>
        </>
      }
    >
      <div className="space-y-2 text-sm">
        <p className="text-gray-600 dark:text-gray-300">{t('nodes.deleteConfirm')}</p>
        <p className="text-xs text-gray-500">{t('nodes.deleteTypeName', { name: node?.nodeName || '' })}</p>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={node?.nodeName} />
      </div>
    </Modal>
  );
};

/** Legacy managed-node-list/node-info drawer, extended with keys, instances and results. */
export const NodeDetailDrawer: React.FC<{ nodeId: string | null; onClose: () => void; onChanged: () => void }> = ({
  nodeId,
  onClose,
  onChanged,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const platform = usePlatform();
  const [tab, setTab] = useState('basic');
  const [editAddr, setEditAddr] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<NodeDetailJava | null>(null);
  const [pwdOpen, setPwdOpen] = useState(false);

  useEffect(() => {
    setTab('basic');
    setEditAddr(null);
  }, [nodeId]);

  const detail = useQuery({ queryKey: ['node-detail', nodeId], queryFn: () => getNodeJava(nodeId!), enabled: !!nodeId });
  const node = detail.data;
  const canWrite = platform.canWriteNode(nodeId || undefined, { instId: node?.instId });

  const refresh = useMutation({
    mutationFn: () => refreshNodeJava(nodeId!),
    onSuccess: () => {
      toast.success(t('nodes.refreshSuccess', { name: node?.nodeName || '' }));
      void queryClient.invalidateQueries({ queryKey: ['node-detail', nodeId] });
      onChanged();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const saveAddr = useMutation({
    mutationFn: (netAddress: string) => updateNodeJava({ nodeId: nodeId!, netAddress }),
    onSuccess: () => {
      setEditAddr(null);
      toast.success(t('nodes.addressUpdated'));
      void queryClient.invalidateQueries({ queryKey: ['node-detail', nodeId] });
      onChanged();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const results = useQuery({
    queryKey: ['node-results', nodeId],
    queryFn: () => apiClient.listNodeResults({ ownerId: nodeId!, pageSize: 50, pageNumber: 1 }),
    enabled: !!nodeId && tab === 'results',
  });

  const blockReason = nodeDeleteBlockReason(node, t);

  return (
    <Drawer
      isOpen={!!nodeId}
      onClose={onClose}
      width="max-w-2xl"
      title={
        <span className="inline-flex items-center gap-2">
          {t('nodes.detailTitle', { name: node?.nodeName || nodeId || '' })}
          {node && <NodeStatusBadge status={node.nodeStatus} />}
        </span>
      }
      footer={
        node ? (
          <>
            {platform.canEnterNode(node.nodeId) && (
              <Button variant="primary" onClick={() => navigate({ to: '/node/$nodeId', params: { nodeId: node.nodeId! } })}>
                {t('nodes.enterNode')}
              </Button>
            )}
            {platform.isCenterAdmin && node.type !== 'embedded' && (
              <Button variant="outline" onClick={() => setPwdOpen(true)}>
                {t('edgeAccount.title')}
              </Button>
            )}
            {canWrite && node.type !== 'embedded' && (
              <span title={blockReason || undefined}>
                <Button variant="danger" disabled={!!blockReason} onClick={() => setDeleting(node)}>
                  {t('common.delete')}
                </Button>
              </span>
            )}
          </>
        ) : undefined
      }
    >
      {detail.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
      {detail.error && <div className="text-xs text-rose-500">{errorText(detail.error)}</div>}
      {node && (
        <div className="space-y-4">
          <Tabs
            activeKey={tab}
            onChange={setTab}
            items={[
              { key: 'basic', label: t('nodes.basicInfo') },
              { key: 'token', label: t('nodes.deployToken') },
              { key: 'instances', label: t('nodes.instances') },
              { key: 'results', label: t('nodes.nodeResults') },
            ]}
          />
          {tab === 'basic' && (
            <div className="px-1">
              <InfoRow label={t('nodes.name')}>{node.nodeName}</InfoRow>
              <InfoRow label={t('nodes.id')}>
                <span className="font-mono">{node.nodeId}</span>
              </InfoRow>
              <InfoRow label={t('nodes.type')}>{node.type || '-'}</InfoRow>
              <InfoRow label={t('nodes.status')}>
                <span className="inline-flex items-center gap-2">
                  <NodeStatusBadge status={node.nodeStatus} />
                  <Button size="sm" variant="link" loading={refresh.isPending} onClick={() => refresh.mutate()}>
                    {t('common.refresh')}
                  </Button>
                </span>
              </InfoRow>
              <InfoRow label={t('nodes.netAddress')}>
                {editAddr !== null ? (
                  <span className="flex gap-2">
                    <Input value={editAddr} onChange={(e) => setEditAddr(e.target.value)} placeholder="http://host:port" />
                    <Button size="sm" variant="primary" loading={saveAddr.isPending} onClick={() => saveAddr.mutate(editAddr)}>
                      {t('common.save')}
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setEditAddr(null)}>
                      {t('common.cancel')}
                    </Button>
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-2">
                    <span className="font-mono">{node.netAddress || '-'}</span>
                    {canWrite && node.type !== 'embedded' && (
                      <button type="button" className="text-blue-600 hover:underline" onClick={() => setEditAddr(node.netAddress || '')}>
                        {t('common.edit')}
                      </button>
                    )}
                  </span>
                )}
              </InfoRow>
              {node.protocol && <InfoRow label={t('nodes.protocol')}>{node.protocol}</InfoRow>}
              <InfoRow label={t('nodes.registerTime')}>{formatTime(node.gmtCreate)}</InfoRow>
              <InfoRow label={t('nodes.cert')}>
                {node.cert === 'configured' ? t('nodes.certConfigured') : <span className="text-gray-400">{t('nodes.certPending')}</span>}
              </InfoRow>
              <InfoRow label={t('nodes.publicKey')}>
                <SecretText text={node.certText} emptyHint={t('nodes.afterDeploy')} />
              </InfoRow>
              <InfoRow label={t('nodes.authCode')}>
                <SecretText text={node.nodeAuthenticationCode} emptyHint={t('nodes.afterDeploy')} />
              </InfoRow>
              {node.description && <InfoRow label={t('nodes.description')}>{node.description}</InfoRow>}
              {blockReason && node.type !== 'embedded' && (
                <div className="mt-3 text-[11px] text-amber-600">{blockReason}</div>
              )}
            </div>
          )}
          {tab === 'token' && <TokenPanel kind="node" nodeId={node.nodeId!} canRefresh={canWrite} />}
          {tab === 'instances' && <NodeInstancesSection instances={node.nodeInstances} resources={node.resources} />}
          {tab === 'results' && (
            <div className="space-y-2 text-xs">
              {results.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
              {!results.isLoading && (results.data?.nodeAllResultsVOList ?? []).length === 0 && (
                <div className="text-gray-400 text-center py-6">{t('nodes.noResults')}</div>
              )}
              {(results.data?.nodeAllResultsVOList ?? []).map((item, idx) => (
                <div key={idx} className="p-2.5 rounded-lg border border-gray-200 dark:border-gray-800">
                  <div className="font-semibold">{item.nodeResultsVO?.productName || item.nodeResultsVO?.domainDataId}</div>
                  <div className="text-gray-400 font-mono">
                    {item.nodeResultsVO?.datatableType} · {item.nodeResultsVO?.sourceProjectName || '-'}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
      <DeleteNodeDialog
        node={deleting}
        onClose={() => setDeleting(null)}
        onDeleted={() => {
          onChanged();
          onClose();
        }}
      />
      {node?.nodeId && (
        <EdgeAccountPasswordDialog open={pwdOpen} onClose={() => setPwdOpen(false)} nodeId={node.nodeId} />
      )}
    </Drawer>
  );
};
