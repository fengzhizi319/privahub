import React, { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, ConfirmDialog, Drawer, FormField, Modal, toast } from '@secretpad/design-system';
import {
  deleteNodeRouteJava,
  deleteP2pNodeJava,
  getNodeRouteJava,
  refreshNodeRouteJava,
  updateNodeRouteJava,
  type NodeRouterJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { getProtocol, isValidNetAddress, stripProtocol, type NetProtocol } from './auth-code';
import { buildRouteUpdate, isEmbeddedRoute } from './route-helpers';
import { errorText, formatTime } from './format';
import { AddressInput, InfoRow, NodeStatusBadge, SecretText } from './ui-common';

/** Legacy `edit-modal.tsx`. */
export const EditCooperativeNodeModal: React.FC<{
  route: NodeRouterJava | null;
  p2p: boolean;
  onClose: () => void;
  onOk: () => void;
}> = ({ route, p2p, onClose, onOk }) => {
  const { t } = useTranslation();
  const [protocol, setProtocol] = useState<NetProtocol>('http://');
  const [address, setAddress] = useState('');
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (!route) return;
    const src = p2p ? route.srcNetAddress : route.dstNetAddress || route.dstNode?.netAddress;
    setAddress(stripProtocol(src));
    setProtocol(getProtocol(src, p2p ? route.srcNode?.protocol : route.dstNode?.protocol));
    setTouched(false);
  }, [route, p2p]);

  const mutation = useMutation({
    mutationFn: () => updateNodeRouteJava(buildRouteUpdate(route!, p2p, protocol, address)),
    onSuccess: () => {
      toast.success(t('coop.editSuccess'));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });
  const invalid = !address ? t('coop.required') : !isValidNetAddress(address) ? t('coop.invalidAddress') : undefined;

  return (
    <Modal
      isOpen={!!route}
      onClose={onClose}
      title={t('coop.editTitle')}
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
              if (!invalid) mutation.mutate();
            }}
          >
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <FormField label={p2p ? t('coop.coopAddress') : t('coop.selfAddress')} required error={touched ? invalid : undefined}>
        <AddressInput protocol={protocol} onProtocolChange={setProtocol} value={address} onChange={setAddress} />
      </FormField>
    </Modal>
  );
};

/** Delete confirm (legacy delete-modal): P2P → p2p/node/delete, else nodeRoute/delete. */
export const DeleteCooperativeNodeDialog: React.FC<{
  route: NodeRouterJava | null;
  p2p: boolean;
  onClose: () => void;
  onOk: () => void;
}> = ({ route, p2p, onClose, onOk }) => {
  const { t } = useTranslation();
  const mutation = useMutation({
    mutationFn: () => (p2p ? deleteP2pNodeJava(route!.routeId!) : deleteNodeRouteJava(route!.routeId!)),
    onSuccess: () => {
      toast.success(t('coop.deleteSuccess'));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });
  return (
    <ConfirmDialog
      isOpen={!!route}
      title={t('coop.deleteTitle')}
      message={t('coop.deleteConfirm')}
      danger
      loading={mutation.isPending}
      confirmText={t('common.delete')}
      cancelText={t('common.cancel')}
      onConfirm={() => mutation.mutate()}
      onCancel={onClose}
    />
  );
};

/** Legacy `cooperative-node-detail-modal.tsx` (nodeRoute/get). */
export const CooperativeNodeDetailDrawer: React.FC<{
  routeId: string | null;
  p2p: boolean;
  canWrite: boolean;
  onClose: () => void;
  onChanged: () => void;
}> = ({ routeId, p2p, canWrite, onClose, onChanged }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<NodeRouterJava | null>(null);
  const [deleting, setDeleting] = useState<NodeRouterJava | null>(null);

  const detailQuery = useQuery({
    queryKey: ['coop-route-detail', routeId],
    queryFn: () => getNodeRouteJava(routeId!),
    enabled: !!routeId,
  });
  const d = detailQuery.data;
  const refresh = useMutation({
    mutationFn: () => refreshNodeRouteJava(routeId!),
    onSuccess: () => {
      toast.success(t('coop.refreshSuccess'));
      void queryClient.invalidateQueries({ queryKey: ['coop-route-detail', routeId] });
      onChanged();
    },
    onError: () => toast.error(t('coop.refreshFailed')),
  });
  const deleteBlocked = p2p && !!d?.isProjectJobRunning;
  const embedded = isEmbeddedRoute(d);

  return (
    <Drawer
      isOpen={!!routeId}
      onClose={onClose}
      title={t('coop.detailTitle', { name: d?.srcNode?.nodeName || '' })}
      footer={
        canWrite && d && !embedded ? (
          <>
            <Button variant="outline" onClick={() => setEditing(d)}>
              {t('common.edit')}
            </Button>
            <span title={deleteBlocked ? t('coop.deleteBlocked') : undefined}>
              <Button variant="danger" disabled={deleteBlocked} onClick={() => setDeleting(d)}>
                {t('common.delete')}
              </Button>
            </span>
          </>
        ) : undefined
      }
    >
      {detailQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
      {detailQuery.error && <div className="text-xs text-rose-500">{errorText(detailQuery.error)}</div>}
      {d && (
        <div className="space-y-4">
          <div>
            <div className="text-sm font-semibold mb-1">{t('coop.coopBasicInfo')}</div>
            <div className="rounded-lg bg-gray-50 dark:bg-gray-850 px-3 py-1">
              {p2p && (
                <>
                  <InfoRow label={t('coop.instName')}>{d.srcNode?.instName || '-'}</InfoRow>
                  <InfoRow label={t('coop.masterNodeId')}>{d.srcNode?.masterNodeId || '-'}</InfoRow>
                </>
              )}
              <InfoRow label={t('coop.computeNodeName')}>{d.srcNode?.nodeName || '-'}</InfoRow>
              <InfoRow label={t('coop.computeNodeId')}>{d.srcNode?.nodeId || d.srcNodeId || '-'}</InfoRow>
              <InfoRow label={t('coop.nodeAddress')}>{d.srcNode?.netAddress || d.srcNetAddress || '-'}</InfoRow>
              {p2p && (
                <>
                  <InfoRow label={t('coop.publicKey')}>
                    <SecretText text={d.srcNode?.certText} />
                  </InfoRow>
                  <InfoRow label={t('coop.authCode')}>
                    <SecretText text={d.srcNode?.nodeAuthenticationCode} />
                  </InfoRow>
                </>
              )}
            </div>
          </div>
          <div className="px-3">
            <InfoRow label={t('coop.selfNode')}>{d.dstNode?.nodeName || d.dstNodeId || '-'}</InfoRow>
            <InfoRow label={t('coop.selfAddress')}>{d.dstNetAddress || '-'}</InfoRow>
            <InfoRow label={t('coop.initiator')}>{d.srcNode?.nodeName || '-'}</InfoRow>
            <InfoRow label={t('coop.commStatus')}>
              <span className="inline-flex items-center gap-2">
                <NodeStatusBadge status={d.status} />
                <Button size="sm" variant="link" loading={refresh.isPending} onClick={() => refresh.mutate()}>
                  {t('common.refresh')}
                </Button>
              </span>
            </InfoRow>
            <InfoRow label={t('coop.cooperateTime')}>{formatTime(d.gmtCreate)}</InfoRow>
            <InfoRow label={t('coop.editTime')}>{formatTime(d.gmtModified)}</InfoRow>
          </div>
        </div>
      )}
      <EditCooperativeNodeModal
        route={editing}
        p2p={p2p}
        onClose={() => setEditing(null)}
        onOk={() => {
          void queryClient.invalidateQueries({ queryKey: ['coop-route-detail', routeId] });
          onChanged();
        }}
      />
      <DeleteCooperativeNodeDialog
        route={deleting}
        p2p={p2p}
        onClose={() => setDeleting(null)}
        onOk={() => {
          onChanged();
          onClose();
        }}
      />
    </Drawer>
  );
};
