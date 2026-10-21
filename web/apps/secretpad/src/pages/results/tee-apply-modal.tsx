import React from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Badge, Button, Modal, toast } from '@secretpad/design-system';
import { applyTeeDownloadJava, pullTeeStatusJava, toTeeResourceType, type NodeResultJava, type PullStatusPartyJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';

/**
 * TEE result download approval (legacy dag-result/apply-download):
 * approval/pull/status lists parties; each party applies via approval/create
 * voteType TEE_DOWNLOAD with voteConfig {jobID, taskID, resourceType, resourceID, projectID, graphID}.
 */
export const TeeApplyModal: React.FC<{ result: NodeResultJava | null; onClose: () => void }> = ({ result, onClose }) => {
  const { t } = useTranslation();
  // 后端只接受 model / rule / table；report / read_data 等无法映射时不发请求（列表也已隐藏入口）。
  const resourceType = toTeeResourceType(result?.datatableType);
  const req = result && resourceType
    ? {
        projectID: result.sourceProjectId || '',
        jobID: result.jobId || '',
        resourceID: result.domainDataId || '',
        resourceType,
      }
    : null;
  const statusQuery = useQuery({
    queryKey: ['tee-pull-status', req],
    queryFn: () => pullTeeStatusJava(req!),
    enabled: !!req,
  });
  const st = statusQuery.data;

  const apply = useMutation({
    mutationFn: (party: PullStatusPartyJava) =>
      applyTeeDownloadJava(party.nodeID || '', {
        jobID: st?.jobID || req?.jobID,
        taskID: st?.taskID,
        resourceType: toTeeResourceType(st?.resourceType) || req?.resourceType,
        resourceID: st?.resourceID || req?.resourceID,
        projectID: req?.projectID,
        graphID: st?.graphID,
      }),
    onSuccess: () => {
      toast.success(t('results.teeApplySuccess'));
      void statusQuery.refetch();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const renderStatus = (p: PullStatusPartyJava) => {
    switch (p.status) {
      case 'REVIEWING':
        return <span className="text-gray-400">{t('results.teeReviewing')}</span>;
      case 'APPROVED':
        return <Badge status="success">{t('results.teeApproved')}</Badge>;
      case 'REJECTED':
        return (
          <span className="inline-flex items-center gap-2">
            <Badge status="error">{t('results.teeRejected')}</Badge>
            <Button size="sm" variant="link" loading={apply.isPending} onClick={() => apply.mutate(p)}>
              {t('results.teeApplyAgain')}
            </Button>
          </span>
        );
      default:
        return (
          <Button size="sm" variant="link" loading={apply.isPending} onClick={() => apply.mutate(p)}>
            {t('results.teeApply')}
          </Button>
        );
    }
  };

  return (
    <Modal
      isOpen={!!result}
      onClose={onClose}
      title={t('results.teeApplyTitle')}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      <div className="space-y-3 text-xs">
        <div className="rounded-lg px-3 py-2 bg-amber-50 dark:bg-amber-950/30 text-amber-700 dark:text-amber-300">
          {t('results.teeApplyHint')}
        </div>
        {statusQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
        {statusQuery.error && <div className="text-rose-500">{errorText(statusQuery.error)}</div>}
        {(st?.parties || []).map((p) => (
          <div key={p.nodeID} className="rounded-lg border border-gray-200 dark:border-gray-800 p-2.5">
            <div className="flex items-center justify-between">
              <span>
                {p.nodeName || p.nodeID} ({p.nodeID})
              </span>
              {renderStatus(p)}
            </div>
            {(p.voteInfos || [])
              .filter((v) => v.action === 'REJECTED')
              .map((v) => (
                <div key={v.nodeID} className="mt-1 text-[11px] text-gray-500">
                  {t('results.teeRejectReason', { node: v.nodeID || '', reason: v.reason || '' })}
                </div>
              ))}
          </div>
        ))}
      </div>
    </Modal>
  );
};
