import React, { useEffect, useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Button, Badge, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type {
  GraphMetaVO,
  GraphDetailVO,
  GraphNodeInfo,
  GraphEdge,
  ComponentSummaryDef,
} from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import type { DAGNode, DAGEdge, DAGComponentDef } from '@secretpad/dag-next';
import { DAGNextWorkspace } from '@secretpad/dag-next';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';
import { useTemplateWizard, TemplateWizard } from '../../features/dag-templates';
import { ModelPackModal } from '../../features/model-pack';
import { ScheduledTaskFromDagModal } from '../../features/scheduled-task-from-dag';

function normalizeCodeName(codeName?: string): { domain: string; name: string } {
  if (!codeName) return { domain: 'unknown', name: 'unknown' };
  const parts = codeName.split('/');
  if (parts.length >= 2) {
    return { domain: parts.slice(0, -1).join('/'), name: parts[parts.length - 1] };
  }
  return { domain: codeName, name: codeName };
}

function mapBackendStatus(status?: string): DAGNode['status'] {
  switch (status) {
    case 'RUNNING':
      return 'Running';
    case 'SUCCEED':
      return 'Success';
    case 'FAILED':
      return 'Failed';
    case 'STOPPED':
      return 'Stopped';
    default:
      return 'Ready';
  }
}

function mapGraphToDAG(graph?: GraphDetailVO): { nodes: DAGNode[]; edges: DAGEdge[] } {
  if (!graph) return { nodes: [], edges: [] };
  const nodes: DAGNode[] = (graph.nodes || []).map((n) => {
    const { domain } = normalizeCodeName(n.codeName);
    return {
      id: n.graphNodeId || String(Math.random()),
      name: n.label || n.codeName || 'Node',
      category: domain || 'Unknown',
      icon: domain === 'read_data' ? '📥' : domain.startsWith('ml') ? '🤖' : '⚙️',
      status: mapBackendStatus(n.status),
      x: n.x ?? 100,
      y: n.y ?? 100,
      progress: n.progress,
      codeName: n.codeName,
      nodeDef: (n as any).nodeDef,
      inputs: (n as any).inputs,
      outputs: (n as any).outputs,
    };
  });
  const edges: DAGEdge[] = (graph.edges || [])
    .map((e) => ({
      id: e.edgeId || `${e.source}-${e.target}-${Math.random().toString(36).slice(2, 6)}`,
      source: e.source || '',
      target: e.target || '',
      sourceAnchor: e.sourceAnchor,
      targetAnchor: e.targetAnchor,
    }))
    .filter((e) => e.source && e.target);
  return { nodes, edges };
}

function mapDAGNodeToGraphNode(node: DAGNode): GraphNodeInfo {
  return {
    graphNodeId: node.id,
    codeName: node.codeName,
    label: node.name,
    x: Math.round(node.x),
    y: Math.round(node.y),
    inputs: node.inputs,
    outputs: node.outputs,
    nodeDef: node.nodeDef,
  };
}

function mapDAGEdgeToGraphEdge(edge: DAGEdge): GraphEdge {
  return {
    edgeId: edge.id,
    source: edge.source,
    target: edge.target,
    sourceAnchor: edge.sourceAnchor,
    targetAnchor: edge.targetAnchor,
  };
}

export const DAGPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [selectedProjectId, setSelectedProjectId] = useState<string>('');
  const [selectedGraphId, setSelectedGraphId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newGraphName, setNewGraphName] = useState('');
  const [isRenameModalOpen, setIsRenameModalOpen] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [deleteGraphTarget, setDeleteGraphTarget] = useState<GraphMetaVO | null>(null);
  const [selectedNode, setSelectedNode] = useState<DAGNode | null>(null);
  const [isPackModalOpen, setIsPackModalOpen] = useState(false);
  const [isScheduledModalOpen, setIsScheduledModalOpen] = useState(false);

  const dagLabels = useMemo(
    () => ({
      operatorLibrary: t('dag.components'),
      noOperators: t('common.empty'),
      connect: t('dag.connect'),
      clickTarget: t('dag.clickTarget'),
      connectionHint: t('dag.clickTarget'),
      parameters: t('dag.parameters'),
      logs: t('dag.logs'),
      output: t('dag.output'),
      save: t('dag.save'),
      run: t('dag.run'),
      nodeIdentifier: 'Node ID',
      operatorName: t('dag.nameLabel'),
      codeName: 'Code Name',
      executionStatus: t('common.statusActive'),
      position: 'Position',
      frontendConfig: t('dag.config'),
      nodeDef: 'NodeDef',
      applyConfig: t('dag.apply'),
      status: t('common.statusActive'),
      noLogs: t('dag.noLogs'),
      noOutput: t('dag.noOutput'),
      refresh: t('common.search'),
      nodeOutput: t('dag.output'),
      deleteNode: t('common.delete'),
      emptyCanvas: t('dag.emptyCanvas'),
      // 高级配置抽屉文案（传递给 dag-next 的 AttributeForm）
      advancedConfig: t('dag.advancedConfig'),
      noAttrs: t('dag.noAttrs'),
      optional: t('dag.optionalAttr'),
      required: t('dag.requiredAttr'),
      none: t('dag.noneSelection'),
      listPlaceholder: t('dag.listPlaceholder'),
      // 组件解释器文案（传递给 dag-next 的 ComponentInterpreter）
      interpreterTitle: t('dag.interpreterTitle'),
      interpreterDesc: t('dag.interpreterDesc'),
      interpreterInputs: t('dag.interpreterInputs'),
      interpreterOutputs: t('dag.interpreterOutputs'),
      interpreterAttrs: t('dag.interpreterAttrs'),
      interpreterLoading: t('dag.interpreterLoading'),
      interpreterNoDef: t('dag.interpreterNoDef'),
      interpreterTypes: t('dag.interpreterTypes'),
      interpretComponent: t('dag.interpretComponent'),
      // 日志查看器文案（传递给 dag-next 的 LogViewer）
      logSearchPlaceholder: t('dag.logSearchPlaceholder'),
      logCopy: t('dag.logCopy'),
      logCopied: t('dag.logCopied'),
      logWrap: t('dag.logWrap'),
      logAutoScroll: t('dag.logAutoScroll'),
      logLines: t('dag.logLines'),
    }),
    [t]
  );

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const projects = projectsQuery.data ?? [];
  const selectedProject = useMemo(() => projects.find((p) => p.projectId === selectedProjectId), [projects, selectedProjectId]);

  // Default the selected project to the first one once projects load.
  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) {
      setSelectedProjectId(projects[0].projectId);
    }
  }, [projects, selectedProjectId]);

  const componentsQuery = useQuery({
    queryKey: ['components'],
    queryFn: async () => {
      const list = await apiClient.getComponents();
      const all: ComponentSummaryDef[] = [];
      const groups: Record<string, DAGComponentDef[]> = {};
      list.forEach((group) => {
        const groupName = group.name || 'Components';
        const defs: DAGComponentDef[] = [];
        (group.comps || []).forEach((c) => {
          all.push(c);
          defs.push({
            domain: c.domain || 'unknown',
            name: c.name || 'unknown',
            version: c.version,
            desc: c.desc,
            icon: c.domain?.startsWith('ml') ? '🤖' : '⚙️',
          });
        });
        if (defs.length) groups[groupName] = defs;
      });
      return { all, groups };
    },
  });
  const components = componentsQuery.data?.all ?? [];
  const componentGroups = componentsQuery.data?.groups ?? {};

  const i18nQuery = useQuery({
    queryKey: ['component-i18n'],
    queryFn: () => apiClient.listComponentI18n(),
  });
  const i18nMap = i18nQuery.data ?? {};

  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: !!selectedProjectId,
  });
  const graphs = graphsQuery.data ?? [];
  const selectedGraph: GraphMetaVO | null =
    graphs.find((g) => g.graphId === selectedGraphId) ?? graphs[0] ?? null;

  // Poll the graph detail while any node is running (replaces setInterval).
  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', selectedProjectId, selectedGraph?.graphId],
    queryFn: () => apiClient.getGraphDetail(selectedProjectId, selectedGraph!.graphId!),
    enabled: !!selectedProjectId && !!selectedGraph?.graphId,
    refetchInterval: (query) => {
      const hasRunning = query.state.data?.nodes?.some((n) => n.status === 'RUNNING');
      return hasRunning ? 3000 : false;
    },
  });
  const graphDetail = graphDetailQuery.data ?? null;

  const invalidateGraphs = () =>
    queryClient.invalidateQueries({ queryKey: ['graphs', selectedProjectId] });

  const templateWizard = useTemplateWizard(selectedProject, (graphId) => {
    setSelectedGraphId(graphId);
  });

  const createGraphMutation = useMutation({
    mutationFn: () => apiClient.createGraph({ projectId: selectedProjectId, name: newGraphName }),
    onSuccess: () => {
      setIsCreateModalOpen(false);
      setNewGraphName('');
      invalidateGraphs();
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const deleteGraphMutation = useMutation({
    mutationFn: (graph: GraphMetaVO) => apiClient.deleteGraph(selectedProjectId, graph.graphId!),
    onSuccess: (_data, graph) => {
      if (selectedGraph?.graphId === graph.graphId) {
        setSelectedGraphId('');
      }
      setDeleteGraphTarget(null);
      invalidateGraphs();
      queryClient.invalidateQueries({ queryKey: ['graph-detail', selectedProjectId, graph.graphId] });
    },
    onError: (e) => {
      setDeleteGraphTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const stopGraphMutation = useMutation({
    mutationFn: () => apiClient.stopGraph(selectedProjectId, selectedGraph!.graphId!),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['graph-detail', selectedProjectId, selectedGraph?.graphId],
      });
      toast.success(t('dag.stopSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const renameGraphMutation = useMutation({
    mutationFn: () => apiClient.renameGraph(selectedProjectId, selectedGraph!.graphId!, renameValue),
    onSuccess: () => {
      setIsRenameModalOpen(false);
      invalidateGraphs();
      toast.success(t('dag.renameSuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const handleCreateGraph = (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProjectId || !newGraphName.trim()) return;
    setError(null);
    createGraphMutation.mutate();
  };

  const handleDeleteGraph = (graph: GraphMetaVO) => {
    if (!selectedProjectId || !graph.graphId) return;
    setDeleteGraphTarget(graph);
  };

  const openRename = () => {
    if (!selectedGraph) return;
    setRenameValue(selectedGraph.name || '');
    setIsRenameModalOpen(true);
  };

  const openTemplate = () => {
    templateWizard.open();
  };

  const handleSaveGraph = async (nodes: DAGNode[], edges: DAGEdge[]) => {
    if (!selectedProjectId || !selectedGraph?.graphId) return;
    try {
      await apiClient.updateGraph(
        selectedProjectId,
        selectedGraph.graphId,
        nodes.map(mapDAGNodeToGraphNode),
        edges.map(mapDAGEdgeToGraphEdge)
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    }
  };

  const handleRunGraph = async (nodes: DAGNode[]) => {
    if (!selectedProjectId || !selectedGraph?.graphId) return;
    try {
      const jobId = await apiClient.startGraph(
        selectedProjectId,
        selectedGraph.graphId,
        nodes.map((n) => n.id)
      );
      // Kick off status polling by refreshing the graph detail.
      queryClient.invalidateQueries({
        queryKey: ['graph-detail', selectedProjectId, selectedGraph.graphId],
      });
      toast.success(t('dag.started', { jobId }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    }
  };

  const handleNodeConfigChange = async (node: DAGNode) => {
    if (!selectedProjectId || !selectedGraph?.graphId) return;
    try {
      await apiClient.updateGraphNode(selectedProjectId, selectedGraph.graphId, mapDAGNodeToGraphNode(node));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    }
  };

  const handleNodeLogs = async (node: DAGNode) => {
    if (!selectedProjectId || !selectedGraph?.graphId) {
      return [];
    }
    return apiClient.getGraphNodeLogs(selectedProjectId, selectedGraph.graphId, node.id);
  };

  const handleNodeOutput = async (node: DAGNode) => {
    if (!selectedProjectId || !selectedGraph?.graphId) {
      return null;
    }
    const outputId = node.outputs?.[0] || `${node.id}-output-0`;
    try {
      return await apiClient.getGraphNodeOutput(selectedProjectId, selectedGraph.graphId, node.id, outputId);
    } catch {
      return null;
    }
  };

  const handleGetComponentDef = async (node: DAGNode) => {
    if (!node.codeName) return null;
    const { domain, name } = normalizeCodeName(node.codeName);
    try {
      const defs = await apiClient.batchGetComponent([{ domain, name }]);
      const def = (defs[node.codeName] || Object.values(defs)[0]) ?? null;
      if (!def) return null;
      // 映射为画布可用的组件元数据：
      // - inputs/outputs 携带名称、描述与允许的数据类型（供组件解释器展示）；
      // - attrs 保留原始 AttributeDef 结构（供高级配置表单解析）。
      const defAny = def as Record<string, any>;
      return {
        desc: def.desc,
        version: defAny.version,
        domain: defAny.domain ?? domain,
        inputs: (def.inputs || []).map((item: any) => ({
          name: item?.name,
          desc: item?.desc,
          types: Array.isArray(item?.types) ? item.types : undefined,
        })),
        outputs: (def.outputs || []).map((item: any) => ({
          name: item?.name,
          desc: item?.desc,
          types: Array.isArray(item?.types) ? item.types : undefined,
        })),
        attrs: def.attrs as Array<Record<string, unknown>> | undefined,
      };
    } catch {
      return null;
    }
  };

  const handleAddNode = async (component: DAGComponentDef): Promise<DAGNode> => {
    const codeName = `${component.domain}/${component.name}`;
    const label = i18nMap[component.name] || i18nMap[codeName] || component.name;
    let outputs: string[] = [];
    try {
      const defs = await apiClient.batchGetComponent([
        { app: 'secretflow', domain: component.domain, name: component.name },
      ]);
      const def = defs[codeName] || Object.values(defs)[0];
      if (def?.outputs) {
        outputs = def.outputs.map((_: any, idx: number) => `${codeName}-output-${idx}`);
      }
    } catch {
      // fallback to no outputs
    }
    return {
      id: `${component.domain}-${component.name}-${Date.now().toString(36)}`,
      name: label,
      category: component.domain,
      icon: component.icon || '⚙️',
      status: 'Ready',
      x: 200 + Math.random() * 120,
      y: 120 + Math.random() * 80,
      codeName,
      nodeDef: {
        domain: component.domain,
        name: component.name,
        version: component.version,
      },
      inputs: [],
      outputs,
    };
  };

  const handleConnect = (sourceId: string, targetId: string) => {
    const sourceNode = graphDetail?.nodes?.find((n) => n.graphNodeId === sourceId);
    const outputIndex = sourceNode?.outputs?.length || 0;
    const sourceAnchor = `${sourceId}-output-${outputIndex}`;
    const targetAnchor = `${targetId}-input-${graphDetail?.edges?.filter((e) => e.target === targetId).length || 0}`;
    return {
      id: `edge-${Date.now().toString(36)}`,
      source: sourceId,
      target: targetId,
      sourceAnchor,
      targetAnchor,
    };
  };

  const handleNodeMove = async (node: DAGNode) => {
    await handleNodeConfigChange(node);
  };

  const { nodes, edges } = mapGraphToDAG(graphDetail || undefined);

  const hasRunningNodes = (graphDetail?.nodes || []).some((n) => n.status === 'RUNNING');

  const loading = graphsQuery.isLoading || graphDetailQuery.isLoading;
  const queryError =
    graphsQuery.error?.message ||
    graphDetailQuery.error?.message ||
    projectsQuery.error?.message ||
    componentsQuery.error?.message ||
    null;

  return (
    <div className="space-y-4 h-full flex flex-col">
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('dag.title')}</h2>
          <p className="text-xs text-gray-500">{t('dag.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          <select
            value={selectedProjectId}
            onChange={(e) => {
              setSelectedProjectId(e.target.value);
              setSelectedGraphId('');
            }}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {projects.map((p) => (
              <option key={p.projectId} value={p.projectId}>
                {p.projectName}
              </option>
            ))}
          </select>

          <select
            value={selectedGraph?.graphId || ''}
            onChange={(e) => setSelectedGraphId(e.target.value)}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {graphs.map((g) => (
              <option key={g.graphId} value={g.graphId || ''}>
                {g.name}
              </option>
            ))}
          </select>

          <AccessGuard access={{ types: [Platform.CENTER] }}>
            <Button variant="primary" size="sm" onClick={() => setIsCreateModalOpen(true)}>
              {t('dag.create')}
            </Button>
            {selectedGraph && (
              <Button variant="outline" size="sm" onClick={openRename}>
                {t('dag.rename')}
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={openTemplate}
              disabled={!selectedProject || selectedProject.nodes.length === 0}
            >
              {t('dag.template')}
            </Button>
            {selectedGraph && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setIsScheduledModalOpen(true)}
              >
                {t('dag.createPeriodicTask')}
              </Button>
            )}
            {selectedNode && selectedNode.status === 'Success' && (selectedNode.codeName || '').includes('train') && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setIsPackModalOpen(true)}
              >
                {t('dag.packModel')}
              </Button>
            )}
            {hasRunningNodes && (
              <Button variant="outline" size="sm" loading={stopGraphMutation.isPending} onClick={() => stopGraphMutation.mutate()}>
                ⏹ {t('dag.stop')}
              </Button>
            )}
            <Button variant="danger" size="sm" onClick={() => selectedGraph && handleDeleteGraph(selectedGraph)}>
              {t('common.delete')}
            </Button>
          </AccessGuard>
        </div>
      </div>

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <div className="flex-1 min-h-0">
        {selectedGraph ? (
          <AccessGuard access={{ types: [Platform.CENTER] }} fallback={<DAGNextWorkspace readOnly title={selectedGraph.name} initialNodes={nodes} initialEdges={edges} labels={dagLabels} onGetComponentDef={handleGetComponentDef} />}>
            <DAGNextWorkspace
              title={selectedGraph.name}
              initialNodes={nodes}
              initialEdges={edges}
              componentGroups={componentGroups}
              i18nMap={i18nMap}
              onSaveGraph={handleSaveGraph}
              onRunGraph={handleRunGraph}
              onNodeMove={handleNodeMove}
              onNodeConfigChange={handleNodeConfigChange}
              onNodeLogs={handleNodeLogs}
              onNodeOutput={handleNodeOutput}
              onNodeSelect={setSelectedNode}
              onAddNode={handleAddNode}
              onConnect={handleConnect}
              onGetComponentDef={handleGetComponentDef}
              loading={loading}
              labels={dagLabels}
            />
          </AccessGuard>
        ) : (
          <Card className="h-full flex items-center justify-center">
            <div className="text-center text-xs text-gray-400">
              <div className="mb-2">{t('dag.noGraph')}</div>
              <AccessGuard access={{ types: [Platform.CENTER] }}>
                <Button variant="primary" size="sm" onClick={() => setIsCreateModalOpen(true)}>
                  {t('dag.create')}
                </Button>
              </AccessGuard>
            </div>
          </Card>
        )}
      </div>

      {components.length > 0 && (
        <Card title={t('dag.components')}>
          <div className="flex flex-wrap gap-2 text-xs">
            {components.slice(0, 20).map((c, idx) => (
              <Badge key={idx} status="default">
                {c.name || c.domain}
              </Badge>
            ))}
          </div>
        </Card>
      )}

      <Modal
        isOpen={isCreateModalOpen}
        onClose={() => {
          setIsCreateModalOpen(false);
          setNewGraphName('');
        }}
        title={t('dag.createTitle')}
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setIsCreateModalOpen(false);
                setNewGraphName('');
              }}
            >
              {t('common.cancel')}
            </Button>
            <Button variant="primary" onClick={handleCreateGraph} loading={createGraphMutation.isPending}>
              {t('common.create')}
            </Button>
          </>
        }
      >
        <form onSubmit={handleCreateGraph} className="space-y-4 text-xs">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dag.nameLabel')}</label>
            <input
              type="text"
              value={newGraphName}
              onChange={(e) => setNewGraphName(e.target.value)}
              placeholder={t('dag.namePlaceholder')}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>
        </form>
      </Modal>

      {/* Rename Graph Modal */}
      <Modal
        isOpen={isRenameModalOpen}
        onClose={() => setIsRenameModalOpen(false)}
        title={t('dag.renameTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setIsRenameModalOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={() => renameGraphMutation.mutate()} loading={renameGraphMutation.isPending} disabled={!renameValue.trim()}>{t('common.save')}</Button>
          </>
        }
      >
        <div className="text-xs">
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dag.nameLabel')}</label>
          <input
            type="text"
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            required
          />
        </div>
      </Modal>

      {/* Delete Graph Confirm Dialog */}
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

      {/* DAG Template Wizard */}
      <TemplateWizard project={selectedProject} {...templateWizard} />

      {/* DAG Model Pack Modal */}
      {selectedNode && selectedGraph && (
        <ModelPackModal
          isOpen={isPackModalOpen}
          onClose={() => setIsPackModalOpen(false)}
          projectId={selectedProjectId}
          graphId={selectedGraph.graphId || ''}
          trainNode={selectedNode}
          onPacked={() => {
            setIsPackModalOpen(false);
            toast.success(t('models.packSuccess'));
          }}
        />
      )}

      {/* DAG Scheduled Task Modal */}
      {selectedGraph && (
        <ScheduledTaskFromDagModal
          isOpen={isScheduledModalOpen}
          onClose={() => setIsScheduledModalOpen(false)}
          projectId={selectedProjectId}
          graphId={selectedGraph.graphId || ''}
          graphName={selectedGraph.name}
          nodes={nodes}
        />
      )}
    </div>
  );
};
