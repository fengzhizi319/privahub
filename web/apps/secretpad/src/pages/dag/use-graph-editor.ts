/**
 * 画布编辑状态：脏标记 + 防抖自动保存 + 刷新时合并（不覆盖本地未保存改动）。
 *
 * - `initialNodes / initialEdges` 仅在「切换图」或「服务端数据变化且本地无未保存改动」时替换，
 *   引用保持稳定（dag-next 以引用变化触发全量同步）；
 * - 本地有未保存改动时，服务端刷新只用于补充组件定义带来的端口 / 完成度信息，结构以本地为准；
 * - 状态（运行中 / 成功…）由 graph/node/status 轮询以覆盖层方式传给画布，不参与合并。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { DAGEdge, DAGNode } from '@secretpad/dag-next';
import type { GraphDetailJava } from '@secretpad/api-client';
import type { ComponentDefLike } from './adapters';
import { decorateNode, mapGraphToDAG } from './adapters';

export type SaveState = 'idle' | 'dirty' | 'saving' | 'saved';

interface Options {
  graphKey: string;
  server: GraphDetailJava | null | undefined;
  defs: Record<string, ComponentDefLike | undefined>;
  translate?: (s: string) => string;
  save: (nodes: DAGNode[], edges: DAGEdge[]) => Promise<void>;
  onSaveError?: (e: unknown) => void;
  debounceMs?: number;
  /** 只读时不自动保存。 */
  readOnly?: boolean;
}

export function useGraphEditor({ graphKey, server, defs, translate, save, onSaveError, debounceMs = 1000, readOnly }: Options) {
  const mapped = useMemo(() => mapGraphToDAG(server, defs, translate), [server, defs, translate]);
  const [canvas, setCanvas] = useState({ graphKey, ...mapped });
  const [saveState, setSaveState] = useState<SaveState>('idle');
  const localRef = useRef<{ nodes: DAGNode[]; edges: DAGEdge[] }>(mapped);
  const dirtyRef = useRef(false);
  const versionRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const graphKeyRef = useRef(graphKey);
  const graphGenerationRef = useRef(0);
  const saveRef = useRef(save);
  const errRef = useRef(onSaveError);
  useEffect(() => {
    saveRef.current = save;
    errRef.current = onSaveError;
  });

  const doSave = useCallback(async () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (!dirtyRef.current) return;
    const version = versionRef.current;
    const savingGraphKey = graphKeyRef.current;
    const savingGeneration = graphGenerationRef.current;
    const snapshot = localRef.current;
    setSaveState('saving');
    try {
      await saveRef.current(snapshot.nodes, snapshot.edges);
      // A response from graph A must never clear graph B's dirty state.
      if (graphKeyRef.current !== savingGraphKey || graphGenerationRef.current !== savingGeneration) return;
      if (versionRef.current === version) {
        dirtyRef.current = false;
        setSaveState('saved');
      } else {
        setSaveState('dirty');
      }
    } catch (e) {
      if (graphKeyRef.current !== savingGraphKey || graphGenerationRef.current !== savingGeneration) return;
      setSaveState('dirty');
      errRef.current?.(e);
    }
  }, []);

  // 服务端数据 / 组件定义变化 → 同步或合并。
  const serverRef = useRef(server);
  useEffect(() => {
    const switched = graphKeyRef.current !== graphKey;
    graphKeyRef.current = graphKey;
    const serverChanged = serverRef.current !== server;
    serverRef.current = server;
    // 仅服务端数据变化（或切换图）且本地无未保存改动时，才以服务端为准；
    // 组件定义变化只补充端口 / 完成度，不重置画布（避免冲掉刚添加、尚未上报的节点）。
    if (switched || (serverChanged && !dirtyRef.current)) {
      if (switched) {
        graphGenerationRef.current += 1;
        if (timerRef.current) clearTimeout(timerRef.current);
        timerRef.current = null;
        versionRef.current += 1;
        dirtyRef.current = false;
        setSaveState('idle');
      }
      localRef.current = mapped;
      setCanvas({ graphKey, ...mapped });
      return;
    }
    // 保留本地结构，仅用最新组件定义补充端口与完成度。
    const merged = localRef.current.nodes.map((n) => (!n.ports?.length && n.codeName && defs[n.codeName] ? decorateNode(n, defs[n.codeName]) : n));
    if (merged.some((n, i) => n !== localRef.current.nodes[i])) {
      localRef.current = { nodes: merged, edges: localRef.current.edges };
      setCanvas({ graphKey, ...localRef.current });
    }
  }, [mapped, graphKey, defs, server]);

  const onGraphChange = useCallback(
    (nodes: DAGNode[], edges: DAGEdge[]) => {
      localRef.current = { nodes, edges };
      if (readOnly) return;
      dirtyRef.current = true;
      versionRef.current += 1;
      setSaveState('dirty');
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => void doSave(), debounceMs);
    },
    [debounceMs, doSave, readOnly],
  );

  /** 立即保存待保存改动（运行前、切图前调用）。 */
  const flush = useCallback(() => doSave(), [doSave]);

  /** 丢弃本地改动，以服务端数据为准（如模板快速配置后）。 */
  const reset = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    dirtyRef.current = false;
    setSaveState('idle');
  }, []);

  /** 局部更新节点（如补充端口），同时更新画布与本地副本。 */
  const patchNodes = useCallback((fn: (nodes: DAGNode[]) => DAGNode[]) => {
    const next = { nodes: fn(localRef.current.nodes), edges: localRef.current.edges };
    localRef.current = next;
    setCanvas({ graphKey: graphKeyRef.current, ...next });
  }, []);

  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    },
    [],
  );

  return {
    // Never mount the next workspace with the previous graph, even for one render.
    initialNodes: canvas.graphKey === graphKey ? canvas.nodes : mapped.nodes,
    initialEdges: canvas.graphKey === graphKey ? canvas.edges : mapped.edges,
    saveState,
    onGraphChange,
    flush,
    reset,
    patchNodes,
    current: () => localRef.current,
    isDirty: () => dirtyRef.current,
  };
}
