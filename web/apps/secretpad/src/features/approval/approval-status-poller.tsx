import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Card, Badge, Button } from '@secretpad/design-system';
import type { PullStatusVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

/**
 * 审批状态轮询组件。
 *
 * 对应后端 `POST /api/v1alpha1/approval/pull/status`（`ApprovalController#pullStatus`）。
 * 用户填写资源定位字段后开启轮询，组件以固定间隔重复请求并展示各参与方
 * （节点维度）的投票状态与明细，直至用户手动停止。
 */

/** 轮询间隔（毫秒）。 */
const POLL_INTERVAL_MS = 3000;

/** 资源类型候选值（与后端 `resourceType` 约定一致：model / rule / table）。 */
const RESOURCE_TYPES = ['model', 'rule', 'table'] as const;

/** 统一的输入框样式。 */
const INPUT_CLASS =
  'w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 text-xs focus:outline-none focus:border-blue-500';

/** 将参与方状态映射为 Badge 语义色。 */
function partyStatusBadge(status?: string): 'success' | 'warning' | 'error' | 'default' {
  switch ((status || '').toUpperCase()) {
    case 'AGREE':
    case 'APPROVED':
    case 'SUCCEED':
      return 'success';
    case 'PENDING':
    case 'WAITING':
    case 'REVIEWING':
      return 'warning';
    case 'REJECT':
    case 'REJECTED':
    case 'FAILED':
      return 'error';
    default:
      return 'default';
  }
}

export const ApprovalStatusPoller: React.FC = () => {
  const { t } = useTranslation();

  // 资源定位字段（pullApprovalStatus 的入参）。
  const [projectID, setProjectID] = useState('');
  const [jobID, setJobID] = useState('');
  const [taskID, setTaskID] = useState('');
  const [resourceID, setResourceID] = useState('');
  const [resourceType, setResourceType] = useState<string>('model');
  // 是否处于轮询状态（控制 useQuery 的 enabled 与 refetchInterval）。
  const [polling, setPolling] = useState(false);

  // 四个必填字段齐备后才允许发起请求。
  const ready =
    projectID.trim() !== '' && jobID.trim() !== '' && taskID.trim() !== '' && resourceID.trim() !== '';

  const statusQuery = useQuery<PullStatusVO>({
    queryKey: ['approval-pull-status', projectID, jobID, taskID, resourceID, resourceType],
    queryFn: () =>
      apiClient.pullApprovalStatus({
        projectID: projectID.trim(),
        jobID: jobID.trim(),
        taskID: taskID.trim(),
        resourceID: resourceID.trim(),
        resourceType,
      }),
    // 仅在用户开启轮询且字段齐备时请求；轮询间隔由 refetchInterval 控制。
    enabled: polling && ready,
    refetchInterval: polling ? POLL_INTERVAL_MS : false,
    refetchOnWindowFocus: false,
  });

  const parties = statusQuery.data?.parties ?? [];

  return (
    <Card
      title={t('approval.pollerTitle')}
      extra={
        <Button
          size="sm"
          variant={polling ? 'danger' : 'primary'}
          disabled={!ready}
          onClick={() => setPolling((p) => !p)}
        >
          {polling ? t('approval.stopPolling') : t('approval.startPolling')}
        </Button>
      }
      bodyClassName="p-4"
    >
      <div className="space-y-3 text-xs">
        {/* 资源定位表单：四必填 + 资源类型下拉 */}
        <div className="grid grid-cols-2 md:grid-cols-5 gap-2">
          <input value={projectID} onChange={(e) => setProjectID(e.target.value)} className={INPUT_CLASS} placeholder={t('approval.field.projectID')} />
          <input value={jobID} onChange={(e) => setJobID(e.target.value)} className={INPUT_CLASS} placeholder={t('approval.field.jobID')} />
          <input value={taskID} onChange={(e) => setTaskID(e.target.value)} className={INPUT_CLASS} placeholder={t('approval.field.taskID')} />
          <input value={resourceID} onChange={(e) => setResourceID(e.target.value)} className={INPUT_CLASS} placeholder={t('approval.field.resourceID')} />
          <select value={resourceType} onChange={(e) => setResourceType(e.target.value)} className={INPUT_CLASS}>
            {RESOURCE_TYPES.map((rt) => (
              <option key={rt} value={rt}>
                {rt}
              </option>
            ))}
          </select>
        </div>

        {/* 轮询进行中提示 */}
        {polling && (
          <div className="text-blue-500 dark:text-blue-400">{t('approval.pollingHint')}</div>
        )}

        {/* 请求错误提示 */}
        {statusQuery.error && (
          <div className="text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-3 py-2">
            {t('common.error', { message: statusQuery.error.message })}
          </div>
        )}

        {/* 轮询结果：参与方（节点）投票状态与明细 */}
        {statusQuery.data && (
          <div className="space-y-2">
            {parties.length === 0 && (
              <div className="text-gray-400">{t('approval.noParties')}</div>
            )}
            {parties.map((party, idx) => (
              <div
                key={`${party.nodeID ?? 'node'}-${idx}`}
                className="border border-gray-100 dark:border-gray-800 rounded-lg p-3 space-y-1.5"
              >
                <div className="flex items-center justify-between">
                  <span className="font-semibold text-gray-900 dark:text-gray-100">
                    {party.nodeName || party.nodeID || t('approval.unknownNode')}
                  </span>
                  <Badge status={partyStatusBadge(party.status)}>
                    {party.status || t('messages.statusUnknown')}
                  </Badge>
                </div>
                {/* 该节点下的逐次投票明细 */}
                {(party.voteInfos ?? []).map((vote, vIdx) => (
                  <div key={`${vote.voteID ?? 'vote'}-${vIdx}`} className="text-gray-500 pl-2 border-l-2 border-gray-100 dark:border-gray-800">
                    <span className="font-medium text-gray-700 dark:text-gray-300">{vote.action || '-'}</span>
                    {vote.reason && <span className="ml-2">· {vote.reason}</span>}
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </Card>
  );
};
