import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Drawer, FormField, Input, RadioGroup, Select, Textarea, toast } from '@secretpad/design-system';
import {
  apiClient,
  createNodeRouteApprovalJava,
  createP2pNodeJava,
  getNodeJava,
  listMyInstNodesJava,
  listRouteNodesJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { getProtocol, isValidNetAddress, parseAuthCode, stripProtocol, type NetProtocol } from './auth-code';
import { errorText } from './format';
import { AddressInput } from './ui-common';

export interface AddCooperativeNodeDrawerProps {
  open: boolean;
  onClose: () => void;
  onOk: () => void;
  /** P2P/AUTONOMY flow (p2p/node/create) instead of the CENTER/EDGE approval flow. */
  p2p: boolean;
  /** Fixed own node (node context / EDGE). When unset the user picks one. */
  ownerNodeId?: string;
}

interface FormState {
  authCode: string;
  instName: string;
  instId: string;
  masterNodeId: string;
  dstNodeName: string;
  dstNodeId: string;
  dstAddress: string;
  dstProtocol: NetProtocol;
  certText: string;
  srcNodeId: string;
  srcAddress: string;
  srcProtocol: NetProtocol;
  routeType: 'bidirectional' | 'single';
}

const EMPTY: FormState = {
  authCode: '',
  instName: '',
  instId: '',
  masterNodeId: '',
  dstNodeName: '',
  dstNodeId: '',
  dstAddress: '',
  dstProtocol: 'http://',
  certText: '',
  srcNodeId: '',
  srcAddress: '',
  srcProtocol: 'http://',
  routeType: 'bidirectional',
};

/** Legacy `add-cooperative-node-modal.tsx`. */
export const AddCooperativeNodeDrawer: React.FC<AddCooperativeNodeDrawerProps> = ({ open, onClose, onOk, p2p, ownerNodeId }) => {
  const { t } = useTranslation();
  const [form, setForm] = useState<FormState>({ ...EMPTY, srcNodeId: ownerNodeId || '' });
  const [authState, setAuthState] = useState<'idle' | 'ok' | 'error'>('idle');
  const [touched, setTouched] = useState(false);
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    if (open) {
      setForm({ ...EMPTY, srcNodeId: ownerNodeId || '' });
      setAuthState('idle');
      setTouched(false);
    }
  }, [open, ownerNodeId]);

  // Own-node candidates.
  const selfNodesQuery = useQuery({
    queryKey: ['coop-self-nodes', p2p],
    queryFn: async () => {
      if (p2p) return listMyInstNodesJava();
      const nodes = await apiClient.getNodes();
      return nodes.map((n) => ({ nodeId: n.nodeId, nodeName: n.nodeName, nodeStatus: n.nodeStatus }));
    },
    enabled: open && !ownerNodeId,
  });
  const selfOptions = (selfNodesQuery.data || [])
    .filter((n) => n.nodeStatus === 'Ready')
    .map((n) => ({ value: n.nodeId || '', label: n.nodeName || n.nodeId || '' }));

  // Own node info → prefill own address.
  const selfInfoQuery = useQuery({
    queryKey: ['coop-self-node', form.srcNodeId],
    queryFn: () => getNodeJava(form.srcNodeId),
    enabled: open && !!form.srcNodeId,
  });
  useEffect(() => {
    const addr = selfInfoQuery.data?.netAddress;
    if (addr) {
      setForm((f) => ({ ...f, srcAddress: stripProtocol(addr), srcProtocol: getProtocol(addr, selfInfoQuery.data?.protocol) }));
    }
  }, [selfInfoQuery.data]);

  // CENTER/EDGE: candidate cooperative nodes.
  const routeNodesQuery = useQuery({
    queryKey: ['coop-route-nodes'],
    queryFn: listRouteNodesJava,
    enabled: open && !p2p,
  });
  const routeNodes = useMemo(() => routeNodesQuery.data || [], [routeNodesQuery.data]);
  const dstOptions = routeNodes
    .filter((n) => n.nodeId !== form.srcNodeId && n.nodeStatus === 'Ready')
    .map((n) => ({ value: n.nodeId || '', label: n.nodeName || n.nodeId || '' }));

  const onSelectDst = (nodeId: string) => {
    const node = routeNodes.find((n) => n.nodeId === nodeId || n.controlNodeId === nodeId);
    setForm((f) => ({
      ...f,
      dstNodeId: nodeId,
      dstNodeName: node?.nodeName || '',
      dstAddress: stripProtocol(node?.netAddress),
      dstProtocol: getProtocol(node?.netAddress, node?.protocol),
    }));
  };

  const parse = () => {
    const parsed = parseAuthCode(form.authCode);
    if (!parsed) {
      setAuthState('error');
      return;
    }
    setAuthState('ok');
    setForm((f) => ({
      ...f,
      certText: parsed.certText,
      dstNodeId: parsed.dstNodeId,
      dstNodeName: parsed.name,
      dstAddress: parsed.dstNetAddress,
      dstProtocol: parsed.protocol,
      instName: parsed.instName,
      instId: parsed.instId,
      masterNodeId: parsed.masterNodeId,
    }));
  };

  const errors: Partial<Record<keyof FormState, string>> = {};
  const req = t('coop.required');
  if (p2p) {
    if (!form.instName) errors.instName = t('coop.parseToFill');
    if (!form.instId) errors.instId = t('coop.parseToFill');
    if (!form.masterNodeId) errors.masterNodeId = t('coop.parseToFill');
    if (!form.certText) errors.certText = req;
    if (!form.dstNodeId) errors.dstNodeId = req;
  }
  if (!form.dstNodeName && p2p) errors.dstNodeName = req;
  if (!p2p && !form.dstNodeId) errors.dstNodeName = req;
  if (form.dstNodeName.length > 32) errors.dstNodeName = t('coop.nameTooLong');
  if (!form.dstAddress) errors.dstAddress = req;
  else if (!isValidNetAddress(form.dstAddress)) errors.dstAddress = t('coop.invalidAddress');
  if (!form.srcNodeId) errors.srcNodeId = req;
  if (!form.srcAddress) errors.srcAddress = req;
  else if (!isValidNetAddress(form.srcAddress)) errors.srcAddress = t('coop.invalidAddress');
  const valid = Object.keys(errors).length === 0;
  const err = (k: keyof FormState) => (touched ? errors[k] : undefined);

  const submit = useMutation({
    mutationFn: async () => {
      const srcNetAddress = `${form.srcProtocol}${form.srcAddress}`;
      const dstNetAddress = `${form.dstProtocol}${form.dstAddress}`;
      if (p2p) {
        return createP2pNodeJava({
          mode: 1,
          masterNodeId: form.masterNodeId,
          dstNodeId: form.dstNodeId,
          name: form.dstNodeName,
          certText: form.certText,
          srcNetAddress,
          dstNetAddress,
          dstInstId: form.instId,
          dstInstName: form.instName,
          srcNodeId: form.srcNodeId,
        });
      }
      return createNodeRouteApprovalJava(form.srcNodeId, {
        srcNodeId: form.srcNodeId,
        desNodeId: form.dstNodeId,
        srcNodeAddr: srcNetAddress,
        desNodeAddr: dstNetAddress,
        isSingle: form.routeType === 'single',
      });
    },
    onSuccess: () => {
      toast.success(p2p ? t('coop.addSuccess') : t('coop.addApprovalSuccess'));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const handleOk = () => {
    setTouched(true);
    if (valid) submit.mutate();
  };

  return (
    <Drawer
      isOpen={open}
      onClose={onClose}
      title={t('coop.addTitle')}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={handleOk} loading={submit.isPending} disabled={touched && !valid}>
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {p2p && (
          <div className="text-xs rounded-lg px-3 py-2 bg-amber-50 dark:bg-amber-950/30 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-900">
            {t('coop.p2pHint')}
          </div>
        )}
        <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('coop.cooperativeNode')}</div>
        {p2p && (
          <>
            <FormField
              label={
                <span className="flex items-center justify-between w-full">
                  <span>{t('coop.authCode')}</span>
                  <button
                    type="button"
                    disabled={!form.authCode}
                    className="text-blue-600 disabled:text-gray-400 hover:underline"
                    onClick={parse}
                  >
                    {t('coop.parse')}
                  </button>
                </span>
              }
              error={authState === 'error' ? t('coop.parseFailed') : undefined}
            >
              <Textarea
                rows={4}
                value={form.authCode}
                placeholder={t('coop.authCodePlaceholder')}
                onChange={(e) => {
                  set('authCode', e.target.value);
                  if (!e.target.value) setAuthState('idle');
                }}
              />
            </FormField>
            {authState === 'ok' && (
              <div className="text-xs rounded-lg px-3 py-2 bg-emerald-50 dark:bg-emerald-950/30 text-emerald-700 dark:text-emerald-300">
                {t('coop.parseOk')}
              </div>
            )}
            <FormField label={t('coop.instName')} required error={err('instName')}>
              <Input value={form.instName} onChange={(e) => set('instName', e.target.value)} placeholder={t('coop.parseToFill')} />
            </FormField>
            <FormField label={t('coop.instId')} required error={err('instId')}>
              <Input value={form.instId} onChange={(e) => set('instId', e.target.value)} placeholder={t('coop.parseToFill')} />
            </FormField>
            <FormField label={t('coop.masterNodeId')} required error={err('masterNodeId')}>
              <Input
                value={form.masterNodeId}
                onChange={(e) => set('masterNodeId', e.target.value)}
                placeholder={t('coop.parseToFill')}
              />
            </FormField>
          </>
        )}
        <FormField label={t('coop.computeNodeName')} required error={err('dstNodeName')}>
          {p2p ? (
            <Input value={form.dstNodeName} onChange={(e) => set('dstNodeName', e.target.value)} />
          ) : (
            <Select
              value={form.dstNodeId}
              placeholder={routeNodesQuery.isLoading ? t('common.loading') : t('coop.select')}
              options={dstOptions}
              onChange={onSelectDst}
            />
          )}
        </FormField>
        <FormField label={t('coop.computeNodeId')} required={p2p} error={err('dstNodeId')}>
          <Input
            value={form.dstNodeId}
            disabled={!p2p}
            placeholder={p2p ? '' : t('coop.autoFill')}
            onChange={(e) => set('dstNodeId', e.target.value)}
          />
        </FormField>
        <FormField label={t('coop.nodeAddress')} required error={err('dstAddress')}>
          <AddressInput
            protocol={form.dstProtocol}
            onProtocolChange={(v) => set('dstProtocol', v)}
            value={form.dstAddress}
            onChange={(v) => set('dstAddress', v)}
            invalid={!!err('dstAddress')}
          />
        </FormField>
        {p2p && (
          <FormField label={t('coop.publicKey')} required error={err('certText')}>
            <Textarea rows={3} value={form.certText} onChange={(e) => set('certText', e.target.value)} />
          </FormField>
        )}
        {!p2p && (
          <FormField label={t('coop.routeType')} help={t('coop.routeTypeHelp')}>
            <RadioGroup
              value={form.routeType}
              onChange={(v) => set('routeType', v as FormState['routeType'])}
              options={[
                { value: 'bidirectional', label: t('coop.routeBidirectional') },
                { value: 'single', label: t('coop.routeSingle') },
              ]}
            />
          </FormField>
        )}

        <div className="text-sm font-semibold text-gray-800 dark:text-gray-200 pt-2">{t('coop.selfNode')}</div>
        {!ownerNodeId ? (
          <FormField label={t('coop.selectSelfNode')} required error={err('srcNodeId')}>
            <Select
              value={form.srcNodeId}
              placeholder={t('coop.select')}
              options={selfOptions}
              onChange={(v) => set('srcNodeId', v)}
            />
          </FormField>
        ) : (
          <FormField label={t('coop.selfNode')}>
            <Input value={selfInfoQuery.data?.nodeName || ownerNodeId} disabled />
          </FormField>
        )}
        <FormField label={t('coop.nodeAddress')} required error={err('srcAddress')}>
          <AddressInput
            protocol={form.srcProtocol}
            onProtocolChange={(v) => set('srcProtocol', v)}
            value={form.srcAddress}
            onChange={(v) => set('srcAddress', v)}
            invalid={!!err('srcAddress')}
          />
        </FormField>
      </div>
    </Drawer>
  );
};
