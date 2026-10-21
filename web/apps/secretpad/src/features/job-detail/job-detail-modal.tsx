/**
 * 任务详情弹窗（跨页面复用）。
 *
 * 与 DAG 画布内的记录回放共用 `JobReplayView`：只读画布快照 + 节点日志 + 按 DistData 类型
 * 渲染的输出（表 / 模型 / 规则 / 报告），状态常量统一使用 `SUCCEED`（见 dag-next status.ts）。
 */
import React from 'react';
import { Modal, Button } from '@secretpad/design-system';
import type { ProjectJobSummaryVOJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { JobReplayView } from './job-replay';

interface JobDetailModalProps {
  projectId: string;
  jobId: string;
  isOpen: boolean;
  onClose: () => void;
  computeMode?: string;
  summary?: ProjectJobSummaryVOJava;
}

export const JobDetailModal: React.FC<JobDetailModalProps> = ({ projectId, jobId, isOpen, onClose, computeMode, summary }) => {
  const { t } = useTranslation();
  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      width="max-w-6xl"
      title={t('dagx.recordReplay')}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      {isOpen && <JobReplayView projectId={projectId} jobId={jobId} computeMode={computeMode} summary={summary} height="60vh" />}
    </Modal>
  );
};
