/**
 * 执行记录回放（旧版 dag-record + record-result-view）：
 * project/job/get 取任务快照 → 只读画布；点节点可看日志与各输出结果（project/job/task/*）。
 * 任务运行中时每 3s 刷新快照状态。
 */
import React, { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Badge } from '@secretpad/design-system';
import type { DAGNode, DAGEdge } from '@secretpad/dag-next';
import { DAGNextWorkspace, isActiveStatus, statusBadge, normalizeStatus } from '@secretpad/dag-next';
import type { ProjectJobJava, ProjectJobSummaryVOJava } from '@secretpad/api-client';
import { getJobTaskLogsJava, getJobTaskOutputJava, getProjectJobJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { mapGraphToDAG } from '../../pages/dag/adapters';
import { useResultActions } from './use-result-actions';

export interface JobReplayViewProps {
  projectId: string;
  jobId: string;
  computeMode?: string;
  summary?: ProjectJobSummaryVOJava;
  /** 自定义快照加载（周期任务等）。 */
  loadJob?: () => Promise<ProjectJobJava>;
  height?: number | string;
}

function countResults(job: ProjectJobJava | undefined) {
  const counts = { table: 0, model: 0, rule: 0, report: 0 };
  job?.graph.nodes.forEach((n) =>
    (n.results || []).forEach((r) => {
      const k = String(r.kind || '').toLowerCase();
      if (k.includes('table')) counts.table += 1;
      else if (k.includes('model')) counts.model += 1;
      else if (k.includes('rule')) counts.rule += 1;
      else if (k.includes('report')) counts.report += 1;
    }),
  );
  return counts;
}

export const JobReplayView: React.FC<JobReplayViewProps> = ({ projectId, jobId, computeMode, summary, loadJob, height = 520 }) => {
  const { t } = useTranslation();
  const jobQuery = useQuery({
    queryKey: ['job-replay', projectId, jobId],
    queryFn: () => (loadJob ? loadJob() : getProjectJobJava(projectId, jobId)),
    enabled: !!projectId && !!jobId,
    refetchInterval: (q) => {
      const d = q.state.data as ProjectJobJava | undefined;
      return d && (isActiveStatus(d.status) || d.graph.nodes.some((n) => isActiveStatus(n.status))) ? 3000 : false;
    },
  });
  const job = jobQuery.data;
  const { nodes, edges } = useMemo(() => mapGraphToDAG(job?.graph), [job]);
  const taskIdOf = useMemo(() => {
    const map = new Map<string, string>();
    job?.graph.nodes.forEach((n) => map.set(n.graphNodeId, n.taskId || `${jobId}-${n.graphNodeId}`));
    return map;
  }, [job, jobId]);
  const { downloadMode, actions, modal } = useResultActions({ projectId, graphId: job?.graph.graphId, computeMode });
  const counts = summary
    ? { table: summary.tableCount ?? 0, model: summary.modelCount ?? 0, rule: summary.ruleCount ?? 0, report: summary.reportCount ?? 0 }
    : countResults(job);

  const onNodeLogs = async (node: DAGNode) => {
    const res = await getJobTaskLogsJava({ projectId, jobId, taskId: taskIdOf.get(node.id) || `${jobId}-${node.id}` });
    return { status: res.status, logs: res.logs };
  };
  const onNodeOutput = async (node: DAGNode, outputId?: string) => {
    const out = outputId || node.outputs?.[0];
    if (!out) return null;
    return getJobTaskOutputJava({ projectId, jobId, taskId: taskIdOf.get(node.id) || `${jobId}-${node.id}`, outputId: out });
  };

  return (
    <div className="space-y-2 text-xs">
      {jobQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
      {jobQuery.error && <div className="text-red-500">{String((jobQuery.error as Error).message)}</div>}
      {job && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <span className="font-mono">{job.jobId}</span>
            <Badge status={statusBadge(job.status) === 'warning' ? 'warning' : statusBadge(job.status)}>{normalizeStatus(job.status) ?? job.status}</Badge>
            <span className="text-gray-500">{t('dagx.recordStats', counts)}</span>
          </div>
          {job.errMsg && <div className="text-red-500 bg-red-50 dark:bg-red-950/30 p-2 rounded break-all">{job.errMsg}</div>}
          <div style={{ height }}>
            <DAGNextWorkspace
              readOnly
              title={job.graph.name || job.jobId}
              initialNodes={nodes as DAGNode[]}
              initialEdges={edges as DAGEdge[]}
              onNodeLogs={onNodeLogs}
              onNodeOutput={onNodeOutput}
              outputViewProps={() => ({ downloadMode, actions })}
              labels={{ logs: t('dag.logs'), output: t('dag.output'), noLogs: t('dag.noLogs'), noOutput: t('dag.noOutput'), refresh: t('dag.refresh') }}
            />
          </div>
        </>
      )}
      {modal}
    </div>
  );
};
