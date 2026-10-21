import React, { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Button, Card, Input, toast } from '@secretpad/design-system';
import { updateNodeJava, type NodeDetailJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { errorText } from '@/features/cooperative-node/format';
import { InfoRow, NodeStatusBadge, SecretText } from '@/features/cooperative-node/ui-common';
import { EdgeAccountPasswordDialog } from '@/features/edge-account/edge-account-dialog';
import { TokenPanel } from '../../nodes/token-panel';
import { NodeInstancesSection } from '../../nodes/node-instances';

/** Legacy my-node "节点能力" (AUTONOMY). */
const NODE_FUNCS = [{ nameKey: 'p2p.funcControlCompute', descKey: 'p2p.funcControlComputeDesc' }];

/** Legacy `modules/my-node/index.tsx` card: node info, keys, token, edge account, instances. */
export const NodeInfoCard: React.FC<{ node: NodeDetailJava; headerExtra?: React.ReactNode; onChanged: () => void }> = ({
  node,
  headerExtra,
  onChanged,
}) => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const queryClient = useQueryClient();
  const [addr, setAddr] = useState<string | null>(null);
  const [pwdOpen, setPwdOpen] = useState(false);
  const nodeId = node.nodeId || '';
  const canWrite = platform.canWriteNode(platform.isAutonomy ? undefined : nodeId);
  const ready = node.nodeStatus === 'Ready' || node.nodeStatus === 'Succeeded';

  const save = useMutation({
    mutationFn: (netAddress: string) => updateNodeJava({ nodeId, netAddress }),
    onSuccess: () => {
      setAddr(null);
      toast.success(t('nodes.addressUpdated'));
      void queryClient.invalidateQueries({ queryKey: ['my-node', nodeId] });
      onChanged();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const showEdgeAccount = (platform.isCenter || platform.isEdge) && node.type !== 'embedded';

  return (
    <Card>
      <div className="flex flex-wrap items-center gap-3 mb-4">
        <h3 className="text-lg font-bold text-gray-900 dark:text-gray-100">
          {node.nodeName}
          {platform.isAutonomy && <span className="ml-1 text-sm font-normal text-gray-500">{t('p2p.nodeCenter')}</span>}
        </h3>
        <span title={!ready && node.nodeStatus ? t('p2p.nodeUnavailableTip') : undefined}>
          <NodeStatusBadge status={node.nodeStatus} />
        </span>
        <div className="ml-auto flex items-center gap-2">{headerExtra}</div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-x-8">
        <InfoRow label={t('nodes.id')}>
          <span className="font-mono">{node.nodeId}</span>
        </InfoRow>
        <InfoRow label={<span title={platform.isAutonomy ? t('p2p.addressChangeTip') : undefined}>{t('nodes.netAddress')}</span>}>
          {addr !== null ? (
            <span className="flex gap-2">
              <Input
                autoFocus
                value={addr}
                onChange={(e) => setAddr(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && save.mutate(addr)}
              />
              <Button size="sm" variant="primary" loading={save.isPending} onClick={() => save.mutate(addr)}>
                {t('common.save')}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setAddr(null)}>
                {t('common.cancel')}
              </Button>
            </span>
          ) : (
            <span className="inline-flex items-center gap-2">
              <span className="font-mono">{node.netAddress || '-'}</span>
              {canWrite && (
                <button type="button" className="text-blue-600 hover:underline" onClick={() => setAddr(node.netAddress || '')}>
                  ✎ {t('common.edit')}
                </button>
              )}
            </span>
          )}
        </InfoRow>
        <InfoRow label={t('nodes.protocol')}>{node.protocol || '-'}</InfoRow>
        {platform.isCenter && (
          <InfoRow label={t('nodes.cert')}>{node.cert === 'configured' ? t('nodes.certConfigured') : t('nodes.certPending')}</InfoRow>
        )}
        {(platform.isP2p || node.certText) && (
          <InfoRow label={t('nodes.publicKey')}>
            <SecretText text={node.certText} />
          </InfoRow>
        )}
        {(platform.isP2p || node.nodeAuthenticationCode) && (
          <InfoRow label={<span title={t('p2p.authCodeTip')}>{t('nodes.authCode')}</span>}>
            <SecretText text={node.nodeAuthenticationCode} />
          </InfoRow>
        )}
        {showEdgeAccount && (
          <InfoRow label={<span title={t('p2p.initialPasswordTip', { nodeId })}>{t('p2p.centerAccount')}</span>}>
            <span className="inline-flex items-center gap-3">
              <span className="font-mono">{nodeId}</span>
              <button type="button" className="text-blue-600 hover:underline" onClick={() => setPwdOpen(true)}>
                {t('edgeAccount.title')}
              </button>
            </span>
          </InfoRow>
        )}
      </div>

      {platform.isAutonomy && (
        <div className="mt-4">
          <div className="text-sm font-semibold mb-1.5">{t('p2p.token')}</div>
          <TokenPanel kind="inst" nodeId={nodeId} canRefresh />
        </div>
      )}
      {!platform.isAutonomy && canWrite && node.type !== 'embedded' && (
        <div className="mt-4">
          <div className="text-sm font-semibold mb-1.5">{t('nodes.deployToken')}</div>
          <TokenPanel kind="node" nodeId={nodeId} canRefresh={platform.isCenterAdmin} />
        </div>
      )}

      {platform.isAutonomy && (
        <div className="mt-5">
          <div className="text-sm font-semibold mb-2">{t('p2p.nodeFunc')}</div>
          {NODE_FUNCS.map((f) => (
            <div key={f.nameKey} className="flex items-center justify-between p-3 rounded-lg bg-gray-50 dark:bg-gray-850 text-xs">
              <div>
                <div className="font-semibold">{t(f.nameKey)}</div>
                <div className="text-gray-500">{t(f.descKey)}</div>
              </div>
              <span className="text-emerald-600">{t('p2p.supported')}</span>
            </div>
          ))}
        </div>
      )}

      <div className="mt-5">
        <NodeInstancesSection instances={node.nodeInstances} resources={node.resources} />
      </div>

      {showEdgeAccount && (
        <EdgeAccountPasswordDialog
          open={pwdOpen}
          onClose={() => setPwdOpen(false)}
          nodeId={nodeId}
          // CENTER manages node accounts locally; EDGE changes its account on the remote CENTER.
        />
      )}
    </Card>
  );
};
