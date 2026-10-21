/**
 * graph/node/status 轮询：
 * - 任一节点 INITIALIZED / RUNNING，或刚发起运行而任务尚未 finished（节点可能仍为 STAGING）时每 3s 轮询；
 * - 由“运行中”转为“结束”时给出完成提示（全部成功 / 有失败）。
 */
import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { GraphStatusJava } from '@secretpad/api-client';
import { listGraphNodeStatusJava } from '@secretpad/api-client';
import { allSucceeded, anyFailed, shouldPollStatus } from './adapters';
import { isActiveStatus } from '@secretpad/dag-next';

export function useNodeStatus(opts: {
  projectId: string;
  graphId?: string;
  intervalMs?: number;
  onFinished?: (result: 'success' | 'failed' | 'stopped', status: GraphStatusJava) => void;
}) {
  const { projectId, graphId, intervalMs = 3000, onFinished } = opts;
  const queryClient = useQueryClient();
  const [runRequested, setRunRequested] = useState(false);
  const runRequestedRef = useRef(false);
  const wasActiveRef = useRef(false);
  /** 发起运行后收到的状态响应次数：避免把上一轮任务的 finished 误判为本轮结束。 */
  const pollsSinceStartRef = useRef(0);
  const finishedRef = useRef(onFinished);
  useEffect(() => {
    finishedRef.current = onFinished;
    runRequestedRef.current = runRequested;
  });

  const key = ['graph-node-status', projectId, graphId];
  const query = useQuery({
    queryKey: key,
    queryFn: () => listGraphNodeStatusJava(projectId, graphId!),
    enabled: !!projectId && !!graphId,
    refetchInterval: (q) =>
      shouldPollStatus(q.state.data as GraphStatusJava | undefined, runRequestedRef.current) ||
      (runRequestedRef.current && !wasActiveRef.current && pollsSinceStartRef.current < 3)
        ? intervalMs
        : false,
  });

  const data = query.data;
  useEffect(() => {
    if (!data) return;
    if (runRequestedRef.current) pollsSinceStartRef.current += 1;
    const active = data.nodes.some((n) => isActiveStatus(n.status));
    const waitingForStart = runRequestedRef.current && !wasActiveRef.current && !active && pollsSinceStartRef.current < 3;
    const polling = waitingForStart || shouldPollStatus(data, runRequestedRef.current);
    if (polling) {
      if (active) wasActiveRef.current = true;
      return;
    }
    if (wasActiveRef.current || runRequestedRef.current) {
      wasActiveRef.current = false;
      setRunRequested(false);
      runRequestedRef.current = false;
      const result = allSucceeded(data) ? 'success' : anyFailed(data) ? 'failed' : 'stopped';
      finishedRef.current?.(result, data);
    }
  }, [data, query.dataUpdatedAt]);

  // 切换图时重置。
  useEffect(() => {
    wasActiveRef.current = false;
    setRunRequested(false);
  }, [graphId]);

  /** 发起运行后调用：开始轮询（即使节点仍为 STAGING）。 */
  const markRunStarted = () => {
    runRequestedRef.current = true;
    pollsSinceStartRef.current = 0;
    wasActiveRef.current = false;
    setRunRequested(true);
    void queryClient.invalidateQueries({ queryKey: key });
  };

  const refresh = () => void queryClient.invalidateQueries({ queryKey: key });

  return { status: data ?? null, runRequested, markRunStarted, refresh, isFetching: query.isFetching };
}
