/**
 * 结果面板操作（下载 / TEE 下载申请 / 查看节点结果），与旧版 result-table / result-model 一致：
 * - AUTONOMY（P2P）：直接下载（data/download）；
 * - TEE 项目（非 AUTONOMY）：发起 TEE_DOWNLOAD 审批；
 * - 其他：仅展示路径与“查看结果”（内置节点）。
 */
import React, { useMemo, useState } from 'react';
import { toast } from '@secretpad/design-system';
import { apiClient, toTeeResourceType } from '@secretpad/api-client';
import type { NormalizedOutput, OutputRow, ResultActions } from '@secretpad/dag-next';
import { usePlatform } from '../../shared/lib/platform';
import { TeeDownloadModal } from './tee-download-modal';
import type { TeeDownloadTarget } from './tee-download-modal';

export function triggerBlobDownload(blob: Blob, filename: string) {
  if (typeof window === 'undefined' || !window.URL?.createObjectURL) return;
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.URL.revokeObjectURL(url);
}

export function useResultActions(opts: { projectId: string; graphId?: string; computeMode?: string }) {
  const { isAutonomy } = usePlatform();
  const [teeTarget, setTeeTarget] = useState<TeeDownloadTarget | null>(null);
  const isTee = (opts.computeMode || '').toUpperCase() === 'TEE';
  const downloadMode: 'direct' | 'tee' | 'none' = isAutonomy ? 'direct' : isTee ? 'tee' : 'none';

  const actions: ResultActions = useMemo(
    () => ({
      onDownload: async (row: OutputRow) => {
        const id = row.tableId || row.domainDataId;
        if (!row.nodeId || !id) return;
        try {
          const blob = await apiClient.downloadData({ nodeId: row.nodeId, domainDataId: id });
          triggerBlobDownload(blob, id);
        } catch (e) {
          toast.error(e instanceof Error ? e.message : String(e));
        }
      },
      onApplyTeeDownload: (row: OutputRow, output: NormalizedOutput) => {
        setTeeTarget({
          projectId: opts.projectId,
          graphId: opts.graphId ?? output.graphId,
          jobId: output.jobId ?? '',
          taskId: output.taskId,
          resourceId: row.tableId || row.domainDataId || '',
          resourceType: toTeeResourceType(output.rawType) ?? toTeeResourceType(output.kind),
        });
      },
      // approval/pull/status 只接受 model / rule / table：其余类型（report、read_data…）隐藏申请入口。
      canApplyTeeDownload: (output: NormalizedOutput) => !!(toTeeResourceType(output.rawType) ?? toTeeResourceType(output.kind)),
      onViewNodeResult: (row: OutputRow) => {
        // node/result/list nameFilter matches the domainDataId (ref_id), not the file path.
        const name = row.tableId || row.domainDataId || '';
        if (typeof window !== 'undefined') window.open(`/results?ownerId=${encodeURIComponent(row.nodeId || '')}&resultName=${encodeURIComponent(name)}`, '_blank');
      },
    }),
    [opts.projectId, opts.graphId],
  );

  const modal = <TeeDownloadModal target={teeTarget} onClose={() => setTeeTarget(null)} />;
  return { downloadMode, actions, modal };
}
