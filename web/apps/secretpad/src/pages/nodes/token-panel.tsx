import React, { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Button, CopyButton, ConfirmDialog, toast } from '@secretpad/design-system';
import { getInstNodeTokenJava, getNodeTokenJava, newInstNodeTokenJava, newNodeTokenJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';
import { getTokenExpiry, tokenRefetchInterval } from './token-expiry';

interface TokenView {
  token?: string;
  status?: string;
  issuedAt?: string;
}

/**
 * Deploy token (node/token + node/newToken) or institution node token
 * (inst/node/token + inst/node/newToken). Tokens are valid 30 minutes: the
 * panel shows the remaining validity and refetches automatically.
 */
export const TokenPanel: React.FC<{ kind: 'node' | 'inst'; nodeId: string; canRefresh: boolean }> = ({
  kind,
  nodeId,
  canRefresh,
}) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [confirm, setConfirm] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const key = ['token', kind, nodeId];

  const load = async (): Promise<TokenView> => {
    if (kind === 'node') {
      const r = await getNodeTokenJava(nodeId);
      return { token: r.token, status: r.tokenStatus, issuedAt: r.lastTransitionTime };
    }
    const r = await getInstNodeTokenJava(nodeId);
    return { token: r.instToken, issuedAt: r.createTime };
  };

  const query = useQuery({
    queryKey: key,
    queryFn: load,
    enabled: !!nodeId,
    refetchInterval: (q) => tokenRefetchInterval(q.state.data?.issuedAt),
  });

  // Tick every 30s so the "minutes left" label stays current.
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(id);
  }, []);

  const renew = useMutation({
    mutationFn: async (): Promise<TokenView> => {
      if (kind === 'node') {
        const r = await newNodeTokenJava(nodeId);
        return { token: r.token, status: r.tokenStatus, issuedAt: r.lastTransitionTime || new Date().toISOString() };
      }
      const r = await newInstNodeTokenJava(nodeId);
      return { token: r.instToken, issuedAt: r.createTime || new Date().toISOString() };
    },
    onSuccess: (data) => {
      queryClient.setQueryData(key, data);
      setConfirm(false);
      toast.success(t('nodes.resetTokenSuccess'));
    },
    onError: (e) => {
      setConfirm(false);
      toast.error(errorText(e));
    },
  });

  const data = query.data;
  const expiry = getTokenExpiry(data?.issuedAt, now);
  const used = data?.status === 'used';

  return (
    <div className="space-y-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        {data?.status && <Badge status={used ? 'default' : 'processing'}>{used ? t('nodes.tokenUsed') : t('nodes.tokenUnused')}</Badge>}
        {expiry.expiresAt !== null && (
          <Badge status={expiry.expired ? 'error' : 'success'}>
            {expiry.expired ? t('nodes.tokenExpired') : t('nodes.tokenMinutesLeft', { minutes: expiry.minutesLeft ?? 0 })}
          </Badge>
        )}
        <span className="text-gray-400">{t('nodes.tokenValidity')}</span>
      </div>
      {query.isLoading ? (
        <div className="text-gray-400">{t('common.loading')}</div>
      ) : query.error ? (
        <div className="text-rose-500">{errorText(query.error)}</div>
      ) : (
        <pre className="p-2 rounded bg-gray-50 dark:bg-gray-800 border border-gray-100 dark:border-gray-700 font-mono text-[10px] whitespace-pre-wrap break-all max-h-32 overflow-auto">
          {data?.token || t('nodes.noToken')}
        </pre>
      )}
      <div className="flex items-center gap-3">
        {data?.token && (
          <CopyButton text={data.token} label={t('nodes.copyToken')} onCopied={() => toast.success(t('nodes.tokenCopied'))} />
        )}
        <Button size="sm" variant="link" onClick={() => void query.refetch()} loading={query.isFetching && !query.isLoading}>
          {t('common.refresh')}
        </Button>
        {canRefresh && (
          <Button size="sm" variant="link" onClick={() => setConfirm(true)}>
            {t('nodes.resetToken')}
          </Button>
        )}
      </div>
      <ConfirmDialog
        isOpen={confirm}
        title={t('nodes.resetToken')}
        message={t('nodes.resetTokenConfirm')}
        loading={renew.isPending}
        confirmText={t('nodes.resetToken')}
        cancelText={t('common.cancel')}
        onConfirm={() => renew.mutate()}
        onCancel={() => setConfirm(false)}
      />
    </div>
  );
};
