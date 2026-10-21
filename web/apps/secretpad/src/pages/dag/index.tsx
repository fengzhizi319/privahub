import { groupProjects } from '../projects/project-list.logic';
/**
 * DAG 训练流编辑页（旧版 modules/main-dag + dag-layout）。
 *
 * - 画布：dag-next DAGNextWorkspace（端口模型、连线校验、撤销栈、缩放感知拖拽）；
 * - 节点 ID：graph/node/max_index → `${graphId}-node-${index}`；
 * - 自动保存：脏标记 + 防抖 graph/update，刷新时合并不覆盖本地改动（use-graph-editor）；
 * - 状态：graph/node/status 轮询（use-node-status），完成提示；日志运行中自动刷新；云日志；
 * - 属性表单：真实数据提供器（attr-data-provider）、组件 i18n、校验、完成度标记；
 * - 运行记录（project/job/list?graphId）+ 回放、模型提交链路、全局配置、模板快速配置、
 *   数据表树、复制训练流、TEE 下载申请、操作引导。
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useSearch } from '@tanstack/react-router';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Modal, ConfirmDialog, Tour, toast } from '@secretpad/design-system';
import type { GraphMetaVO, GraphDetailJava, ProjectDetailJava } from '@secretpad/api-client';
import {
  apiClient,
  batchComponentsJava,
  createGraphJava,
  fullUpdateGraphJava,
  getCloudLogsJava,
  getComponentI18nJava,
  getGraphDetailJava,
  getGraphNodeLogsJava,
  getGraphNodeOutputJava,
  getProjectDatatableColumnsJava,
  getProjectDetailJava,
  listComponentsJava,
  refreshGraphNodeMaxIndexJava,
  startGraphJava,
  stopGraphNodeJava,
  updateGraphNodeJava,
} from '@secretpad/api-client';
import type { BinningData, LinearModelData, ConnectionError, DAGComponentDef, DAGEdge, DAGNode, TemplateType, ValidationError } from '@secretpad/dag-next';
import {
  DAGNextWorkspace,
  TemplateQuickConfig,
  binModificationsSerializer,
  binModificationsUnSerializer,
  modelModificationsSerializer,
  modelModificationsUnSerializer,
  createComponentTranslator,
  maxNodeIndex,
  nodeIdOf,
  normalizeOutput,
  downstreamIds,
  isModificationNode,
  clearNodeDefAttrs,
} from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { useTemplateWizard, TemplateWizard, templateByKey } from '../../features/dag-templates';
import { ModelPackModal } from '../../features/model-pack';
import { ScheduledTaskFromDagModal } from '../../features/scheduled-task-from-dag';
import { useResultActions } from '../../features/job-detail';
import { PAGE_TOUR_KEYS, useDagResultTour, usePageTour } from '../../features/guide-tour';
import type { ComponentDefLike } from './adapters';
import { canSubmitModelFrom, decorateNode, groupComponents, iconFor, splitCodeName, toGraphEdge, toGraphNodeInfo, toStatusOverlay } from './adapters';
import { createAttrDataProvider } from './attr-data-provider';
import { useGraphEditor } from './use-graph-editor';
import { useNodeStatus } from './use-node-status';
import { RecordsDrawer } from './records-drawer';
import { AdvancedConfigModal } from './advanced-config-modal';
import type { AdvancedConfigValue } from './advanced-config-modal';
import { DataTableTree } from './data-table-tree';
import { QUICK_CONFIG_TEMPLATES, quickConfigToTemplateConfigs } from './quick-config';
import { dagEditPermissions } from './edit-permissions';

const EMPTY_DEFS: Record<string, ComponentDefLike | undefined> = {};

export const DAGPage: React.FC = () => {
  const { t, locale } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const platform = usePlatform();
  const search = useSearch({ strict: false }) as { projectId?: string; graphId?: string; dagId?: string };

  const [selectedProjectId, setSelectedProjectId] = useState<string>(search.projectId || '');
  const [selectedGraphId, setSelectedGraphId] = useState<string>(search.graphId || search.dagId || '');
  // Router search can change without remounting this page (links and browser history).
  // useState initializers alone would keep the previous project's canvas selected.
  useEffect(() => {
    if (search.projectId) setSelectedProjectId(search.projectId);
    setSelectedGraphId(search.graphId || search.dagId || '');
  }, [search.projectId, search.graphId, search.dagId]);

  const [error, setError] = useState<string | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newGraphName, setNewGraphName] = useState('');
  const [isRenameModalOpen, setIsRenameModalOpen] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [deleteGraphTarget, setDeleteGraphTarget] = useState<GraphMetaVO | null>(null);
  const [selectedNode, setSelectedNode] = useState<DAGNode | null>(null);
  const [isPackModalOpen, setIsPackModalOpen] = useState(false);
  const [isScheduledModalOpen, setIsScheduledModalOpen] = useState(false);
  const [isRecordsDrawerOpen, setIsRecordsDrawerOpen] = useState(false);
  const [isAdvancedOpen, setIsAdvancedOpen] = useState(false);
  const [quickConfigType, setQuickConfigType] = useState<TemplateType | null>(null);
  const [tourOpen, setTourOpen] = useState(false);
  const [extraDefs, setExtraDefs] = useState<Record<string, ComponentDefLike | undefined>>(EMPTY_DEFS);

  /* ------------------------------------------------------------------ */
  /* 数据                                                                */
  /* ------------------------------------------------------------------ */

  const projectsQuery = useQuery({ queryKey: ['projects'], queryFn: () => apiClient.getProjects() });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);
  const selectedProject = useMemo(() => projects.find((p) => p.projectId === selectedProjectId), [projects, selectedProjectId]);

  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) setSelectedProjectId(projects[0].projectId);
  }, [projects, selectedProjectId]);

  const projectDetailQuery = useQuery({
    queryKey: ['project-detail-java', selectedProjectId],
    queryFn: () => getProjectDetailJava(selectedProjectId),
    enabled: !!selectedProjectId,
  });
  const projectDetail: ProjectDetailJava | null = projectDetailQuery.data ?? null;
  const computeMode = ((projectDetail?.computeMode || selectedProject?.computeMode || 'MPC').toUpperCase() === 'TEE' ? 'TEE' : 'MPC') as 'MPC' | 'TEE';

  const componentsQuery = useQuery({ queryKey: ['components-java'], queryFn: () => listComponentsJava() });
  const i18nQuery = useQuery({ queryKey: ['component-i18n-java'], queryFn: () => getComponentI18nJava() });
  const translator = useMemo(() => createComponentTranslator(i18nQuery.data ?? {}), [i18nQuery.data]);

  const componentGroups = useMemo(
    () => groupComponents(componentsQuery.data ?? [], computeMode, (c) => (locale === 'zh-CN' ? translator.translate(c.domain, undefined, undefined, computeMode) : c.domain)),
    [componentsQuery.data, computeMode, translator, locale],
  );
  // 组件库名称翻译：component/i18n 为 Map<app, Map<"domain/name:version", Map<原文, 译文>>>，按 codeName 查。
  const i18nMap = useMemo(() => {
    if (locale !== 'zh-CN') return {} as Record<string, string>;
    const map: Record<string, string> = { ...translator.flat };
    (componentsQuery.data ?? []).forEach((c) => {
      const cn = `${c.domain}/${c.name}`;
      const tr = translator.translate(c.name, cn, c.version, computeMode);
      if (tr && tr !== c.name) map[cn] = tr;
    });
    return map;
  }, [translator, componentsQuery.data, computeMode, locale]);
  const componentVersion = useCallback(
    (codeName?: string) => {
      const { domain, name } = splitCodeName(codeName);
      return (componentsQuery.data ?? []).find((c) => c.domain === domain && c.name === name);
    },
    [componentsQuery.data],
  );

  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: !!selectedProjectId,
  });
  const graphs = useMemo(() => graphsQuery.data ?? [], [graphsQuery.data]);
  const selectedGraph: GraphMetaVO | null = graphs.find((g) => g.graphId === selectedGraphId) ?? graphs[0] ?? null;
  const graphId = selectedGraph?.graphId || '';
  // Node inspectors and graph-specific dialogs belong to one canvas only.
  useEffect(() => {
    setSelectedNode(null);
    setIsPackModalOpen(false);
    setIsScheduledModalOpen(false);
    setIsRecordsDrawerOpen(false);
    setIsAdvancedOpen(false);
    setIsRenameModalOpen(false);
    setDeleteGraphTarget(null);
    setError(null);
  }, [selectedProjectId, graphId]);

  // 细粒度编辑权限（旧版 ProjectEditService.canEdit）：画布级 / 项目级分开控制，只读时仍可查看日志与结果。
  const perms = dagEditPermissions({
    platformType: platform.platformType,
    ownerId: platform.ownerId,
    project: selectedProject ? { status: projectDetail?.status || selectedProject.status } : null,
    projectsLoaded: projectsQuery.isSuccess && !!selectedProjectId,
    graphOwnerId: selectedGraph?.ownerId,
  });
  const readOnly = !perms.graph;
  const canEditProject = perms.project;
  // 旧版 dag-guide-tour：首次进入画布（已选训练流）自动弹出（localStorage DAGGuideTourOne）。
  const dagTour = usePageTour(PAGE_TOUR_KEYS.dagOne, !!graphId);
  // 旧版第二段 DAG tour（DAGGuideTourTwo）：第一段结束后，首次运行成功时提示查看结果与历史记录。
  const resultTour = useDagResultTour(tourOpen || dagTour.open);

  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', selectedProjectId, graphId],
    queryFn: () => getGraphDetailJava(selectedProjectId, graphId),
    enabled: !!selectedProjectId && !!graphId,
  });
  const graphDetail: GraphDetailJava | null = graphDetailQuery.data ?? null;

  // 组件定义（端口 / 属性 / 完成度）：按图中出现的 codeName 批量拉取。
  const codeNames = useMemo(() => [...new Set((graphDetail?.nodes || []).map((n) => n.codeName).filter(Boolean) as string[])].sort(), [graphDetail]);
  const appOf = useCallback((codeName: string) => componentVersion(codeName)?.app || (computeMode === 'TEE' ? 'trustedflow' : 'secretflow'), [componentVersion, computeMode]);
  const defsQuery = useQuery({
    queryKey: ['component-defs', codeNames.join(','), computeMode],
    queryFn: () =>
      batchComponentsJava(
        codeNames.map((cn) => {
          const { domain, name } = splitCodeName(cn);
          return { app: appOf(cn), domain, name, version: componentVersion(cn)?.version };
        }),
      ),
    enabled: codeNames.length > 0,
  });
  const defs = useMemo<Record<string, ComponentDefLike | undefined>>(
    () => (!defsQuery.data && extraDefs === EMPTY_DEFS ? EMPTY_DEFS : { ...((defsQuery.data as Record<string, ComponentDefLike>) || {}), ...extraDefs }),
    [defsQuery.data, extraDefs],
  );

  const fetchDef = useCallback(
    async (codeName: string): Promise<ComponentDefLike | undefined> => {
      if (defs[codeName]) return defs[codeName];
      const { domain, name } = splitCodeName(codeName);
      try {
        const res = await queryClient.fetchQuery({
          queryKey: ['component-def', codeName, computeMode],
          queryFn: () => batchComponentsJava([{ app: appOf(codeName), domain, name, version: componentVersion(codeName)?.version }]),
          staleTime: 5 * 60 * 1000,
        });
        const def = (res[codeName] || Object.values(res)[0]) as ComponentDefLike | undefined;
        if (def) setExtraDefs((prev) => ({ ...prev, [codeName]: def }));
        return def;
      } catch {
        return undefined;
      }
    },
    [defs, queryClient, computeMode, appOf, componentVersion],
  );

  /* ------------------------------------------------------------------ */
  /* 编辑状态与保存                                                        */
  /* ------------------------------------------------------------------ */

  const graphOptionsRef = useRef<{ maxParallelism?: number; dataSourceConfig?: GraphDetailJava['dataSourceConfig'] }>({});
  useEffect(() => {
    graphOptionsRef.current = { maxParallelism: graphDetail?.maxParallelism, dataSourceConfig: graphDetail?.dataSourceConfig };
  }, [graphDetail]);

  const saveGraph = useCallback(
    async (nodes: DAGNode[], edges: DAGEdge[]) => {
      if (!selectedProjectId || !graphId) return;
      await fullUpdateGraphJava({
        projectId: selectedProjectId,
        graphId,
        nodes: nodes.map((n) => toGraphNodeInfo(n, edges)),
        edges: edges.map(toGraphEdge),
        maxParallelism: graphOptionsRef.current.maxParallelism,
        dataSourceConfig: graphOptionsRef.current.dataSourceConfig,
      });
    },
    [selectedProjectId, graphId],
  );

  const labelOf = useCallback((s: string) => (locale === 'zh-CN' ? translator.translate(s) : s), [translator, locale]);
  const editor = useGraphEditor({
    graphKey: `${selectedProjectId}/${graphId}`,
    server: graphDetail,
    defs,
    translate: labelOf,
    save: saveGraph,
    readOnly,
    onSaveError: (e) => toast.error(t('dagx.autoSaveFailed', { message: e instanceof Error ? e.message : String(e) })),
  });

  const nodeStatus = useNodeStatus({
    projectId: selectedProjectId,
    graphId: graphId || undefined,
    onFinished: (result) => {
      if (result === 'success') {
        toast.success(t('dagx.runSucceeded'));
        resultTour.markRunSucceeded();
      }
      else if (result === 'failed') toast.error(t('dagx.runFailed'));
      void queryClient.invalidateQueries({ queryKey: ['dag-records', selectedProjectId, graphId] });
    },
  });
  const statusOverlay = useMemo(
    () => toStatusOverlay(nodeStatus.status, (id) => editor.current().nodes.find((n) => n.id === id)?.codeName),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [nodeStatus.status],
  );
  // 第二段 tour 高亮的「查看结果」节点：最后一个运行成功的节点（旧版优先二分类评估 / 全表统计）。
  const resultNodeId = useMemo(() => {
    const ok = Object.keys(statusOverlay).filter((id) => statusOverlay[id].status === 'Success');
    return ok[ok.length - 1];
  }, [statusOverlay]);
  const statusOf = useCallback((n: DAGNode) => statusOverlay[n.id]?.status ?? n.status, [statusOverlay]);

  const invalidateGraphs = () => queryClient.invalidateQueries({ queryKey: ['graphs', selectedProjectId] });
  const reloadGraph = () => {
    editor.reset();
    return queryClient.invalidateQueries({ queryKey: ['graph-detail', selectedProjectId, graphId] });
  };

  const templateWizard = useTemplateWizard(selectedProject, (id) => {
    setSelectedGraphId(id);
    // Keep the new canvas bookmarkable; refresh must not reopen the previous graph.
    void navigate({ to: '/dag', search: { projectId: selectedProjectId, graphId: id } });
  });

  /* ------------------------------------------------------------------ */
  /* 图管理                                                              */
  /* ------------------------------------------------------------------ */

  const createGraphMutation = useMutation({
    mutationFn: () => apiClient.createGraph({ projectId: selectedProjectId, name: newGraphName }),
    onSuccess: (id) => {
      setIsCreateModalOpen(false);
      setNewGraphName('');
      void invalidateGraphs();
      if (id) setSelectedGraphId(id);
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteGraphMutation = useMutation({
    mutationFn: (graph: GraphMetaVO) => apiClient.deleteGraph(selectedProjectId, graph.graphId!),
    onSuccess: (_data, graph) => {
      if (graphId === graph.graphId) setSelectedGraphId('');
      setDeleteGraphTarget(null);
      void invalidateGraphs();
    },
    onError: (e) => {
      setDeleteGraphTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const renameGraphMutation = useMutation({
    mutationFn: () => apiClient.renameGraph(selectedProjectId, graphId, renameValue),
    onSuccess: () => {
      setIsRenameModalOpen(false);
      void invalidateGraphs();
      toast.success(t('dag.renameSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  /** 复制训练流（旧版 pipeline copy：名称追加 -copy，节点 ID 与锚点按新图 ID 改写）。 */
  const copyGraphMutation = useMutation({
    mutationFn: async () => {
      await editor.flush();
      const { nodes, edges } = editor.current();
      const newId = await createGraphJava({ projectId: selectedProjectId, name: `${selectedGraph?.name || 'pipeline'}-copy` });
      if (!newId) throw new Error('graph/create returned no graphId');
      const rewrite = (s?: string) => (s && graphId ? s.split(graphId).join(newId) : s);
      const newNodes = nodes.map((n) => ({
        ...n,
        id: rewrite(n.id)!,
        outputs: (n.outputs || []).map((o) => rewrite(o)!),
        ports: (n.ports || []).map((p) => ({ ...p, id: rewrite(p.id)! })),
        status: 'Ready' as const,
      }));
      const newEdges = edges.map((e) => ({ ...e, id: rewrite(e.id)!, source: rewrite(e.source)!, target: rewrite(e.target)!, sourceAnchor: rewrite(e.sourceAnchor), targetAnchor: rewrite(e.targetAnchor) }));
      await fullUpdateGraphJava({ projectId: selectedProjectId, graphId: newId, nodes: newNodes.map((n) => toGraphNodeInfo(n, newEdges)), edges: newEdges.map(toGraphEdge) });
      return newId;
    },
    onSuccess: (newId) => {
      toast.success(t('dagx.copySuccess'));
      void invalidateGraphs();
      setSelectedGraphId(newId);
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const handleCreateGraph = (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProjectId || !newGraphName.trim()) return;
    setError(null);
    createGraphMutation.mutate();
  };

  /* ------------------------------------------------------------------ */
  /* 运行                                                                */
  /* ------------------------------------------------------------------ */

  const startNodes = async (ids: string[], breakpoint = false) => {
    if (!selectedProjectId || !graphId) return;
    try {
      await editor.flush();
      const jobId = await startGraphJava(selectedProjectId, graphId, ids, breakpoint);
      nodeStatus.markRunStarted();
      toast.success(breakpoint ? t('dagx.continueStarted') : t('dag.started', { jobId: jobId || ids.length }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const stopNodes = async (graphNodeId?: string) => {
    if (!selectedProjectId || !graphId) return;
    try {
      await stopGraphNodeJava(selectedProjectId, graphId, graphNodeId);
      nodeStatus.refresh();
      toast.success(t('dagx.nodeStopped'));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  /* ------------------------------------------------------------------ */
  /* 节点                                                                */
  /* ------------------------------------------------------------------ */

  /** 分配节点 ID：graph/node/max_index（currentIndex 为本地最大序号），默认从 32 起。 */
  const allocateNodeIds = useCallback(
    async (count: number) => {
      const local = maxNodeIndex(editor.current().nodes.map((n) => n.id));
      let max = local;
      try {
        max = Math.max(local, await refreshGraphNodeMaxIndexJava(selectedProjectId, graphId, local || undefined));
      } catch {
        max = Math.max(local, 32);
      }
      return Array.from({ length: count }, (_, i) => nodeIdOf(graphId, max + i + 1));
    },
    [editor, selectedProjectId, graphId],
  );

  const handleAddNode = async (component: DAGComponentDef & { datatableId?: string; datatableName?: string }): Promise<DAGNode> => {
    const codeName = `${component.domain}/${component.name}`;
    const [id] = await allocateNodeIds(1);
    const def = await fetchDef(codeName);
    const label = component.datatableName || (locale === 'zh-CN' ? translator.translate(component.name, codeName, component.version, computeMode) : component.name);
    const nodeDef: Record<string, unknown> = { domain: component.domain, name: component.name, version: component.version || componentVersion(codeName)?.version };
    if (component.datatableId) {
      nodeDef.attrPaths = ['datatable_selected'];
      nodeDef.attrs = [{ s: component.datatableId, is_na: false }];
    }
    const base: DAGNode = {
      id,
      name: label,
      category: component.domain,
      icon: component.icon || iconFor(component.domain),
      status: 'Ready',
      x: 200 + Math.random() * 120,
      y: 120 + Math.random() * 80,
      codeName,
      nodeDef,
      inputs: [],
      outputs: [],
    };
    return decorateNode(base, def);
  };

  const handleGetComponentDef = async (node: DAGNode) => {
    if (!node.codeName) return null;
    const def = await fetchDef(node.codeName);
    if (!def) return null;
    return {
      desc: def.desc,
      version: def.version,
      domain: def.domain,
      inputs: (def.inputs || []) as never,
      outputs: (def.outputs || []) as never,
      attrs: def.attrs,
    };
  };

  const persisted = useCallback((id: string) => !!graphDetail?.nodes.some((n) => n.graphNodeId === id), [graphDetail]);

  const handleNodeConfigChange = async (node: DAGNode) => {
    if (!selectedProjectId || !graphId) return;
    try {
      // 未保存的新节点先整图保存，避免 graph/node/update 找不到节点。
      if (!persisted(node.id) || editor.isDirty()) await editor.flush();
      else await updateGraphNodeJava({ projectId: selectedProjectId, graphId, node: toGraphNodeInfo(node, editor.current().edges) });
      toast.success(t('dagx.configSaved'));
      // 旧版 graph-service.saveNodeConfig：上游配置变化后，下游的分箱 / 模型参数修改节点
      // 必须清空 nodeDef.attrs，才能重新读取最新的上游输出。
      const { nodes, edges } = editor.current();
      const downstream = new Set(downstreamIds(node.id, edges).filter((id) => id !== node.id));
      const stale = nodes.filter((n) => downstream.has(n.id) && isModificationNode(n.codeName) && (n.nodeDef?.attrs || n.nodeDef?.attrPaths));
      if (stale.length) {
        const ids = new Set(stale.map((n) => n.id));
        editor.patchNodes((list) => list.map((n) => (ids.has(n.id) ? { ...n, nodeDef: clearNodeDefAttrs(n.nodeDef), configFinished: false } : n)));
        editor.onGraphChange(editor.current().nodes, editor.current().edges);
        await editor.flush();
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleNodeOutput = async (node: DAGNode, outputId?: string) => {
    const out = outputId || node.resultOutputs?.[0]?.id || node.outputs?.[0];
    if (!selectedProjectId || !graphId || !out) return null;
    try {
      return await getGraphNodeOutputJava({ projectId: selectedProjectId, graphId, graphNodeId: node.id, outputId: out });
    } catch {
      return null;
    }
  };

  /**
   * 分箱 / 线性模型参数修改（旧版 ParamsModificationsRenderView）：
   * - latest：nodeDef.attrs[0].s（已保存配置）；
   * - upstream：本节点第 2 个输出（outputs[1]）的 tabs[0]；
   * - 未指定时：已配置取 latest，否则取 upstream。
   */
  const loadModification = async <T,>(node: DAGNode, source: 'upstream' | 'latest' | undefined, unser: (v: never) => T | undefined): Promise<T | null> => {
    const saved = (node.nodeDef?.attrs as Array<{ s?: string }> | undefined)?.[0]?.s;
    if (saved && source !== 'upstream') {
      try {
        return unser(JSON.parse(saved) as never) ?? null;
      } catch {
        /* fallthrough */
      }
    }
    if (source === 'latest') return null;
    const outputId = node.outputs?.[1] ?? node.outputs?.[0];
    if (!outputId) return null;
    const raw = await handleNodeOutput(node, outputId);
    let list: unknown = normalizeOutput(raw)?.raw.tabs;
    if (typeof list === 'string') {
      try {
        list = JSON.parse(list);
      } catch {
        return null;
      }
    }
    const first = Array.isArray(list) ? list[0] : undefined;
    if (!first) return null;
    try {
      return unser((typeof first === 'string' ? JSON.parse(first) : first) as never) ?? null;
    } catch {
      return null;
    }
  };

  /** 修改类组件的属性路径：组件定义中对应 custom_protobuf_cls 的属性，回退到已保存路径。 */
  const modificationAttrPath = async (node: DAGNode, cls: string, fallback: string) => {
    const def = node.codeName ? await fetchDef(node.codeName) : undefined;
    const attr = (def?.attrs || []).find((a) => (a.custom_protobuf_cls ?? a.customProtobufCls) === cls);
    if (attr) return [...((attr.prefixes as string[] | undefined) || []), String(attr.name)].join('/');
    return (node.nodeDef?.attrPaths as string[] | undefined)?.[0] || fallback;
  };

  const saveModification = async (node: DAGNode, attrPath: string, serialized: unknown, run = false) => {
    const nodeDef = { ...(node.nodeDef || {}), attrPaths: [attrPath], attrs: [{ s: JSON.stringify(serialized) }] };
    editor.patchNodes((nodes) => nodes.map((n) => (n.id === node.id ? { ...n, nodeDef, configFinished: true } : n)));
    editor.onGraphChange(editor.current().nodes, editor.current().edges);
    await editor.flush();
    if (run) await startNodes([node.id]);
    else toast.success(t('dagx.configSaved'));
  };

  const handleSaveBinning = async (node: DAGNode, data: BinningData, run = false) =>
    saveModification(node, await modificationAttrPath(node, 'Binning_modifications', 'bin_modifications'), binModificationsSerializer(data as never), run);

  const handleSaveModelParams = async (node: DAGNode, data: LinearModelData) =>
    saveModification(node, await modificationAttrPath(node, 'linear_model_pb2', 'model_modifications'), modelModificationsSerializer(data));

  /** 属性数据提供器：按节点构造（列来自上游）。 */
  const attrDataProviderFor = useCallback(
    (node: DAGNode, nodes: DAGNode[], edges: DAGEdge[]) =>
      createAttrDataProvider(node, nodes, edges, {
        projectId: selectedProjectId,
        graphId,
        project: projectDetail,
        fetchOutput: (graphNodeId, outputId) => getGraphNodeOutputJava({ projectId: selectedProjectId, graphId, graphNodeId, outputId }).catch(() => null),
        fetchTableColumns: ({ nodeId, datatableId }) => getProjectDatatableColumnsJava({ projectId: selectedProjectId, nodeId, datatableId }).catch(() => []),
        fetchModels: async () => {
          try {
            const models = await apiClient.getModels(selectedProjectId);
            return models.map((m) => ({ id: m.modelId || '', name: m.modelName || m.modelId || '' }));
          } catch {
            return [];
          }
        },
      }),
    [selectedProjectId, graphId, projectDetail],
  );

  const attrTranslate = useCallback(
    (node: DAGNode) => {
      if (locale !== 'zh-CN') return undefined;
      const version = (node.nodeDef?.version as string | undefined) ?? componentVersion(node.codeName)?.version;
      return (text: string) => translator.translate(text, node.codeName, version, computeMode);
    },
    [translator, locale, componentVersion, computeMode],
  );

  const { downloadMode, actions: resultActions, modal: teeModal } = useResultActions({ projectId: selectedProjectId, graphId, computeMode });

  const handleConnectionRejected = (reason: ConnectionError) => toast.warning(t(`dagx.connect.${reason}`));
  const handleValidationFailed = (_node: DAGNode, errors: ValidationError[]) => toast.error(t('dagx.configInvalid', { count: errors.length }));

  /* ------------------------------------------------------------------ */
  /* 全局配置 / 快速配置                                                    */
  /* ------------------------------------------------------------------ */

  const advancedInitial: AdvancedConfigValue = useMemo(
    () => ({ maxParallelism: graphDetail?.maxParallelism, dataSourceConfig: graphDetail?.dataSourceConfig || [] }),
    [graphDetail],
  );
  const advancedMutation = useMutation({
    mutationFn: async (value: AdvancedConfigValue) => {
      graphOptionsRef.current = { maxParallelism: value.maxParallelism, dataSourceConfig: value.dataSourceConfig };
      const { nodes, edges } = editor.current();
      await saveGraph(nodes, edges);
    },
    onSuccess: () => {
      setIsAdvancedOpen(false);
      toast.success(t('dag.saveSuccess'));
      void queryClient.invalidateQueries({ queryKey: ['graph-detail', selectedProjectId, graphId] });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const quickTables = useMemo(
    () =>
      (projectDetail?.nodes || []).flatMap((n) =>
        n.datatables.map((tb) => ({ datatableId: tb.datatableId, datatableName: tb.datatableName || tb.datatableId, nodeId: n.nodeId, nodeName: n.nodeName || n.nodeId, isPartitionTable: tb.isPartitionTable })),
      ),
    [projectDetail],
  );
  const quickColumns = useCallback(
    async (table: { nodeId: string; datatableId: string }) =>
      (await getProjectDatatableColumnsJava({ projectId: selectedProjectId, nodeId: table.nodeId, datatableId: table.datatableId }).catch(() => [])).map((c) => c.colName),
    [selectedProjectId],
  );
  // 旧版 quick-config-drawer：切换训练流时关闭抽屉。
  useEffect(() => setQuickConfigType(null), [graphId]);

  const saveQuickConfig = async (values: Record<string, unknown>) => {
    if (!quickConfigType || !graphId) return;
    const entry = QUICK_CONFIG_TEMPLATES.find((q) => q.type === quickConfigType);
    const template = entry ? templateByKey(entry.templateKey) : undefined;
    if (!template) return;
    try {
      const { nodes, edges } = template.build({ graphId, configs: quickConfigToTemplateConfigs(quickConfigType, values, projectDetail) });
      await fullUpdateGraphJava({ projectId: selectedProjectId, graphId, nodes: nodes as never, edges: edges as never });
      toast.success(t('dagx.quickConfigSaved'));
      setQuickConfigType(null);
      await reloadGraph();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  /* ------------------------------------------------------------------ */
  /* 渲染                                                                */
  /* ------------------------------------------------------------------ */

  const dagLabels = useMemo(
    () => ({
      operatorLibrary: t('dagx.components'),
      openPalette: t('dagx.ui.openPalette'),
      closePalette: t('dagx.ui.closePalette'),
      closeInspector: t('dagx.ui.closeInspector'),
      components: t('dagx.components'),
      noOperators: t('common.empty'),
      connect: t('dag.connect'),
      clickTarget: t('dag.clickTarget'),
      connectionHint: t('dag.clickTarget'),
      parameters: t('dag.parameters'),
      logs: t('dag.logs'),
      output: t('dag.output'),
      save: t('dag.save'),
      run: t('dag.run'),
      operatorName: t('dag.nameLabel'),
      applyConfig: t('dag.apply'),
      noLogs: t('dag.noLogs'),
      noOutput: t('dag.noOutput'),
      refresh: t('dag.refresh'),
      nodeOutput: t('dag.output'),
      deleteNode: t('common.delete'),
      emptyCanvas: t('dag.emptyCanvas'),
      advancedConfig: t('dag.advancedConfig'),
      configIssues: t('display.issues'), validateConfig: t('display.validate'), configValid: t('display.valid'),
      noAttrs: t('dag.noAttrs'),
      optional: t('dag.optionalAttr'),
      required: t('dag.requiredAttr'),
      none: t('dag.noneSelection'),
      listPlaceholder: t('dag.listPlaceholder'),
      interpreterTitle: t('dag.interpreterTitle'),
      interpreterDesc: t('dag.interpreterDesc'),
      interpreterInputs: t('dag.interpreterInputs'),
      interpreterOutputs: t('dag.interpreterOutputs'),
      interpreterAttrs: t('dag.interpreterAttrs'),
      interpreterLoading: t('dag.interpreterLoading'),
      interpreterNoDef: t('dag.interpreterNoDef'),
      interpreterTypes: t('dag.interpreterTypes'),
      interpretComponent: t('dag.interpretComponent'),
      logSearchPlaceholder: t('dag.logSearchPlaceholder'),
      logCopy: t('dag.logCopy'),
      logCopied: t('dag.logCopied'),
      logWrap: t('dag.logWrap'),
      logAutoScroll: t('dag.logAutoScroll'),
      logLines: t('dag.logLines'),
      runSingle: t('dag.runSingle'),
      runDown: t('dag.runDown'),
      runUp: t('dag.runUp'),
      stop: t('dag.stop'),
      stopNode: t('dagx.stopNode'),
      continueRun: t('dagx.continueRun'),
      tidyLayout: t('dag.tidyLayout'),
      selectNodeFirst: t('dag.selectNodeFirst'),
      cloudLogs: t('dagx.cloudLogs'),
      cloudLogNotConfigured: t('cloudLogs.notConfigured'),
      cloudLogHelp: t('cloudLogs.helpDoc'),
      unfinished: t('dagx.unfinished'),
      selectOutput: t('dagx.selectOutput'),
      autoRefresh: t('dagx.autoRefresh'),
      unsaved: t('dagx.unsaved'),
      saving: t('dagx.saving'),
      saved: t('dagx.saved'),
      copy: t('dagx.copy'),
      paste: t('dagx.paste'),
      duplicate: t('dagx.duplicate'),
      delete: t('dagx.delete'),
      ready: t('dagx.status.ready'),
      pending: t('dagx.status.pending'),
      running: t('dagx.status.running'),
      success: t('dagx.status.success'),
      failed: t('dagx.status.failed'),
      stopped: t('dagx.status.stopped'),
      nodeIdentifier: t('dagx.ui.nodeIdentifier'),
      codeName: t('dagx.ui.codeName'),
      executionStatus: t('dagx.ui.executionStatus'),
      position: t('dagx.ui.position'),
      frontendConfig: t('dagx.ui.frontendConfig'),
      nodeDef: t('dagx.ui.nodeDef'),
      status: t('dagx.ui.status'),
      fullscreen: t('dagx.ui.fullscreen'),
      exitFullscreen: t('dagx.ui.exitFullscreen'),
      exportJson: t('dagx.ui.exportJson'),
      importJson: t('dagx.ui.importJson'),
      search: t('dagx.ui.search'),
      searchPlaceholder: t('dagx.ui.searchPlaceholder'),
      paletteSearch: t('dagx.ui.paletteSearch'),
      undo: t('dagx.ui.undo'),
      redo: t('dagx.ui.redo'),
      snapOn: t('dagx.ui.snapOn'),
      snapOff: t('dagx.ui.snapOff'),
      binningTab: t('dagx.ui.binningTab'),
      records: t('dagx.records'),
    }),
    [t],
  );

  const hasActive = Object.values(statusOverlay).some((s) => s.status === 'Running' || s.status === 'Pending');
  const canSubmitModel = canSubmitModelFrom(selectedNode, editor.current().nodes, editor.current().edges, statusOf);
  const loading = graphsQuery.isLoading || graphDetailQuery.isLoading;
  const queryError = graphsQuery.error?.message || graphDetailQuery.error?.message || projectsQuery.error?.message || componentsQuery.error?.message || null;

  const toolbarExtra = selectedGraph ? (
    <>
      <Button size="sm" variant="ghost" onClick={() => setIsAdvancedOpen(true)} title={t('dagx.advancedConfig')}>
        ⚙️
      </Button>
      {!readOnly && (
        <select
          aria-label={t('dagx.quickConfig')}
          value=""
          onChange={(e) => e.target.value && setQuickConfigType(e.target.value as TemplateType)}
          className="px-1 py-0.5 rounded bg-gray-800 border border-gray-700 text-gray-300 text-[10px]"
        >
          <option value="">{t('dagx.quickConfig')}</option>
          {QUICK_CONFIG_TEMPLATES.filter((q) => (computeMode === 'TEE' ? ['PSI_TEE', 'TEE'].includes(q.type) || !['PSI_TEE', 'TEE'].includes(q.type) : !['PSI_TEE', 'TEE'].includes(q.type))).map((q) => (
            <option key={q.type} value={q.type}>
              {q.type}
            </option>
          ))}
        </select>
      )}
      <span data-tour="dag-records">
        <Button size="sm" variant="ghost" onClick={() => setIsRecordsDrawerOpen(true)} title={t('dagx.records')}>
          📜
        </Button>
      </span>
      <Button size="sm" variant="ghost" onClick={() => setTourOpen(true)} title={t('dagx.tour.start')}>
        ❔
      </Button>
    </>
  ) : null;

  return (
    <div className="space-y-4 h-full flex flex-col">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dag.title')}</h2>
          <p className="text-xs text-gray-500">
            {t('dag.subtitle')} · {t('dagx.computeMode')}: {computeMode}
          </p>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          <select
            aria-label="project"
            value={selectedProjectId}
            onChange={(e) => {
              void editor.flush();
              setSelectedProjectId(e.target.value);
              setSelectedGraphId('');
            }}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
          >
            {groupProjects(projects, 'mode').map(group => <optgroup key={group.key} label={group.key}>
              {group.projects.map(p => <option key={p.projectId} value={p.projectId}>{p.projectName}</option>)}
            </optgroup>)}
          </select>
          <select
            aria-label="graph"
            value={graphId}
            onChange={(e) => {
              void editor.flush();
              setSelectedGraphId(e.target.value);
            }}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
          >
            {graphs.map((g) => (
              <option key={g.graphId} value={g.graphId || ''}>
                {g.name}
              </option>
            ))}
          </select>
          {canEditProject && (
            <>
              <Button variant="primary" size="sm" onClick={() => setIsCreateModalOpen(true)}>
                {t('dag.create')}
              </Button>
              <Button variant="outline" size="sm" onClick={() => templateWizard.open()} disabled={!selectedProject || (selectedProject?.nodes || []).length === 0}>
                {t('dag.template')}
              </Button>
            </>
          )}
          {!readOnly && selectedGraph && (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setRenameValue(selectedGraph.name || '');
                  setIsRenameModalOpen(true);
                }}
              >
                {t('dag.rename')}
              </Button>
              <Button variant="outline" size="sm" loading={copyGraphMutation.isPending} onClick={() => copyGraphMutation.mutate()}>
                {t('dagx.copyPipeline')}
              </Button>
            </>
          )}
          {canEditProject && selectedGraph && (
            <Button variant="outline" size="sm" onClick={() => setIsScheduledModalOpen(true)}>
              {t('dag.createPeriodicTask')}
            </Button>
          )}
          {canEditProject && selectedNode && canSubmitModel && (
            <Button variant="outline" size="sm" onClick={() => setIsPackModalOpen(true)}>
              {t('dagx.submitModel')}
            </Button>
          )}
          {!readOnly && hasActive && (
            <Button variant="outline" size="sm" onClick={() => void stopNodes()}>
              ⏹ {t('dag.stop')}
            </Button>
          )}
          {!readOnly && selectedGraph && (
            <Button variant="danger" size="sm" onClick={() => setDeleteGraphTarget(selectedGraph)}>
              {t('common.delete')}
            </Button>
          )}
        </div>
      </div>

      {perms.reason && selectedGraph && (
        <div role="status" className="text-xs text-amber-600 bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-900 rounded-lg px-4 py-2">
          {t(`dagx.readOnlyReason.${perms.reason}`)} · {t('dagx.readOnly')}
        </div>
      )}

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <div className="flex-1 min-h-[420px] lg:min-h-[560px]">
        {selectedGraph ? (
          <DAGNextWorkspace
            // 按训练流重新挂载：撤销/重做栈、选中与视口不跨图保留（否则切图后 Ctrl+Z 会把上一张图写进当前图）。
            key={`${selectedProjectId}/${graphId}`}
            title={selectedGraph.name}
            readOnly={readOnly}
            initialNodes={editor.initialNodes}
            initialEdges={editor.initialEdges}
            componentGroups={componentGroups}
            i18nMap={i18nMap}
            computeMode={computeMode}
            nodeStatuses={statusOverlay}
            saveState={editor.saveState}
            toolbarExtra={toolbarExtra}
            sidebarTabs={[{ key: 'tables', label: t('dagx.dataTables'), content: <DataTableTree projectId={selectedProjectId} project={projectDetail} readOnly={readOnly} manageNodeId={canEditProject && platform.isP2p ? platform.ownerId : undefined} /> }]}
            centerOverlay={
              // 旧版 quick-config-drawer：挂在 DAG 画布区域内（getContainer=.center），不遮挡全局 Header。
              <TemplateQuickConfig
                visible={!!quickConfigType && !readOnly}
                templateType={quickConfigType ?? 'PSI'}
                tables={quickTables}
                fetchColumns={quickColumns}
                labels={{ title: t('dagx.quickConfig'), save: t('common.save'), cancel: t('common.cancel') }}
                onSave={saveQuickConfig}
                onClose={() => setQuickConfigType(null)}
              />
            }
            onGraphChange={editor.onGraphChange}
            onSaveGraph={(nodes, edges) => {
              editor.onGraphChange(nodes, edges);
              return editor.flush();
            }}
            onRunGraph={(nodes) => startNodes(nodes.map((n) => n.id))}
            onRunSingle={(node) => startNodes([node.id])}
            onRunDown={(_n, list) => startNodes(list.map((n) => n.id))}
            onRunUp={(_n, list) => startNodes(list.map((n) => n.id))}
            onStopGraph={hasActive ? () => stopNodes() : undefined}
            onStopNode={(node) => stopNodes(node.id)}
            onContinueRun={(node) => startNodes([node.id], true)}
            onNodeConfigChange={handleNodeConfigChange}
            onNodeLogs={async (node) => {
              const res = await getGraphNodeLogsJava(selectedProjectId, graphId, node.id);
              return { status: res.status, logs: res.logs };
            }}
            onCloudLogs={async (node) => {
              const res = await getCloudLogsJava({ projectId: selectedProjectId, graphNodeId: node.id });
              return { status: res.status, logs: res.logs, config: res.config };
            }}
            onNodeOutput={handleNodeOutput}
            outputViewProps={() => ({ downloadMode, actions: resultActions })}
            onNodeBinningData={(node, source) => loadModification<BinningData>(node, source, binModificationsUnSerializer as never)}
            onSaveBinningData={(node, data) => handleSaveBinning(node, data)}
            onBinningMerge={(node, data) => handleSaveBinning(node, data, true)}
            onNodeModelParams={(node, source) => loadModification<LinearModelData>(node, source, modelModificationsUnSerializer as never)}
            onSaveModelParams={handleSaveModelParams}
            onNodeSelect={setSelectedNode}
            onAddNode={handleAddNode}
            allocateNodeIds={allocateNodeIds}
            onConnect={(source, target, anchors) => (anchors ? { id: `${anchors.sourceAnchor}__${anchors.targetAnchor}`, source, target, ...anchors } : null)}
            onConnectionRejected={handleConnectionRejected}
            onValidationFailed={handleValidationFailed}
            onGetComponentDef={handleGetComponentDef}
            attrDataProviderFor={attrDataProviderFor}
            attrTranslate={attrTranslate}
            loading={loading}
            labels={dagLabels}
          />
        ) : (
          <Card className="h-full flex items-center justify-center">
            <div className="text-center text-xs text-gray-400">
              <div className="mb-2">{t('dag.noGraph')}</div>
              {canEditProject && (
                <Button variant="primary" size="sm" onClick={() => setIsCreateModalOpen(true)}>
                  {t('dag.create')}
                </Button>
              )}
            </div>
          </Card>
        )}
      </div>

      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => {
          setIsCreateModalOpen(false);
          setNewGraphName('');
        }}
        title={t('dag.createTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsCreateModalOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" onClick={handleCreateGraph} loading={createGraphMutation.isPending}>
              {t('common.create')}
            </Button>
          </>
        }
      >
        <form onSubmit={handleCreateGraph} className="space-y-4 text-xs">
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dag.nameLabel')}</label>
          <input
            type="text"
            value={newGraphName}
            onChange={(e) => setNewGraphName(e.target.value)}
            placeholder={t('dag.namePlaceholder')}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
            required
          />
        </form>
      </Modal>

      <Modal
        isOpen={isRenameModalOpen}
        onClose={() => setIsRenameModalOpen(false)}
        title={t('dag.renameTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsRenameModalOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" onClick={() => renameGraphMutation.mutate()} loading={renameGraphMutation.isPending} disabled={!renameValue.trim()}>
              {t('common.save')}
            </Button>
          </>
        }
      >
        <div className="text-xs">
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dag.nameLabel')}</label>
          <input
            type="text"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
          />
        </div>
      </Modal>

      <ConfirmDialog
        isOpen={!!deleteGraphTarget}
        title={t('common.delete')}
        message={t('dag.deleteConfirm')}
        danger
        loading={deleteGraphMutation.isPending}
        confirmText={t('common.delete')}
        cancelText={t('common.cancel')}
        onConfirm={() => deleteGraphTarget && deleteGraphMutation.mutate(deleteGraphTarget)}
        onCancel={() => setDeleteGraphTarget(null)}
      />

      <TemplateWizard project={selectedProject} {...templateWizard} />

      {selectedNode && selectedGraph && isPackModalOpen && (
        <ModelPackModal
          isOpen={isPackModalOpen}
          onClose={() => setIsPackModalOpen(false)}
          projectId={selectedProjectId}
          graphId={graphId}
          trainNode={selectedNode}
          nodes={editor.current().nodes}
          edges={editor.current().edges}
          statusOf={statusOf}
          onPacked={() => setIsPackModalOpen(false)}
        />
      )}

      {selectedGraph && (
        <ScheduledTaskFromDagModal
          isOpen={isScheduledModalOpen}
          onClose={() => setIsScheduledModalOpen(false)}
          projectId={selectedProjectId}
          graphId={graphId}
          graphName={selectedGraph.name}
          nodes={editor.current().nodes}
        />
      )}

      {selectedGraph && (
        <RecordsDrawer
          open={isRecordsDrawerOpen}
          onClose={() => setIsRecordsDrawerOpen(false)}
          projectId={selectedProjectId}
          graphId={graphId}
          computeMode={computeMode}
          readOnly={readOnly}
        />
      )}

      <AdvancedConfigModal
        open={isAdvancedOpen}
        projectId={selectedProjectId}
        initial={advancedInitial}
        readOnly={!canEditProject}
        saving={advancedMutation.isPending}
        onClose={() => setIsAdvancedOpen(false)}
        onSave={(v) => advancedMutation.mutate(v)}
      />

      {teeModal}

      <Tour
        open={tourOpen || dagTour.open}
        onClose={() => {
          setTourOpen(false);
          dagTour.close();
        }}
        labels={{ next: t('common.next'), prev: t('common.previous'), finish: t('common.close'), skip: t('common.cancel') }}
        steps={[
          { target: '[data-tour="dag-palette"]', title: t('dagx.tour.palette.title'), content: t('dagx.tour.palette.desc') },
          { target: '[data-tour="dag-canvas"]', title: t('dagx.tour.canvas.title'), content: t('dagx.tour.canvas.desc') },
          { target: '[data-tour="dag-toolbar"]', title: t('dagx.tour.toolbar.title'), content: t('dagx.tour.toolbar.desc') },
          { target: '[data-tour="dag-run"]', title: t('dagx.tour.run.title'), content: t('dagx.tour.run.desc') },
          { target: '[data-tour="dag-records"]', title: t('dagx.tour.records.title'), content: t('dagx.tour.records.desc') },
        ]}
      />

      <Tour
        open={resultTour.open}
        onClose={resultTour.close}
        labels={{ next: t('common.next'), prev: t('common.previous'), finish: t('common.close'), skip: t('common.cancel') }}
        steps={[
          {
            target: resultNodeId ? `[data-node-id="${resultNodeId}"]` : '[data-tour="dag-canvas"]',
            title: t('dagx.tour.viewResult.title'),
            content: t('dagx.tour.viewResult.desc'),
          },
          { target: '[data-tour="dag-records"]', title: t('dagx.tour.history.title'), content: t('dagx.tour.history.desc') },
        ]}
      />
    </div>
  );
};
