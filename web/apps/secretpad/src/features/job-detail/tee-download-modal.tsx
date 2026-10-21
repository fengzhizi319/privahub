/**
 * TEE 结果下载审批（旧版 dag-result/apply-download.tsx）：
 * approval/pull/status 查询各参与方申请状态，approval/create（voteType TEE_DOWNLOAD）发起申请。
 */
import React from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Button, Modal, toast } from '@secretpad/design-system';
import { applyTeeDownloadJava, pullTeeStatusJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

export interface TeeDownloadTarget {
  projectId: string;
  jobId: string;
  taskId?: string;
  resourceId: string;
  resourceType?: string;
  graphId?: string;
}

const STATUS_BADGE: Record<string, 'success' | 'processing' | 'error' | 'default'> = {
  APPROVED: 'success',
  REVIEWING: 'processing',
  REJECTED: 'error',
  NOT_INITIATED: 'default',
};

export const TeeDownloadModal: React.FC<{ target: TeeDownloadTarget | null; onClose: () => void }> = ({ target, onClose }) => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const key = ['tee-pull-status', target?.projectId, target?.jobId, target?.taskId, target?.resourceId];
  const statusQuery = useQuery({
    queryKey: key,
    queryFn: () =>
      pullTeeStatusJava({
        projectID: target!.projectId,
        jobID: target!.jobId,
        taskID: target!.taskId,
        resourceID: target!.resourceId,
        resourceType: target!.resourceType,
      }),
    enabled: !!target,
  });
  const apply = useMutation({
    mutationFn: (nodeID: string) =>
      applyTeeDownloadJava(nodeID, {
        jobID: statusQuery.data?.jobID ?? target!.jobId,
        taskID: statusQuery.data?.taskID ?? target!.taskId,
        resourceType: statusQuery.data?.resourceType ?? target!.resourceType,
        resourceID: statusQuery.data?.resourceID ?? target!.resourceId,
        projectID: target!.projectId,
        graphID: statusQuery.data?.graphID ?? target!.graphId,
      }),
    onSuccess: () => {
      toast.success(t('dagx.teeApplied'));
      void queryClient.invalidateQueries({ queryKey: key });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });
  const parties = statusQuery.data?.parties ?? [];
  return (
    <Modal isOpen={!!target} onClose={onClose} title={t('dagx.teeDownloadTitle')} footer={<Button onClick={onClose}>{t('common.close')}</Button>}>
      <div className="text-xs space-y-2 max-h-[60vh] overflow-auto">
        {statusQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
        {statusQuery.error && <div className="text-red-500">{String((statusQuery.error as Error).message)}</div>}
        <div className="text-gray-500 break-all">{target?.resourceId}</div>
        {parties.map((p) => {
          const st = p.status || 'NOT_INITIATED';
          return (
            <div key={p.nodeID} className="flex items-center justify-between p-2 rounded-lg border border-gray-200 dark:border-gray-700">
              <span className="font-medium">{p.nodeName || p.nodeID}</span>
              <span className="flex items-center gap-2">
                <Badge status={STATUS_BADGE[st] ?? 'default'}>{t(`dagx.teeStatus.${st}`)}</Badge>
                {(st === 'NOT_INITIATED' || st === 'REJECTED') && (
                  <Button size="sm" variant="primary" loading={apply.isPending} onClick={() => p.nodeID && apply.mutate(p.nodeID)}>
                    {t('dagx.teeApply')}
                  </Button>
                )}
              </span>
            </div>
          );
        })}
      </div>
    </Modal>
  );
};
