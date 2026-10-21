import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Badge } from '@secretpad/design-system';
import { AttributeForm } from './attribute-form';
import type { AttrDataProvider, AttributeFormHandle, AttributeFormChangeMeta } from './attribute-form';
import { ComponentInterpreter } from './component-interpreter';
import { LogViewer } from './log-viewer';
import { ResultVisualization } from './result-visualization';
import type { ResultVisualizationProps } from './result-visualization';
import { BinningModification } from './binning-modification';
import type { BinningData } from './binning-modification';
import { LinearModelEditor } from './linear-model-editor';
import type { LinearModelData } from './custom-serializers';
import type { ConnectionError, PortDef } from './graph-model';
import {
  CONTINUE_RUN_CODE_NAMES,
  clearNodeDefAttrs,
  deriveInputs,
  edgeIdOf,
  inputAnchor,
  isModificationNode,
  outputAnchor,
  parseAnchor,
  pickConnection,
  remapImportedGraph,
  validateConnection,
  downstreamIds,
  upstreamIds,
} from './graph-model';
import type { DAGNodeStatus } from './status';
import type { ValidationError } from './component-config';

// 统一导出属性动态表单。
export { AttributeForm, buildAttrTree, prepareDefs, buildInitialState, serializeState, collectErrors, defaultFormatError, MultiSelect } from './attribute-form';
export type {
  AttributeDef,
  AttributeValue,
  AttributeFormLabels,
  AttributeFormProps,
  AttrDataProvider,
  AttributeFormHandle,
  AttributeFormChangeMeta,
  ColumnOption,
  OptionItem,
  AttrTreeNode,
  FormState,
} from './attribute-form';
export * from './component-config';
export * from './custom-serializers';
export { CaseWhenEditor, CalculateOpEditor, GroupByEditor, UpstreamFeatureView, CustomProtobufField } from './custom-renderers';
export * from './graph-model';
export * from './status';
// 统一导出组件解释器。
export { ComponentInterpreter } from './component-interpreter';
export type { ComponentInterpreterProps, ComponentInterpreterLabels, InterpreterMetadata, IoPortMeta } from './component-interpreter';
// 统一导出日志查看器。
export { LogViewer } from './log-viewer';
export type { LogViewerProps, LogViewerLabels } from './log-viewer';
// 统一导出结果可视化组件集。
export { ResultVisualization, OutputTable, StatsChart, KeyValuePanel, CorrelationHeatmap } from './result-visualization';
export type { ResultVisualizationProps, ResultVisualizationLabels, OutputTableProps, StatsChartProps, KeyValuePanelProps, CorrelationHeatmapProps } from './result-visualization';
export {
  NodeResultView,
  ReportResult,
  ReportTabsView,
  ReportDivView,
  TableResult,
  PathResult,
  SortableTable,
  SimpleTabs,
  HBarChart,
  MultiLineChart,
  PivotTable,
  ChartFrame,
  FeatureImportanceVis,
  SampleVis,
  movePivotField,
  registerResultVisualization,
  hasResultVisualization,
} from './result/result-view';
export type { NodeResultViewProps, ResultActions, ResultViewLabels, TabVisProps, ReportVisProps } from './result/result-view';
export * from './result/report';
export * from './result/output';
// 统一导出执行记录时间线。
export { ExecutionTimeline } from './execution-timeline';
export type { ExecutionTimelineProps, ExecutionTimelineLabels, ExecutionRecord, JobStatus, PaginationInfo } from './execution-timeline';
// 统一导出 SQL 编辑器。
export { SqlEditor } from './sql-editor';
export type { SqlEditorProps, SqlEditorLabels } from './sql-editor';

export { TemplateQuickConfig, validateQuickConfig, quickConfigValues } from './template-quick-config';
export type { TemplateQuickConfigProps, QuickConfigLabels, QuickConfigValues, TemplateType, TableInfo } from './template-quick-config';

export { LinearModelEditor } from './linear-model-editor';
export type { LinearModelEditorProps } from './linear-model-editor';
export { BinningModification } from './binning-modification';
export type { BinningModificationProps, BinningModificationLabels, BinningData, BinningRecord, Bin } from './binning-modification';

export type { DAGNodeStatus } from './status';

export interface DAGNode {
  id: string;
  name: string;
  category: string;
  icon: string;
  status: DAGNodeStatus;
  x: number;
  y: number;
  config?: Record<string, unknown>;
  codeName?: string;
  nodeDef?: Record<string, unknown>;
  inputs?: string[];
  outputs?: string[];
  progress?: number;
  /** 由组件 IoDef 生成的端口（report / read_data 输出不出端口）。 */
  ports?: PortDef[];
  /** 可查看的结果输出（排除 read_data）。 */
  resultOutputs?: Array<{ id: string; name?: string; type?: string }>;
  /** 配置完成度（false 时在画布上标记“未配置”）。 */
  configFinished?: boolean;
}

export interface DAGEdge {
  id: string;
  source: string;
  target: string;
  sourceAnchor?: string;
  targetAnchor?: string;
}

export interface DAGComponentDef {
  domain: string;
  name: string;
  version?: string;
  desc?: string;
  icon?: string;
}

interface ComponentMetadata {
  desc?: string;
  version?: string;
  domain?: string;
  inputs?: Array<{ name?: string; type?: string; desc?: string; types?: string[]; attrs?: Array<Record<string, unknown>> }>;
  outputs?: Array<{ name?: string; type?: string; desc?: string; types?: string[] }>;
  attrs?: Array<Record<string, unknown>>;
}

export type GraphChangeReason =
  | 'add'
  | 'delete'
  | 'move'
  | 'connect'
  | 'disconnect'
  | 'config'
  | 'rename'
  | 'paste'
  | 'layout'
  | 'undo'
  | 'redo'
  | 'import';

export interface DAGCanvasLabels {
  operatorLibrary?: string;
  openPalette?: string;
  closePalette?: string;
  closeInspector?: string;
  noOperators?: string;
  nodesEdges?: string;
  connect?: string;
  clickTarget?: string;
  connectionHint?: string;
  parameters?: string;
  logs?: string;
  output?: string;
  save?: string;
  run?: string;
  nodeIdentifier?: string;
  operatorName?: string;
  codeName?: string;
  executionStatus?: string;
  position?: string;
  frontendConfig?: string;
  nodeDef?: string;
  applyConfig?: string;
  status?: string;
  noLogs?: string;
  noOutput?: string;
  refresh?: string;
  nodeOutput?: string;
  deleteNode?: string;
  emptyCanvas?: string;
  advancedConfig?: string;
  configIssues?: string;
  validateConfig?: string;
  configValid?: string;
  noAttrs?: string;
  optional?: string;
  required?: string;
  none?: string;
  listPlaceholder?: string;
  interpreterTitle?: string;
  interpreterDesc?: string;
  interpreterInputs?: string;
  interpreterOutputs?: string;
  interpreterAttrs?: string;
  interpreterLoading?: string;
  interpreterNoDef?: string;
  interpreterTypes?: string;
  interpretComponent?: string;
  logSearchPlaceholder?: string;
  logCopy?: string;
  logCopied?: string;
  logWrap?: string;
  logAutoScroll?: string;
  logLines?: string;
  runSingle?: string;
  runDown?: string;
  runUp?: string;
  stop?: string;
  stopNode?: string;
  continueRun?: string;
  tidyLayout?: string;
  selectNodeFirst?: string;
  fullscreen?: string;
  exitFullscreen?: string;
  exportJson?: string;
  importJson?: string;
  records?: string;
  search?: string;
  searchPlaceholder?: string;
  paletteSearch?: string;
  undo?: string;
  redo?: string;
  snapOn?: string;
  snapOff?: string;
  binningTab?: string;
  cloudLogs?: string;
  /** 云日志未配置（cloud_log/sls 返回 config:false）提示。 */
  cloudLogNotConfigured?: string;
  cloudLogHelp?: string;
  unfinished?: string;
  selectOutput?: string;
  autoRefresh?: string;
  unsaved?: string;
  saving?: string;
  saved?: string;
  components?: string;
  copy?: string;
  paste?: string;
  duplicate?: string;
  delete?: string;
  pending?: string;
  ready?: string;
  running?: string;
  success?: string;
  failed?: string;
  stopped?: string;
}

export interface DAGCanvasProps {
  title?: string;
  initialNodes?: DAGNode[];
  initialEdges?: DAGEdge[];
  components?: DAGComponentDef[];
  componentGroups?: Record<string, DAGComponentDef[]>;
  i18nMap?: Record<string, string>;
  /** 只读：禁止编辑，但仍可查看日志、输出与配置。 */
  readOnly?: boolean;
  loading?: boolean;
  labels?: DAGCanvasLabels;
  /** 轮询得到的节点状态覆盖（graph/node/status）。 */
  nodeStatuses?: Record<string, { status: DAGNodeStatus; progress?: number }>;
  /** 保存状态提示：dirty / saving / saved。 */
  saveState?: 'dirty' | 'saving' | 'saved' | 'idle';
  /** 默认开启网格吸附。 */
  defaultSnapToGrid?: boolean;
  /** 日志自动刷新间隔（节点运行中时生效），ms。 */
  logPollInterval?: number;
  /** 顶栏右侧额外按钮。 */
  toolbarExtra?: React.ReactNode;
  /** 左侧栏额外 Tab（如数据表树）。 */
  sidebarTabs?: Array<{ key: string; label: string; content: React.ReactNode }>;
  /**
   * 画布区域内的浮层（如模板快速配置抽屉）。渲染在工具栏下方、以画布行为定位容器，
   * 与旧版 dag-layout 中 Drawer `getContainer=.center` 一致，不覆盖全局 Header。
   */
  centerOverlay?: React.ReactNode;
  onNodeSelect?: (node: DAGNode | null) => void;
  onNodeMove?: (node: DAGNode) => void | Promise<void>;
  onNodeConfigChange?: (node: DAGNode) => void | Promise<void>;
  onNodeLogs?: (node: DAGNode) => Promise<string[] | { status?: string; logs?: string[] }>;
  /** 云日志（SLS）。 */
  onCloudLogs?: (node: DAGNode) => Promise<string[] | { status?: string; logs?: string[]; config?: boolean }>;
  onNodeOutput?: (node: DAGNode, outputId?: string) => Promise<Record<string, unknown> | null>;
  /** 自定义输出渲染（宿主注入下载 / TEE 申请等操作）。 */
  outputViewProps?: (node: DAGNode, outputId?: string) => Partial<ResultVisualizationProps>;
  onGetComponentDef?: (node: DAGNode) => Promise<ComponentMetadata | null>;
  /** 分箱修改数据；source=upstream 取上游输出，latest 取已保存配置。 */
  onNodeBinningData?: (node: DAGNode, source?: 'upstream' | 'latest') => Promise<BinningData | null>;
  onSaveBinningData?: (node: DAGNode, data: BinningData) => void | Promise<void>;
  /** 分箱合并（标记 markForMerge → 保存 → 执行节点）。 */
  onBinningMerge?: (node: DAGNode, data: BinningData) => void | Promise<void>;
  /** 线性模型参数修改数据。 */
  onNodeModelParams?: (node: DAGNode, source?: 'upstream' | 'latest') => Promise<LinearModelData | null>;
  onSaveModelParams?: (node: DAGNode, data: LinearModelData) => void | Promise<void>;
  attrDataProvider?: AttrDataProvider;
  /** 按节点生成属性数据提供器（列候选依赖上游）。优先于 attrDataProvider。 */
  attrDataProviderFor?: (node: DAGNode, nodes: DAGNode[], edges: DAGEdge[]) => AttrDataProvider | undefined;
  /** 属性名 / 描述翻译。 */
  attrTranslate?: (node: DAGNode) => ((text: string) => string) | undefined;
  /** 计算模式（面板样式注册表按模式区分）。 */
  computeMode?: 'MPC' | 'TEE';
  /** 属性校验失败（应用配置时）。 */
  onValidationFailed?: (node: DAGNode, errors: ValidationError[]) => void;
  onSaveGraph?: (nodes: DAGNode[], edges: DAGEdge[]) => void | Promise<void>;
  /** 任意结构 / 位置 / 配置变化（用于脏标记与自动保存）。 */
  onGraphChange?: (nodes: DAGNode[], edges: DAGEdge[], reason: GraphChangeReason) => void;
  onRunGraph?: (nodes: DAGNode[], edges: DAGEdge[]) => void | Promise<void>;
  onRunSingle?: (node: DAGNode) => void | Promise<void>;
  onRunDown?: (node: DAGNode, downstreamNodes: DAGNode[]) => void | Promise<void>;
  onRunUp?: (node: DAGNode, upstreamNodes: DAGNode[]) => void | Promise<void>;
  onStopGraph?: () => void | Promise<void>;
  /** 停止单个节点。 */
  onStopNode?: (node: DAGNode) => void | Promise<void>;
  /** 断点继续执行（训练组件）。 */
  onContinueRun?: (node: DAGNode) => void | Promise<void>;
  onAddNode?: (component: DAGComponentDef) => DAGNode | Promise<DAGNode>;
  /** 为粘贴 / 复制节点分配新 ID（graph/node/max_index）。 */
  allocateNodeIds?: (count: number) => Promise<string[]>;
  onConnect?: (
    sourceId: string,
    targetId: string,
    anchors?: { sourceAnchor: string; targetAnchor: string },
  ) => DAGEdge | Promise<DAGEdge> | null | undefined;
  /** 连线被校验拒绝。 */
  onConnectionRejected?: (reason: ConnectionError) => void;
}

const NODE_WIDTH = 144; // w-36
const NODE_HEIGHT = 64;
const GRID_SIZE = 16;
const DRAG_THRESHOLD = 3;

/**
 * 稳定的空值默认值：解构默认参数（如 `initialNodes = []`）会在组件每次渲染时
 * 重新求值产生新引用，而下方 useEffect([initialNodes]) 以引用变化为依据做
 * 「服务端 → 画布」全量同步，因此默认值一律使用模块级常量。
 */
const DEFAULT_NODES: DAGNode[] = [];
const DEFAULT_EDGES: DAGEdge[] = [];
const DEFAULT_COMPONENTS: DAGComponentDef[] = [];
const DEFAULT_I18N_MAP: Record<string, string> = {};
const DEFAULT_LABELS: DAGCanvasLabels = {};

function getStatusBadge(status: DAGNodeStatus, labels: DAGCanvasLabels): { status: 'success' | 'processing' | 'error' | 'warning' | 'default'; label: string } {
  switch (status) {
    case 'Success':
      return { status: 'success', label: labels.success ?? 'Success' };
    case 'Running':
      return { status: 'processing', label: labels.running ?? 'Running' };
    case 'Pending':
      return { status: 'processing', label: labels.pending ?? 'Pending' };
    case 'Failed':
      return { status: 'error', label: labels.failed ?? 'Failed' };
    case 'Stopped':
      return { status: 'warning', label: labels.stopped ?? 'Stopped' };
    default:
      return { status: 'default', label: labels.ready ?? 'Ready' };
  }
}

function safeJsonStringify(value: unknown, fallback = '{}'): string {
  try {
    return JSON.stringify(value ?? null, null, 2);
  } catch {
    return fallback;
  }
}

function safeJsonParse(value: string): Record<string, unknown> | null {
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function formatReactChild(val: unknown, fallback = ''): string {
  if (val === null || val === undefined) return fallback;
  if (typeof val === 'string') return val;
  if (typeof val === 'number' || typeof val === 'boolean') return String(val);
  if (typeof val === 'object') {
    const o = val as { name?: string; desc?: string; label?: string; codeName?: string };
    return o.name || o.desc || o.label || o.codeName || fallback;
  }
  return String(val);
}

/** 端口在节点上的相对位置：输入在左侧，输出在右侧，按序号均分高度。 */
function portOffset(port: PortDef, ports: PortDef[]): { x: number; y: number } {
  const group = ports.filter((p) => p.group === port.group);
  const idx = group.findIndex((p) => p.id === port.id);
  const y = (NODE_HEIGHT * (idx + 1)) / (group.length + 1);
  return { x: port.group === 'input' ? 0 : NODE_WIDTH, y };
}

function anchorPoint(node: DAGNode, anchor: string | undefined, fallback: 'input' | 'output'): { x: number; y: number } {
  const ports = node.ports || [];
  const port = anchor ? ports.find((p) => p.id === anchor) : undefined;
  if (port) {
    const o = portOffset(port, ports);
    return { x: node.x + o.x, y: node.y + o.y };
  }
  return { x: node.x + (fallback === 'input' ? 0 : NODE_WIDTH), y: node.y + NODE_HEIGHT / 2 };
}

/**
 * 自动布局算法：拓扑排序 + 分层布局（对应原版 `tidy-layout.ts` 中的 Dagre 布局）。
 */
function tidyLayout(nodes: DAGNode[], edges: DAGEdge[]): DAGNode[] {
  if (nodes.length === 0) return nodes;
  const inDegree = new Map<string, number>();
  const adjacency = new Map<string, string[]>();
  nodes.forEach((n) => {
    inDegree.set(n.id, 0);
    adjacency.set(n.id, []);
  });
  edges.forEach((e) => {
    if (inDegree.has(e.target) && adjacency.has(e.source)) {
      inDegree.set(e.target, (inDegree.get(e.target) || 0) + 1);
      adjacency.get(e.source)!.push(e.target);
    }
  });
  const layers: string[][] = [];
  let queue = nodes.filter((n) => (inDegree.get(n.id) || 0) === 0).map((n) => n.id);
  const visited = new Set<string>();
  while (queue.length > 0) {
    layers.push([...queue]);
    queue.forEach((id) => visited.add(id));
    const next: string[] = [];
    for (const id of queue) {
      for (const target of adjacency.get(id) || []) {
        const deg = (inDegree.get(target) || 1) - 1;
        inDegree.set(target, deg);
        if (deg === 0 && !visited.has(target)) next.push(target);
      }
    }
    queue = next;
  }
  const unvisited = nodes.filter((n) => !visited.has(n.id));
  if (unvisited.length > 0) layers.push(unvisited.map((n) => n.id));
  const nodeMap = new Map(nodes.map((n) => [n.id, n]));
  const result: DAGNode[] = [];
  layers.forEach((layer, layerIdx) => {
    const x = 80 + layerIdx * 220;
    const offsetY = 60 + (300 - ((layer.length - 1) * 100) / 2);
    layer.forEach((id, nodeIdx) => {
      const node = nodeMap.get(id);
      if (node) result.push({ ...node, x, y: offsetY + nodeIdx * 100 });
    });
  });
  return result;
}

type InspectorTab = 'config' | 'log' | 'cloudLog' | 'output' | 'binning';

export const DAGNextWorkspace: React.FC<DAGCanvasProps> = ({
  title = 'DAG Pipeline Editor',
  initialNodes = DEFAULT_NODES,
  initialEdges = DEFAULT_EDGES,
  components = DEFAULT_COMPONENTS,
  componentGroups,
  i18nMap = DEFAULT_I18N_MAP,
  readOnly = false,
  loading = false,
  nodeStatuses,
  saveState,
  defaultSnapToGrid = true,
  logPollInterval = 3000,
  toolbarExtra,
  sidebarTabs,
  centerOverlay,
  onNodeSelect,
  onNodeMove,
  onNodeConfigChange,
  onNodeLogs,
  onCloudLogs,
  onNodeOutput,
  outputViewProps,
  onGetComponentDef,
  onNodeBinningData,
  onSaveBinningData,
  onBinningMerge,
  onNodeModelParams,
  onSaveModelParams,
  attrDataProvider,
  attrDataProviderFor,
  attrTranslate,
  computeMode = 'MPC',
  onValidationFailed,
  onSaveGraph,
  onGraphChange,
  onRunGraph,
  onRunSingle,
  onRunDown,
  onRunUp,
  onStopGraph,
  onStopNode,
  onContinueRun,
  onAddNode,
  allocateNodeIds,
  onConnect,
  onConnectionRejected,
  labels = DEFAULT_LABELS,
}) => {
  const [nodes, setNodes] = useState<DAGNode[]>(initialNodes);
  const [edges, setEdges] = useState<DAGEdge[]>(initialEdges);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [zoom, setZoom] = useState(100);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [panOffset, setPanOffset] = useState({ x: 0, y: 0 });
  const [isPanning, setIsPanning] = useState(false);
  const panStartRef = useRef({ x: 0, y: 0, panX: 0, panY: 0 });
  const [searchText, setSearchText] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const [paletteSearchText, setPaletteSearchText] = useState('');
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());
  const [sidebarTab, setSidebarTab] = useState<string>('components');
  /** 撤销/重做历史栈（对应原版 X6 history 插件）。 */
  const undoStackRef = useRef<Array<{ nodes: DAGNode[]; edges: DAGEdge[] }>>([]);
  const redoStackRef = useRef<Array<{ nodes: DAGNode[]; edges: DAGEdge[] }>>([]);
  const [canUndo, setCanUndo] = useState(false);
  const [canRedo, setCanRedo] = useState(false);
  const [snapToGrid, setSnapToGrid] = useState(defaultSnapToGrid);
  const [activeTab, setActiveTab] = useState<InspectorTab>('config');
  const [mobilePaletteOpen, setMobilePaletteOpen] = useState(false);
  const [binningData, setBinningData] = useState<BinningData | null>(null);
  const [modelParams, setModelParams] = useState<LinearModelData | null>(null);
  const [modificationSource, setModificationSource] = useState<'upstream' | 'latest' | undefined>(undefined);
  const [drag, setDrag] = useState<{ nodeId: string; offsetX: number; offsetY: number; startX: number; startY: number; moved: boolean } | null>(null);
  const [linking, setLinking] = useState<{ nodeId: string; anchor: string; x: number; y: number } | null>(null);
  const [connectSourceId, setConnectSourceId] = useState<string | null>(null);
  const [logs, setLogs] = useState<{ status?: string; logs: string[] }>({ logs: [] });
  const [cloudLogs, setCloudLogs] = useState<{ status?: string; logs: string[]; config?: boolean }>({ logs: [] });
  const [output, setOutput] = useState<Record<string, unknown> | null>(null);
  const [selectedOutputId, setSelectedOutputId] = useState<string | undefined>(undefined);
  const [componentDef, setComponentDef] = useState<ComponentMetadata | null>(null);
  const [panelLoading, setPanelLoading] = useState(false);
  const [pendingConnection, setPendingConnection] = useState(false);
  const [interpreterComponent, setInterpreterComponent] = useState<DAGComponentDef | null>(null);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; node: DAGNode } | null>(null);
  const [selectionRect, setSelectionRect] = useState<{ x1: number; y1: number; x2: number; y2: number } | null>(null);
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(new Set());
  const [hoveredNodeId, setHoveredNodeId] = useState<string | null>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const clipboardRef = useRef<DAGNode[] | null>(null);
  const [hasClipboard, setHasClipboard] = useState(false);
  const formRef = useRef<AttributeFormHandle>(null);
  /** 标记下一次 nodes/edges 变化来自用户操作（用于 onGraphChange）。 */
  const changeReasonRef = useRef<GraphChangeReason | null>(null);
  const nodesRef = useRef(nodes);
  const edgesRef = useRef(edges);
  useEffect(() => {
    nodesRef.current = nodes;
    edgesRef.current = edges;
  });

  useEffect(() => {
    setNodes(initialNodes);
  }, [initialNodes]);

  useEffect(() => {
    setEdges(initialEdges);
  }, [initialEdges]);

  /** 用户操作导致的变化 → 通知宿主（脏标记 / 自动保存）。 */
  useEffect(() => {
    const reason = changeReasonRef.current;
    if (!reason) return;
    changeReasonRef.current = null;
    onGraphChange?.(nodes, edges, reason);
  }, [nodes, edges, onGraphChange]);

  const markChange = (reason: GraphChangeReason) => {
    changeReasonRef.current = reason;
  };

  const effectiveStatus = useCallback(
    (node: DAGNode): DAGNodeStatus => nodeStatuses?.[node.id]?.status ?? node.status,
    [nodeStatuses],
  );
  const effectiveProgress = (node: DAGNode) => nodeStatuses?.[node.id]?.progress ?? node.progress;

  const selectedNode = useMemo(() => nodes.find((n) => n.id === selectedNodeId) ?? null, [nodes, selectedNodeId]);
  const selectedStatus = selectedNode ? effectiveStatus(selectedNode) : 'Ready';

  // 组件定义：仅在选中节点 / codeName 变化时拉取。
  const selectedCodeName = selectedNode?.codeName;
  const selectedNodeRef = useRef(selectedNode);
  useEffect(() => {
    selectedNodeRef.current = selectedNode;
  });
  useEffect(() => {
    const node = selectedNodeRef.current;
    if (!node || !onGetComponentDef) {
      setComponentDef(null);
      return;
    }
    let cancelled = false;
    onGetComponentDef(node)
      .then((def) => {
        if (!cancelled) setComponentDef(def);
      })
      .catch(() => {
        if (!cancelled) setComponentDef(null);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedNodeId, selectedCodeName, onGetComponentDef]);

  /* ------------------------------------------------------------------------ */
  /* 撤销/重做                                                                 */
  /* ------------------------------------------------------------------------ */
  const pushHistory = useCallback(() => {
    undoStackRef.current.push({ nodes: [...nodesRef.current], edges: [...edgesRef.current] });
    if (undoStackRef.current.length > 50) undoStackRef.current.shift();
    redoStackRef.current = [];
    setCanUndo(true);
    setCanRedo(false);
  }, []);

  const handleUndo = useCallback(() => {
    if (undoStackRef.current.length === 0) return;
    const prev = undoStackRef.current.pop()!;
    redoStackRef.current.push({ nodes: [...nodesRef.current], edges: [...edgesRef.current] });
    markChange('undo');
    setNodes(prev.nodes);
    setEdges(prev.edges);
    setCanUndo(undoStackRef.current.length > 0);
    setCanRedo(true);
  }, []);

  const handleRedo = useCallback(() => {
    if (redoStackRef.current.length === 0) return;
    const next = redoStackRef.current.pop()!;
    undoStackRef.current.push({ nodes: [...nodesRef.current], edges: [...edgesRef.current] });
    markChange('redo');
    setNodes(next.nodes);
    setEdges(next.edges);
    setCanRedo(redoStackRef.current.length > 0);
    setCanUndo(true);
  }, []);

  const snapValue = useCallback((v: number) => (snapToGrid ? Math.round(v / GRID_SIZE) * GRID_SIZE : v), [snapToGrid]);

  /** 屏幕坐标 → 画布世界坐标（考虑平移与缩放）。 */
  const toWorld = useCallback(
    (clientX: number, clientY: number) => {
      const rect = canvasRef.current?.getBoundingClientRect();
      const scale = zoom / 100;
      const left = rect?.left ?? 0;
      const top = rect?.top ?? 0;
      return { x: (clientX - left - panOffset.x) / scale, y: (clientY - top - panOffset.y) / scale };
    },
    [zoom, panOffset],
  );

  /* ------------------------------------------------------------------------ */
  /* 节点操作                                                                   */
  /* ------------------------------------------------------------------------ */

  const handleSelectNode = (node: DAGNode | null) => {
    setSelectedNodeId(node?.id ?? null);
    setActiveTab('config');
    setLogs({ logs: [] });
    setCloudLogs({ logs: [] });
    setOutput(null);
    setSelectedOutputId(undefined);
    setBinningData(null);
    setModelParams(null);
    setModificationSource(undefined);
    onNodeSelect?.(node);
  };

  const closeInspector = () => {
    handleSelectNode(null);
    setSelectedNodeIds(new Set());
  };

  const handleDeleteNodes = (ids: string[]) => {
    if (readOnly || ids.length === 0) return;
    pushHistory();
    markChange('delete');
    const idSet = new Set(ids);
    const remainingEdges = edgesRef.current.filter((e) => !idSet.has(e.source) && !idSet.has(e.target));
    setEdges(remainingEdges);
    setNodes((prev) =>
      prev
        .filter((n) => !idSet.has(n.id))
        .map((n) => (edgesRef.current.some((e) => e.target === n.id && idSet.has(e.source)) ? { ...n, inputs: deriveInputs(n.id, remainingEdges) } : n)),
    );
    if (selectedNodeId && idSet.has(selectedNodeId)) handleSelectNode(null);
    setSelectedNodeIds(new Set());
  };

  const handleDeleteNode = (nodeId: string) => handleDeleteNodes([nodeId]);

  /** 复制节点（重新分配 ID、改写锚点、参数修改类组件清空 nodeDef）。 */
  const cloneNodes = async (sources: DAGNode[], offset = 40): Promise<DAGNode[]> => {
    const ids = allocateNodeIds ? await allocateNodeIds(sources.length) : sources.map((s, i) => `${s.category}-${s.name}-${Date.now().toString(36)}-${i}`);
    return sources.map((src, i) => {
      const id = ids[i];
      const rewrite = (a: string) => (a.startsWith(`${src.id}-`) ? `${id}-${a.slice(src.id.length + 1)}` : a);
      return {
        ...src,
        id,
        x: src.x + offset,
        y: src.y + offset,
        status: 'Ready',
        progress: undefined,
        inputs: [],
        outputs: (src.outputs || []).map(rewrite),
        ports: (src.ports || []).map((p) => ({ ...p, id: rewrite(p.id) })),
        resultOutputs: (src.resultOutputs || []).map((o) => ({ ...o, id: rewrite(o.id) })),
        nodeDef: isModificationNode(src.codeName) ? clearNodeDefAttrs(src.nodeDef) : src.nodeDef,
      };
    });
  };

  const handlePasteNode = async () => {
    const src = clipboardRef.current;
    if (!src || src.length === 0 || readOnly) return;
    const created = await cloneNodes(src);
    pushHistory();
    markChange('paste');
    setNodes((prev) => [...prev, ...created]);
    handleSelectNode(created[0]);
  };

  const handleDuplicateNode = async (node: DAGNode) => {
    if (readOnly) return;
    const [created] = await cloneNodes([node]);
    pushHistory();
    markChange('paste');
    setNodes((prev) => [...prev, created]);
    handleSelectNode(created);
  };

  const handleCopyNode = (node: DAGNode) => {
    const multi = selectedNodeIds.size > 1 ? nodes.filter((n) => selectedNodeIds.has(n.id)) : [node];
    clipboardRef.current = multi.map((n) => ({ ...n }));
    setHasClipboard(true);
  };

  /* 键盘快捷键：Delete/Cmd+C/V/D/Z。 */
  const keyHandlerRef = useRef<(e: KeyboardEvent) => void>(() => undefined);
  useEffect(() => {
    keyHandlerRef.current = (e: KeyboardEvent) => {
      if (readOnly) return;
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (e.target as HTMLElement)?.isContentEditable) return;
      const isMeta = e.metaKey || e.ctrlKey;
      if (e.key === 'Delete' || e.key === 'Backspace') {
        const ids = selectedNodeIds.size > 0 ? [...selectedNodeIds] : selectedNode ? [selectedNode.id] : [];
        if (ids.length > 0) {
          e.preventDefault();
          handleDeleteNodes(ids);
        }
        return;
      }
      if (isMeta && e.key === 'c' && selectedNode) {
        e.preventDefault();
        handleCopyNode(selectedNode);
        return;
      }
      if (isMeta && e.key === 'v' && clipboardRef.current) {
        e.preventDefault();
        void handlePasteNode();
        return;
      }
      if (isMeta && e.key === 'd' && selectedNode) {
        e.preventDefault();
        void handleDuplicateNode(selectedNode);
        return;
      }
      if (isMeta && e.key === 'z' && !e.shiftKey) {
        e.preventDefault();
        handleUndo();
        return;
      }
      if (isMeta && (e.key === 'Z' || e.key === 'y')) {
        e.preventDefault();
        handleRedo();
      }
    };
  });
  useEffect(() => {
    const handler = (e: KeyboardEvent) => keyHandlerRef.current(e);
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  const handleNodeMouseDown = (e: React.MouseEvent, node: DAGNode) => {
    if (readOnly || e.button !== 0) return;
    e.stopPropagation();
    const w = toWorld(e.clientX, e.clientY);
    setDrag({ nodeId: node.id, offsetX: w.x - node.x, offsetY: w.y - node.y, startX: e.clientX, startY: e.clientY, moved: false });
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (linking) {
      const w = toWorld(e.clientX, e.clientY);
      setLinking({ ...linking, x: w.x, y: w.y });
      return;
    }
    if (!drag) return;
    const moved = drag.moved || Math.abs(e.clientX - drag.startX) > DRAG_THRESHOLD || Math.abs(e.clientY - drag.startY) > DRAG_THRESHOLD;
    if (!moved) return;
    if (!drag.moved) {
      pushHistory();
      setDrag({ ...drag, moved: true });
    }
    const w = toWorld(e.clientX, e.clientY);
    const x = Math.max(0, w.x - drag.offsetX);
    const y = Math.max(0, w.y - drag.offsetY);
    setNodes((prev) => prev.map((n) => (n.id === drag.nodeId ? { ...n, x, y } : n)));
  };

  const handleMouseUp = () => {
    if (linking) {
      setLinking(null);
      return;
    }
    if (!drag) return;
    const current = drag;
    setDrag(null);
    if (!current.moved) return;
    const node = nodesRef.current.find((n) => n.id === current.nodeId);
    if (!node) return;
    const finalNode = { ...node, x: Math.round(snapValue(node.x)), y: Math.round(snapValue(node.y)) };
    markChange('move');
    setNodes((prev) => prev.map((n) => (n.id === finalNode.id ? finalNode : n)));
    void onNodeMove?.(finalNode);
  };

  const buildLocalNode = (component: DAGComponentDef, x: number, y: number): DAGNode => {
    const codeName = `${component.domain}/${component.name}`;
    const label = i18nMap[component.name] || i18nMap[codeName] || component.name;
    return {
      id: `node-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`,
      name: label,
      category: component.domain,
      icon: component.icon || '⚙️',
      status: 'Ready',
      x,
      y,
      codeName,
      nodeDef: { domain: component.domain, name: component.name, version: component.version },
      inputs: [],
      outputs: [],
    };
  };

  const addNode = async (component: DAGComponentDef, pos?: { x: number; y: number }) => {
    if (readOnly) return;
    let newNode: DAGNode;
    if (onAddNode) {
      newNode = await onAddNode(component);
      if (pos) newNode = { ...newNode, x: pos.x, y: pos.y };
    } else {
      newNode = buildLocalNode(component, pos?.x ?? 200 + Math.random() * 120, pos?.y ?? 120 + Math.random() * 80);
    }
    pushHistory();
    markChange('add');
    setNodes((prev) => [...prev, newNode]);
    handleSelectNode(newNode);
    setMobilePaletteOpen(false);
  };

  /* ------------------------------------------------------------------------ */
  /* 连线                                                                       */
  /* ------------------------------------------------------------------------ */

  const portsOf = useCallback((id: string) => nodesRef.current.find((n) => n.id === id)?.ports, []);

  const commitEdge = (edge: DAGEdge, reason: GraphChangeReason = 'connect') => {
    pushHistory();
    markChange(reason);
    const nextEdges = [...edgesRef.current, edge];
    setEdges(nextEdges);
    setNodes((prev) =>
      prev.map((n) => {
        if (n.id === edge.target) {
          const updated = { ...n, inputs: deriveInputs(n.id, nextEdges) };
          // 重连参数修改类组件时清空其配置，保证拿到最新上游（旧版 onEdgeConnected）。
          return isModificationNode(n.codeName) ? { ...updated, nodeDef: clearNodeDefAttrs(n.nodeDef) } : updated;
        }
        if (n.id === edge.source && edge.sourceAnchor && !(n.outputs || []).includes(edge.sourceAnchor) && !(n.ports || []).length) {
          return { ...n, outputs: [...(n.outputs || []), edge.sourceAnchor] };
        }
        return n;
      }),
    );
  };

  const handleConnect = async (sourceId: string, targetId: string, explicit?: { sourceAnchor: string; targetAnchor: string }) => {
    if (readOnly) return;
    const src = nodesRef.current.find((n) => n.id === sourceId);
    const tgt = nodesRef.current.find((n) => n.id === targetId);
    const hasPorts = !!src?.ports?.length && !!tgt?.ports?.length;
    let anchors = explicit;
    if (sourceId === targetId) {
      onConnectionRejected?.('SELF');
      return;
    }
    if (hasPorts) {
      if (anchors) {
        const res = validateConnection({ source: sourceId, target: targetId, ...anchors }, { edges: edgesRef.current, portsOf });
        if (!res.ok) {
          onConnectionRejected?.(res.reason);
          return;
        }
      } else {
        const res = pickConnection(sourceId, targetId, { edges: edgesRef.current, portsOf });
        if (!res.ok) {
          onConnectionRejected?.(res.reason);
          return;
        }
        anchors = { sourceAnchor: res.candidate.sourceAnchor, targetAnchor: res.candidate.targetAnchor };
      }
    } else {
      // 无端口信息（旧数据）：仍做方向 / 重复 / 成环校验；输出锚点取首个输出（修复旧版 outputs.length 越界）。
      const sourceAnchor = explicit?.sourceAnchor ?? src?.outputs?.[0] ?? outputAnchor(sourceId, 0);
      const usedInputs = edgesRef.current.filter((e) => e.target === targetId).map((e) => parseAnchor(e.targetAnchor)?.index ?? -1);
      let idx = 0;
      while (usedInputs.includes(idx)) idx++;
      const targetAnchor = explicit?.targetAnchor ?? inputAnchor(targetId, idx);
      const res = validateConnection({ source: sourceId, target: targetId, sourceAnchor, targetAnchor }, { edges: edgesRef.current, portsOf: () => undefined });
      if (!res.ok) {
        onConnectionRejected?.(res.reason);
        return;
      }
      let hostEdge: DAGEdge | null | undefined;
      if (onConnect) hostEdge = await (explicit ? onConnect(sourceId, targetId, explicit) : onConnect(sourceId, targetId));
      commitEdge(hostEdge ?? { id: edgeIdOf(sourceAnchor, targetAnchor), source: sourceId, target: targetId, sourceAnchor, targetAnchor });
      return;
    }
    const { sourceAnchor, targetAnchor } = anchors!;
    let edge: DAGEdge | null | undefined;
    if (onConnect) edge = await onConnect(sourceId, targetId, { sourceAnchor, targetAnchor });
    commitEdge(edge ?? { id: edgeIdOf(sourceAnchor, targetAnchor), source: sourceId, target: targetId, sourceAnchor, targetAnchor });
  };

  const handleDeleteEdge = (edgeId: string) => {
    if (readOnly) return;
    const edge = edgesRef.current.find((e) => e.id === edgeId);
    if (!edge) return;
    pushHistory();
    markChange('disconnect');
    const nextEdges = edgesRef.current.filter((e) => e.id !== edgeId);
    setEdges(nextEdges);
    setNodes((prev) => prev.map((n) => (n.id === edge.target ? { ...n, inputs: deriveInputs(n.id, nextEdges) } : n)));
  };

  const handleNodeClick = (node: DAGNode) => {
    if (!pendingConnection) {
      handleSelectNode(node);
      return;
    }
    if (!connectSourceId) {
      setConnectSourceId(node.id);
      return;
    }
    if (connectSourceId === node.id) {
      setConnectSourceId(null);
      setPendingConnection(false);
      return;
    }
    void handleConnect(connectSourceId, node.id);
    setConnectSourceId(null);
    setPendingConnection(false);
  };

  /* ------------------------------------------------------------------------ */
  /* 画布：框选 / 平移                                                           */
  /* ------------------------------------------------------------------------ */

  const handleCanvasMouseDown = (e: React.MouseEvent) => {
    if (e.button === 1 || (e.button === 0 && e.altKey)) {
      e.preventDefault();
      setIsPanning(true);
      panStartRef.current = { x: e.clientX, y: e.clientY, panX: panOffset.x, panY: panOffset.y };
      return;
    }
    if (readOnly || pendingConnection || e.button !== 0) return;
    const rect = canvasRef.current?.getBoundingClientRect();
    if (!rect) return;
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    setSelectionRect({ x1: x, y1: y, x2: x, y2: y });
  };

  const handleCanvasMouseMove = (e: React.MouseEvent) => {
    if (isPanning) {
      setPanOffset({ x: panStartRef.current.panX + e.clientX - panStartRef.current.x, y: panStartRef.current.panY + e.clientY - panStartRef.current.y });
      return;
    }
    if (!selectionRect || !canvasRef.current) return;
    const rect = canvasRef.current.getBoundingClientRect();
    const updated = { ...selectionRect, x2: e.clientX - rect.left, y2: e.clientY - rect.top };
    setSelectionRect(updated);
    const scale = zoom / 100;
    const toW = (sx: number, sy: number) => ({ x: (sx - panOffset.x) / scale, y: (sy - panOffset.y) / scale });
    const a = toW(Math.min(updated.x1, updated.x2), Math.min(updated.y1, updated.y2));
    const b = toW(Math.max(updated.x1, updated.x2), Math.max(updated.y1, updated.y2));
    const ids = new Set<string>();
    for (const n of nodes) {
      if (n.x >= a.x && n.x + NODE_WIDTH <= b.x && n.y >= a.y && n.y + NODE_HEIGHT <= b.y) ids.add(n.id);
    }
    setSelectedNodeIds(ids);
  };

  const handleCanvasMouseUp = () => {
    setIsPanning(false);
    if (selectionRect) {
      const dx = Math.abs(selectionRect.x2 - selectionRect.x1);
      const dy = Math.abs(selectionRect.y2 - selectionRect.y1);
      if (dx < 5 && dy < 5) setSelectedNodeIds(new Set());
      setSelectionRect(null);
    }
  };

  const handleCanvasClick = () => {
    if (pendingConnection) return;
    if (!selectionRect) {
      setSelectedNodeIds(new Set());
      setConnectSourceId(null);
      if (selectedNodeId) handleSelectNode(null);
    }
  };

  /* ------------------------------------------------------------------------ */
  /* 检查器：配置 / 日志 / 输出 / 分箱                                            */
  /* ------------------------------------------------------------------------ */

  const handleSaveConfig = async () => {
    if (!selectedNode || !onNodeConfigChange) return;
    const errors = formRef.current?.validate() ?? [];
    if (errors.length > 0) {
      onValidationFailed?.(selectedNode, errors);
      return;
    }
    await onNodeConfigChange(selectedNode);
  };

  const loadLogs = useCallback(
    async (silent = false) => {
      const node = selectedNodeRef.current;
      if (!node || !onNodeLogs) return;
      if (!silent) setPanelLoading(true);
      try {
        const result = await onNodeLogs(node);
        if (Array.isArray(result)) setLogs({ logs: result });
        else setLogs({ status: result?.status, logs: result?.logs || [] });
      } catch {
        /* 保留上次日志 */
      } finally {
        if (!silent) setPanelLoading(false);
      }
    },
    [onNodeLogs],
  );

  const loadCloudLogs = useCallback(async () => {
    const node = selectedNodeRef.current;
    if (!node || !onCloudLogs) return;
    setPanelLoading(true);
    try {
      const result = await onCloudLogs(node);
      if (Array.isArray(result)) setCloudLogs({ logs: result });
      else setCloudLogs({ status: result?.status, logs: result?.logs || [], config: result?.config });
    } catch {
      setCloudLogs({ logs: [] });
    } finally {
      setPanelLoading(false);
    }
  }, [onCloudLogs]);

  const outputChoices = useMemo(() => {
    if (!selectedNode) return [] as Array<{ id: string; name?: string; type?: string }>;
    if (selectedNode.resultOutputs?.length) return selectedNode.resultOutputs;
    return (selectedNode.outputs || []).filter(Boolean).map((id) => ({ id, name: id }));
  }, [selectedNode]);

  const loadOutput = async (outputId?: string) => {
    const node = selectedNodeRef.current;
    if (!node || !onNodeOutput) return;
    setPanelLoading(true);
    try {
      setOutput(await onNodeOutput(node, outputId));
    } catch {
      setOutput(null);
    } finally {
      setPanelLoading(false);
    }
  };

  const isModelParamsNode = (node: DAGNode | null) => (node?.codeName || '') === 'preprocessing/model_param_modifications';

  const loadBinning = async (source = modificationSource) => {
    const node = selectedNodeRef.current;
    if (!node) return;
    setPanelLoading(true);
    try {
      if (isModelParamsNode(node)) {
        if (onNodeModelParams) setModelParams(await onNodeModelParams(node, source));
      } else if (onNodeBinningData) {
        setBinningData(await onNodeBinningData(node, source));
      }
    } finally {
      setPanelLoading(false);
    }
  };

  const isBinningNode = (node: DAGNode | null): boolean => {
    if (!node) return false;
    if (isModelParamsNode(node)) return !!onNodeModelParams;
    const cn = (node.codeName || node.id || '').toLowerCase();
    return !!onNodeBinningData && (cn.includes('binning') || cn.includes('woe') || cn.includes('vert_bin'));
  };

  const handleTabChange = (tab: InspectorTab) => {
    setActiveTab(tab);
    if (tab === 'log') void loadLogs();
    if (tab === 'cloudLog') void loadCloudLogs();
    if (tab === 'output') {
      const first = selectedOutputId ?? outputChoices[0]?.id;
      setSelectedOutputId(first);
      void loadOutput(first);
    }
    if (tab === 'binning') void loadBinning();
  };

  // 节点运行中时日志自动刷新。
  const nodeIsActive = selectedStatus === 'Running' || selectedStatus === 'Pending';
  useEffect(() => {
    if (activeTab !== 'log' || !nodeIsActive || !onNodeLogs) return;
    const timer = setInterval(() => void loadLogs(true), logPollInterval);
    return () => clearInterval(timer);
  }, [activeTab, nodeIsActive, onNodeLogs, logPollInterval, loadLogs, selectedNodeId]);

  const handleConfigJsonChange = (key: 'config' | 'nodeDef', value: string) => {
    if (!selectedNode) return;
    const parsed = safeJsonParse(value);
    if (!parsed) return;
    markChange('config');
    setNodes((prev) => prev.map((n) => (n.id === selectedNode.id ? { ...n, [key]: parsed } : n)));
  };

  const handleNodeDefObjectChange = (nodeDef: Record<string, unknown>, meta?: AttributeFormChangeMeta) => {
    if (!selectedNode) return;
    markChange('config');
    setNodes((prev) =>
      prev.map((n) => (n.id === selectedNode.id ? { ...n, nodeDef, configFinished: meta ? meta.isFinished : n.configFinished } : n)),
    );
  };

  const handleRunSingle = async (node = selectedNode) => {
    if (!onRunSingle || !node) return;
    await onRunSingle(node);
  };
  const handleRunDown = async (node = selectedNode) => {
    if (!onRunDown || !node) return;
    const ids = new Set(downstreamIds(node.id, edges));
    await onRunDown(node, [node, ...nodes.filter((n) => ids.has(n.id))]);
  };
  const handleRunUp = async (node = selectedNode) => {
    if (!onRunUp || !node) return;
    const ids = new Set(upstreamIds(node.id, edges));
    await onRunUp(node, [...nodes.filter((n) => ids.has(n.id)), node]);
  };

  const handleTidyLayout = () => {
    if (readOnly) return;
    pushHistory();
    markChange('layout');
    setNodes(tidyLayout(nodes, edges));
  };

  const handleExportJson = () => {
    const data = JSON.stringify({ nodes, edges }, null, 2);
    if (typeof URL === 'undefined' || !URL.createObjectURL) return;
    const blob = new Blob([data], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${title || 'dag'}-export.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  /**
   * 导入 JSON：节点 ID 按当前图重新分配（宿主 allocateNodeIds → graph/node/max_index），
   * 锚点 / 端口 / 连线随之改写，状态重置为 Ready；标记 import 变更由宿主自动保存持久化。
   */
  const applyImportedGraph = async (data: { nodes?: unknown; edges?: unknown } | null | undefined) => {
    if (!data || readOnly) return;
    const rawNodes = Array.isArray(data.nodes) ? (data.nodes as DAGNode[]).filter((n) => n && typeof n.id === 'string') : [];
    const rawEdges = Array.isArray(data.edges) ? (data.edges as DAGEdge[]).filter((e) => e && typeof e.source === 'string') : [];
    const ids = allocateNodeIds && rawNodes.length > 0 ? await allocateNodeIds(rawNodes.length) : rawNodes.map((n) => n.id);
    const remapped = remapImportedGraph(rawNodes, rawEdges, ids);
    const importedNodes = remapped.nodes.map((n) => ({ ...n, status: 'Ready' as const, progress: undefined, inputs: deriveInputs(n.id, remapped.edges) }));
    pushHistory();
    markChange('import');
    setNodes(importedNodes);
    setEdges(remapped.edges);
    handleSelectNode(null);
  };

  const handleImportJson = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = (ev) => {
        void applyImportedGraph(safeJsonParse(ev.target?.result as string));
      };
      reader.readAsText(file);
    };
    input.click();
  };

  const groups = componentGroups || (components.length > 0 ? { Components: components } : {});

  const handleInterpretFetch = async (component: DAGComponentDef) => {
    if (!onGetComponentDef) return null;
    const codeName = `${component.domain}/${component.name}`;
    const meta = await onGetComponentDef({ id: `__interpreter__${codeName}`, name: component.name, category: component.domain, icon: '⚙️', status: 'Ready', x: 0, y: 0, codeName });
    if (!meta) return null;
    return { desc: meta.desc, version: meta.version ?? component.version, domain: meta.domain ?? component.domain, inputs: meta.inputs, outputs: meta.outputs, attrs: meta.attrs };
  };

  /* ------------------------------------------------------------------------ */
  /* 渲染                                                                       */
  /* ------------------------------------------------------------------------ */

  const renderComponentPalette = () => {
    const paletteFilter = (paletteSearchText || '').toLowerCase();
    const filteredGroups = Object.entries(groups)
      .map(([group, items]) => {
        if (!paletteFilter) return [group, items] as const;
        const filtered = items.filter((c) => {
          const codeName = `${c.domain}/${c.name}`;
          const label = i18nMap[c.name] || i18nMap[codeName] || c.name;
          return String(label).toLowerCase().includes(paletteFilter) || c.name.toLowerCase().includes(paletteFilter) || group.toLowerCase().includes(paletteFilter);
        });
        return [group, filtered] as const;
      })
      .filter(([, items]) => items.length > 0);

    return (
      <div className="flex-1 overflow-y-auto flex flex-col min-h-0">
        <div className="px-2 py-1.5 border-b border-gray-800">
          <input
            type="text"
            value={paletteSearchText}
            onChange={(e) => setPaletteSearchText(e.target.value)}
            placeholder={labels.paletteSearch ?? 'Search operators...'}
            className="w-full px-2 py-1 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[10px] focus:outline-none focus:border-blue-500 placeholder-gray-600"
          />
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-4 text-xs" data-tour="dag-palette">
          {filteredGroups.map(([group, items]) => (
            <div key={group}>
              <button type="button" aria-expanded={!!paletteFilter || !collapsedGroups.has(group)}
                className="w-full px-2 py-1 text-gray-400 font-semibold uppercase text-[10px] cursor-pointer hover:text-gray-300 flex items-center justify-between select-none"
                onClick={() =>
                  setCollapsedGroups((prev) => {
                    const next = new Set(prev);
                    if (next.has(group)) next.delete(group);
                    else next.add(group);
                    return next;
                  })
                }
              >
                <span>{group}</span>
                <span className="text-gray-600">
                  {!paletteFilter && collapsedGroups.has(group) ? '▶' : '▼'} {items.length}
                </span>
              </button>
              {(!!paletteFilter || !collapsedGroups.has(group)) &&
                items.map((component, idx) => {
                  const codeName = `${component.domain}/${component.name}`;
                  const label = formatReactChild(i18nMap[component.name] || i18nMap[codeName] || component.name, component.name);
                  return (
                    <div
                      key={`${component.domain}-${component.name}-${idx}`}
                      draggable={!readOnly}
                      onDragStart={(e) => {
                        e.dataTransfer.setData('application/json', JSON.stringify(component));
                        e.dataTransfer.setData('text/plain', `${component.domain}/${component.name}`);
                      }}
                      className="p-2 mt-1 rounded bg-gray-900 border border-gray-800 hover:border-blue-500 cursor-grab active:cursor-grabbing flex items-center gap-2 transition-all group select-none"
                      title={formatReactChild(component.desc, codeName)}
                    >
                      <div className="flex items-center gap-2 flex-1 min-w-0" onClick={() => void addNode(component)}>
                        <span>{formatReactChild(component.icon, '⚙️')}</span>
                        <span className="truncate">{label}</span>
                      </div>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          setInterpreterComponent(component);
                        }}
                        className="opacity-0 group-hover:opacity-100 text-gray-500 hover:text-blue-400 transition-opacity flex-shrink-0"
                        title={labels.interpretComponent ?? '查看算子详情'}
                      >
                        ℹ️
                      </button>
                    </div>
                  );
                })}
            </div>
          ))}
          {filteredGroups.length === 0 && <div className="text-gray-600 px-2">{labels.noOperators ?? 'No operators available'}</div>}
        </div>
      </div>
    );
  };

  const renderSidebar = () => {
    const tabs = [{ key: 'components', label: labels.components ?? labels.operatorLibrary ?? 'Operator Library' }, ...(sidebarTabs || [])];
    const current = tabs.some((t) => t.key === sidebarTab) ? sidebarTab : 'components';
    return (
      <div className={`${mobilePaletteOpen ? 'absolute inset-y-0 left-0 z-40 flex shadow-2xl' : 'hidden'} lg:static lg:z-auto lg:flex lg:shadow-none w-56 max-w-[80vw] lg:max-w-none bg-gray-950/80 border-r border-gray-800 flex-col min-h-0`}>
        <div className="flex border-b border-gray-800">
          {tabs.map((t) => (
            <button
              key={t.key}
              type="button"
              onClick={() => setSidebarTab(t.key)}
              className={`flex-1 p-2.5 font-semibold text-[10px] uppercase tracking-wider ${current === t.key ? 'text-blue-400 border-b-2 border-blue-500' : 'text-gray-400'}`}
            >
              {t.key === 'components' ? labels.operatorLibrary ?? 'Operator Library' : t.label}
            </button>
          ))}
          <button
            type="button"
            onClick={() => setMobilePaletteOpen(false)}
            className="lg:hidden shrink-0 px-3 text-gray-400 hover:text-gray-200"
            title={labels.closePalette ?? 'Close'}
            aria-label={labels.closePalette ?? 'Close'}
          >
            ✕
          </button>
        </div>
        {current === 'components' ? renderComponentPalette() : <div className="flex-1 overflow-y-auto p-2 text-xs">{sidebarTabs?.find((t) => t.key === current)?.content}</div>}
      </div>
    );
  };

  const renderEdge = (e: DAGEdge) => {
    const srcNode = nodes.find((n) => n.id === e.source);
    const tgtNode = nodes.find((n) => n.id === e.target);
    if (!srcNode || !tgtNode) return null;
    const p1 = anchorPoint(srcNode, e.sourceAnchor, 'output');
    const p2 = anchorPoint(tgtNode, e.targetAnchor, 'input');
    const cx = (p1.x + p2.x) / 2;
    const st = effectiveStatus(srcNode);
    const isRunning = st === 'Running';
    const strokeColor = isRunning ? '#06b6d4' : st === 'Success' ? '#22c55e' : st === 'Failed' ? '#ef4444' : '#3b82f6';
    const d = `M ${p1.x} ${p1.y} C ${cx} ${p1.y}, ${cx} ${p2.y}, ${p2.x} ${p2.y}`;
    return (
      <g key={e.id} data-edge-id={e.id}>
        <path d={d} fill="none" stroke="transparent" strokeWidth={10} className={readOnly ? '' : 'pointer-events-auto cursor-pointer'} onClick={() => handleDeleteEdge(e.id)}>
          <title>{`${e.sourceAnchor ?? e.source} → ${e.targetAnchor ?? e.target}`}</title>
        </path>
        <path d={d} fill="none" stroke={strokeColor} strokeWidth={isRunning ? 2.5 : 2} strokeDasharray={isRunning ? '6 3' : '4 2'} className="pointer-events-none" />
      </g>
    );
  };

  const linkingValidity = (target: DAGNode, port: PortDef): boolean => {
    if (!linking) return false;
    return validateConnection(
      { source: linking.nodeId, target: target.id, sourceAnchor: linking.anchor, targetAnchor: port.id },
      { edges, portsOf: (id) => nodes.find((n) => n.id === id)?.ports },
    ).ok;
  };

  const renderPorts = (node: DAGNode) => {
    const ports = node.ports || [];
    return ports.map((port) => {
      const o = portOffset(port, ports);
      const valid = port.group === 'input' && linking ? linkingValidity(node, port) : false;
      return (
        <span
          key={port.id}
          data-port-id={port.id}
          title={`${port.name ?? port.id}${port.types.length ? ` (${port.types.join(' | ')})` : ''}`}
          className={`absolute w-2.5 h-2.5 rounded-full border-2 z-20 ${
            port.group === 'input'
              ? valid
                ? 'bg-green-400 border-green-200 scale-125'
                : 'bg-gray-900 border-blue-400'
              : 'bg-blue-500 border-blue-200 cursor-crosshair'
          }`}
          style={{ left: o.x - 5, top: o.y - 5 }}
          onMouseDown={(e) => {
            if (readOnly || port.group !== 'output') return;
            e.stopPropagation();
            const w = toWorld(e.clientX, e.clientY);
            setLinking({ nodeId: node.id, anchor: port.id, x: w.x, y: w.y });
          }}
          onMouseUp={(e) => {
            if (!linking || port.group !== 'input') return;
            e.stopPropagation();
            const from = linking;
            setLinking(null);
            void handleConnect(from.nodeId, node.id, { sourceAnchor: from.anchor, targetAnchor: port.id });
          }}
        />
      );
    });
  };

  const renderNode = (node: DAGNode) => {
    const status = effectiveStatus(node);
    const isSelected = selectedNodeId === node.id;
    const isConnectSource = connectSourceId === node.id;
    const isMultiSelected = selectedNodeIds.has(node.id);
    const badge = getStatusBadge(status, labels);
    const q = searchText.trim().toLowerCase();
    const isSearchMatch = q !== '' && [node.name, node.id, node.codeName].some((v) => (v || '').toLowerCase().includes(q));
    const isSearchDimmed = q !== '' && !isSearchMatch;
    const unfinished = node.configFinished === false && status === 'Ready';
    const progress = effectiveProgress(node);
    return (
      <div
        key={node.id}
        data-node-id={node.id}
        onMouseDown={(e) => handleNodeMouseDown(e, node)}
        onMouseEnter={() => setHoveredNodeId(node.id)}
        onMouseLeave={() => setHoveredNodeId((h) => (h === node.id ? null : h))}
        onClick={(e) => {
          e.stopPropagation();
          handleNodeClick(node);
        }}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          handleSelectNode(node);
          setContextMenu({ x: e.clientX, y: e.clientY, node });
        }}
        style={{ left: `${node.x}px`, top: `${node.y}px` }}
        className={`absolute w-36 p-3 rounded-lg bg-gray-950 border-2 shadow-lg transition-colors z-10 select-none group ${
          isSelected
            ? 'border-blue-500 shadow-blue-500/20 ring-2 ring-blue-500/30'
            : isConnectSource
              ? 'border-amber-500 ring-2 ring-amber-500/30'
              : isMultiSelected
                ? 'border-blue-400 ring-1 ring-blue-400/40'
                : status === 'Running' || status === 'Pending'
                  ? 'border-cyan-500 animate-pulse shadow-cyan-500/30'
                  : status === 'Success'
                    ? 'border-green-600/60'
                    : status === 'Failed'
                      ? 'border-red-600/60'
                      : unfinished
                        ? 'border-amber-600/60 border-dashed'
                        : 'border-gray-800 hover:border-gray-700'
        } ${drag?.nodeId === node.id ? 'cursor-grabbing' : readOnly ? 'cursor-default' : 'cursor-grab'} ${isSearchMatch ? 'ring-2 ring-yellow-400/60 border-yellow-500' : ''} ${isSearchDimmed ? 'opacity-30' : ''}`}
      >
        {renderPorts(node)}
        <div className="flex items-center gap-2 mb-1">
          <span className="text-base">{formatReactChild(node.icon, '⚙️')}</span>
          <span className="font-semibold text-xs text-gray-200 truncate">{formatReactChild(node.name, 'Node')}</span>
        </div>
        <div className="flex items-center justify-between text-[10px] text-gray-400">
          <span className="truncate">{unfinished ? <span className="text-amber-500">{labels.unfinished ?? 'Unconfigured'}</span> : node.category}</span>
          <Badge status={badge.status}>
            <span className="text-[9px]">{badge.label}</span>
          </Badge>
        </div>
        {status === 'Running' && (
          <div className="mt-1.5 w-full bg-gray-800 rounded-full h-1 overflow-hidden">
            <div
              className="bg-cyan-500 h-1 rounded-full transition-all duration-500"
              style={{ width: `${typeof progress === 'number' ? Math.min(100, Math.max(0, progress * 100)) : 50}%` }}
            />
          </div>
        )}
        {hoveredNodeId === node.id && !drag && (
          <div className="absolute left-full top-0 ml-2 z-50 pointer-events-none" role="tooltip">
            <div className="bg-gray-950 border border-gray-700 rounded-lg p-2.5 shadow-xl text-[10px] text-gray-300 w-44">
              {node.codeName && <div className="text-gray-500 mb-0.5 break-all">{node.codeName}</div>}
              <div className="text-gray-500 break-all">{node.id}</div>
              {((node.inputs?.length ?? 0) > 0 || (node.outputs?.length ?? 0) > 0) && (
                <div className="text-gray-500 mt-0.5">
                  I/O: {node.inputs?.filter(Boolean).length || 0} in · {node.outputs?.length || 0} out
                </div>
              )}
            </div>
          </div>
        )}
        {!readOnly && (
          <button
            type="button"
            onMouseDown={(e) => e.stopPropagation()}
            onClick={(e) => {
              e.stopPropagation();
              handleDeleteNode(node.id);
            }}
            className="absolute -top-2 -right-2 w-5 h-5 rounded-full bg-red-600 text-white text-[10px] flex items-center justify-center opacity-0 hover:opacity-100 focus:opacity-100 transition-opacity"
            title={labels.deleteNode ?? 'Delete node'}
          >
            ✕
          </button>
        )}
      </div>
    );
  };

  const nodeProvider = selectedNode ? attrDataProviderFor?.(selectedNode, nodes, edges) ?? attrDataProvider : attrDataProvider;
  const translateFn = selectedNode ? attrTranslate?.(selectedNode) : undefined;
  const canContinue = !!selectedNode && !!onContinueRun && CONTINUE_RUN_CODE_NAMES.includes(selectedNode.codeName || '') && (selectedStatus === 'Stopped' || selectedStatus === 'Failed');
  const canStopNode = !!selectedNode && !!onStopNode && (selectedStatus === 'Running' || selectedStatus === 'Pending');
  const tabBtn = (tab: InspectorTab, text: string) => (
    <button
      type="button"
      onClick={() => handleTabChange(tab)}
      className={`flex-1 py-3 text-center border-b-2 ${activeTab === tab ? 'border-blue-500 text-blue-400' : 'border-transparent text-gray-400'}`}
    >
      {text}
    </button>
  );

  const linkingSource = linking ? nodes.find((n) => n.id === linking.nodeId) : undefined;

  return (
    <div className={`flex flex-col bg-gray-900 text-gray-100 overflow-hidden border border-gray-800 shadow-2xl ${isFullscreen ? 'fixed inset-0 z-50 rounded-none' : 'h-full w-full rounded-xl'}`}>
      {/* Top Bar */}
      <div className="min-h-12 py-1.5 bg-gray-950 border-b border-gray-800 px-2 sm:px-4 flex items-center justify-between gap-2 text-xs" data-tour="dag-toolbar">
        <div className="flex items-center gap-2 sm:gap-3 min-w-0 shrink-0 max-w-[45vw] lg:max-w-none">
          <button
            type="button"
            onClick={() => setMobilePaletteOpen((open) => !open)}
            className="lg:hidden shrink-0 p-1.5 rounded bg-gray-800 text-gray-300 hover:text-white"
            title={mobilePaletteOpen ? labels.closePalette ?? 'Close' : labels.openPalette ?? 'Component library'}
            aria-label={mobilePaletteOpen ? labels.closePalette ?? 'Close' : labels.openPalette ?? 'Component library'}
          >
            📚
          </button>
          <span className="font-semibold text-blue-400 truncate">⚡ {title}</span>
          <span className="hidden sm:inline text-gray-600">|</span>
          <span className="hidden sm:inline text-gray-400 whitespace-nowrap shrink-0" title={labels.nodesEdges}>
            {labels.nodesEdges ? `${labels.nodesEdges}: ${nodes.length} / ${edges.length}` : `${nodes.length} nodes · ${edges.length} edges`}
          </span>
          {saveState && saveState !== 'idle' && (
            <span className={`text-[10px] whitespace-nowrap shrink-0 ${saveState === 'dirty' ? 'text-amber-400' : saveState === 'saving' ? 'text-cyan-400' : 'text-green-400'}`}>
              {saveState === 'dirty' ? labels.unsaved ?? 'Unsaved' : saveState === 'saving' ? labels.saving ?? 'Saving…' : labels.saved ?? 'Saved'}
            </span>
          )}
          {showSearch && (
            <input
              type="text"
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              placeholder={labels.searchPlaceholder ?? 'Search nodes...'}
              autoFocus
              className="w-36 px-2 py-0.5 rounded bg-gray-800 border border-gray-700 text-gray-200 text-[10px] focus:outline-none focus:border-blue-500 placeholder-gray-500"
            />
          )}
          <button
            type="button"
            onClick={() => {
              setShowSearch((s) => !s);
              if (showSearch) setSearchText('');
            }}
            className="shrink-0 text-gray-500 hover:text-blue-400 transition-colors text-sm"
            title={labels.search ?? 'Search'}
          >
            {showSearch ? '✕' : '🔍'}
          </button>
        </div>
        <div className="flex items-center gap-2 min-w-0 overflow-x-auto [&>*]:shrink-0">
          {!readOnly && onConnect && (
            <Button
              size="sm"
              variant={pendingConnection ? 'primary' : 'ghost'}
              onClick={() => {
                setPendingConnection((p) => !p);
                setConnectSourceId(null);
              }}
            >
              {pendingConnection ? labels.clickTarget ?? 'Click target' : labels.connect ?? 'Connect'}
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => setZoom((z) => Math.max(50, z - 10))}>
            🔍 -
          </Button>
          <span className="font-mono text-gray-400 text-xs w-10 text-center">{zoom}%</span>
          <Button size="sm" variant="ghost" onClick={() => setZoom((z) => Math.min(150, z + 10))}>
            🔍 +
          </Button>
          <Button size="sm" variant="ghost" onClick={() => { setZoom(100); setPanOffset({ x: 0, y: 0 }); }} title="Reset View">
            🎯
          </Button>
          {!readOnly && (
            <>
              <Button size="sm" variant="ghost" onClick={handleUndo} disabled={!canUndo} title={labels.undo ?? 'Undo (Ctrl+Z)'}>
                ↩️
              </Button>
              <Button size="sm" variant="ghost" onClick={handleRedo} disabled={!canRedo} title={labels.redo ?? 'Redo (Ctrl+Shift+Z)'}>
                ↪️
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setSnapToGrid((s) => !s)}
                title={snapToGrid ? labels.snapOn ?? 'Grid Snap: ON' : labels.snapOff ?? 'Grid Snap: OFF'}
                className={snapToGrid ? 'text-cyan-400' : ''}
              >
                🧲
              </Button>
              <Button size="sm" variant="ghost" onClick={handleTidyLayout} title={labels.tidyLayout ?? 'Tidy Layout'}>
                📐
              </Button>
            </>
          )}
          <Button size="sm" variant="ghost" onClick={() => setIsFullscreen((f) => !f)} title={isFullscreen ? labels.exitFullscreen ?? 'Exit Fullscreen' : labels.fullscreen ?? 'Fullscreen'}>
            {isFullscreen ? '🡐' : '⛶'}
          </Button>
          <Button size="sm" variant="ghost" onClick={handleExportJson} title={labels.exportJson ?? 'Export JSON'}>
            📤
          </Button>
          {!readOnly && (
            <Button size="sm" variant="ghost" onClick={handleImportJson} title={labels.importJson ?? 'Import JSON'}>
              📥
            </Button>
          )}
          {toolbarExtra}
          <div className="h-4 w-px bg-gray-800 mx-1" />
          {onSaveGraph && !readOnly && (
            <Button size="sm" variant="outline" loading={loading} onClick={() => void onSaveGraph(nodes, edges)}>
              💾 {labels.save ?? 'Save'}
            </Button>
          )}
          {onRunGraph && !readOnly && (
            <span data-tour="dag-run">
              <Button size="sm" variant="primary" loading={loading} onClick={() => void onRunGraph(nodes, edges)}>
                ▶ {labels.run ?? 'Run All'}
              </Button>
            </span>
          )}
          {onRunSingle && !readOnly && (
            <Button size="sm" variant="outline" onClick={() => void handleRunSingle()} disabled={!selectedNode} title={labels.runSingle ?? 'Run Single Node'}>
              ▶️
            </Button>
          )}
          {onRunDown && !readOnly && (
            <Button size="sm" variant="outline" onClick={() => void handleRunDown()} disabled={!selectedNode} title={labels.runDown ?? 'Run Downstream'}>
              ⬇️
            </Button>
          )}
          {onRunUp && !readOnly && (
            <Button size="sm" variant="outline" onClick={() => void handleRunUp()} disabled={!selectedNode} title={labels.runUp ?? 'Run Upstream'}>
              ⬆️
            </Button>
          )}
          {onStopGraph && !readOnly && (
            <Button size="sm" variant="danger" onClick={() => void onStopGraph()} title={labels.stop ?? 'Stop'}>
              ⏹ {labels.stop ?? 'Stop'}
            </Button>
          )}
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden relative">
        {(mobilePaletteOpen || selectedNode) && (
          <div
            className="absolute inset-0 z-30 bg-black/50 lg:hidden"
            onClick={() => {
              setMobilePaletteOpen(false);
              if (selectedNode) closeInspector();
            }}
          />
        )}
        {renderSidebar()}
        {centerOverlay}

        {/* Canvas */}
        <div
          ref={canvasRef}
          data-tour="dag-canvas"
          className="flex-1 bg-gray-900 relative overflow-hidden bg-[radial-gradient(#374151_1px,transparent_1px)] [background-size:16px_16px]"
          onMouseMove={(e) => {
            handleMouseMove(e);
            handleCanvasMouseMove(e);
          }}
          onMouseUp={() => {
            handleMouseUp();
            handleCanvasMouseUp();
          }}
          onMouseLeave={() => {
            handleMouseUp();
            handleCanvasMouseUp();
          }}
          onMouseDown={handleCanvasMouseDown}
          onClick={handleCanvasClick}
          onWheel={(e) => {
            if (e.ctrlKey || e.metaKey) {
              e.preventDefault();
              setZoom((z) => Math.min(150, Math.max(50, z + (e.deltaY > 0 ? -5 : 5))));
            }
          }}
          onDragOver={(e) => {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'copy';
          }}
          onDrop={(e) => {
            e.preventDefault();
            if (readOnly) return;
            const raw = e.dataTransfer.getData('application/json');
            if (!raw) return;
            let component: DAGComponentDef;
            try {
              component = JSON.parse(raw);
            } catch (err) {
              console.error('Failed to parse dragged component:', err);
              return;
            }
            if (!component || !component.name || !component.domain) return;
            // 落点换算为世界坐标（考虑缩放 / 平移），节点左上角相对光标偏移 (70, 20)。
            const w = toWorld(e.clientX, e.clientY);
            const pos = { x: Math.max(20, Math.round(w.x - 70)), y: Math.max(20, Math.round(w.y - 20)) };
            void addNode(component, pos).catch((err) => console.error('Failed to add dropped component:', err));
          }}
          style={{ cursor: isPanning ? 'grabbing' : pendingConnection || linking ? 'crosshair' : 'default' }}
        >
          <div className="absolute origin-top-left" style={{ transform: `translate(${panOffset.x}px, ${panOffset.y}px) scale(${zoom / 100})`, width: '100%', height: '100%' }}>
            <svg className="absolute inset-0 w-full h-full pointer-events-none z-0 overflow-visible">
              {edges.map(renderEdge)}
              {linking && linkingSource && (() => {
                const p1 = anchorPoint(linkingSource, linking.anchor, 'output');
                const cx = (p1.x + linking.x) / 2;
                return <path d={`M ${p1.x} ${p1.y} C ${cx} ${p1.y}, ${cx} ${linking.y}, ${linking.x} ${linking.y}`} fill="none" stroke="#f59e0b" strokeWidth={2} strokeDasharray="4 2" />;
              })()}
            </svg>
            {nodes.map(renderNode)}
          </div>

          {nodes.length === 0 && (
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
              <div className="text-center text-gray-500 text-xs">
                <div className="text-3xl mb-2">🗂️</div>
                <div>{labels.emptyCanvas ?? 'Canvas is empty. Add operators from the library on the left.'}</div>
              </div>
            </div>
          )}

          {pendingConnection && (
            <div className="absolute top-2 left-2 px-2 py-1 rounded bg-amber-900/50 text-amber-200 text-[10px] border border-amber-700/50">
              {labels.connectionHint ?? 'Connection mode: click source, then target'}
            </div>
          )}

          {selectionRect && (
            <div
              className="absolute border-2 border-blue-500 bg-blue-500/10 pointer-events-none z-30"
              style={{
                left: Math.min(selectionRect.x1, selectionRect.x2),
                top: Math.min(selectionRect.y1, selectionRect.y2),
                width: Math.abs(selectionRect.x2 - selectionRect.x1),
                height: Math.abs(selectionRect.y2 - selectionRect.y1),
              }}
            />
          )}

          {nodes.length > 0 && (
            <div className="absolute bottom-3 right-3 w-36 h-24 bg-gray-950/90 border border-gray-700 rounded-lg overflow-hidden shadow-lg z-20">
              <svg
                width="144"
                height="96"
                viewBox={`0 0 ${Math.max(800, ...nodes.map((n) => n.x + 200))} ${Math.max(600, ...nodes.map((n) => n.y + 150))}`}
                className="w-full h-full"
              >
                {edges.map((e) => {
                  const src = nodes.find((n) => n.id === e.source);
                  const tgt = nodes.find((n) => n.id === e.target);
                  if (!src || !tgt) return null;
                  return <line key={e.id} x1={src.x + 72} y1={src.y + 32} x2={tgt.x + 72} y2={tgt.y + 32} stroke="#4b5563" strokeWidth="2" />;
                })}
                {nodes.map((n) => {
                  const st = effectiveStatus(n);
                  return (
                    <rect
                      key={n.id}
                      x={n.x}
                      y={n.y}
                      width={144}
                      height={64}
                      rx={8}
                      fill={st === 'Running' ? '#06b6d4' : st === 'Success' ? '#22c55e' : st === 'Failed' ? '#ef4444' : selectedNodeIds.has(n.id) ? '#3b82f6' : '#6b7280'}
                      opacity={0.8}
                    />
                  );
                })}
              </svg>
              <div className="absolute top-0.5 left-1 text-[8px] text-gray-500">Minimap</div>
            </div>
          )}

          {selectedNodeIds.size > 1 && (
            <div className="absolute top-2 left-2 px-2 py-1 rounded bg-blue-900/50 text-blue-200 text-[10px] border border-blue-700/50 z-20">
              {selectedNodeIds.size} selected
            </div>
          )}
        </div>

        {/* Inspector */}
        {selectedNode && (
          <div className="max-lg:absolute max-lg:inset-y-0 max-lg:right-0 max-lg:z-40 max-lg:max-w-[85vw] max-lg:shadow-2xl w-80 bg-gray-950 border-l border-gray-800 flex flex-col" data-tour="dag-inspector">
            <div className="flex border-b border-gray-800 text-xs font-medium">
              {tabBtn('config', labels.parameters ?? 'Parameters')}
              {onNodeLogs && tabBtn('log', labels.logs ?? 'Logs')}
              {onCloudLogs && tabBtn('cloudLog', labels.cloudLogs ?? 'Cloud Logs')}
              {onNodeOutput && tabBtn('output', labels.output ?? 'Output')}
              {isBinningNode(selectedNode) && tabBtn('binning', labels.binningTab ?? 'Binning')}
              <button
                type="button"
                onClick={closeInspector}
                className="lg:hidden shrink-0 px-3 text-gray-400 hover:text-gray-200"
                title={labels.closeInspector ?? 'Close'}
                aria-label={labels.closeInspector ?? 'Close'}
              >
                ✕
              </button>
            </div>

            <div className="p-4 flex-1 overflow-y-auto text-xs space-y-4">
              {activeTab === 'config' && (
                <>
                  <div>
                    <label className="text-gray-400 block mb-1">{labels.nodeIdentifier ?? 'Node Identifier'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-300 truncate">{selectedNode.id}</div>
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.operatorName ?? 'Operator Name'}</label>
                    <input
                      type="text"
                      value={selectedNode.name}
                      onChange={(e) => {
                        const name = e.target.value;
                        markChange('rename');
                        setNodes((prev) => prev.map((n) => (n.id === selectedNode.id ? { ...n, name } : n)));
                      }}
                      disabled={readOnly}
                      className="w-full p-2 rounded bg-gray-900 border border-gray-800 text-gray-200 focus:outline-none focus:border-blue-500 disabled:opacity-50"
                    />
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.codeName ?? 'Code Name'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-400 truncate">{selectedNode.codeName || '-'}</div>
                  </div>

                  {componentDef && (
                    <div className="p-3 rounded bg-gray-900 border border-gray-800 space-y-2">
                      {componentDef.desc && <div className="text-gray-400 text-[10px] leading-relaxed">{translateFn ? translateFn(componentDef.desc) : componentDef.desc}</div>}
                      {componentDef.inputs && componentDef.inputs.length > 0 && (
                        <div>
                          <div className="text-gray-500 text-[10px] font-semibold mb-1">Inputs</div>
                          <div className="text-gray-400 text-[10px] font-mono truncate">{componentDef.inputs.map((i, idx) => i.name || `input-${idx}`).join(', ')}</div>
                        </div>
                      )}
                      {componentDef.outputs && componentDef.outputs.length > 0 && (
                        <div>
                          <div className="text-gray-500 text-[10px] font-semibold mb-1">Outputs</div>
                          <div className="text-gray-400 text-[10px] font-mono truncate">{componentDef.outputs.map((o, idx) => o.name || `output-${idx}`).join(', ')}</div>
                        </div>
                      )}
                      {((componentDef.attrs && componentDef.attrs.length > 0) || (componentDef.inputs || []).some((i) => (i.attrs || []).length > 0)) && (
                        <div className="p-3 rounded bg-gray-950 border border-gray-800">
                          <AttributeForm
                            key={selectedNode.id}
                            ref={formRef}
                            defs={componentDef.attrs}
                            inputs={componentDef.inputs as Array<Record<string, unknown>> | undefined}
                            codeName={selectedNode.codeName}
                            computeMode={computeMode}
                            nodeDef={selectedNode.nodeDef}
                            readOnly={readOnly}
                            onNodeDefChange={handleNodeDefObjectChange}
                            dataProvider={nodeProvider}
                            translate={translateFn}
                            onOpenModification={() => handleTabChange('binning')}
                            labels={{
                              issues: labels.configIssues, validate: labels.validateConfig, valid: labels.configValid,
                              advanced: labels.advancedConfig ?? 'Advanced Config',
                              noAttrs: labels.noAttrs,
                              optional: labels.optional,
                              required: labels.required,
                              none: labels.none,
                              listPlaceholder: labels.listPlaceholder,
                            }}
                          />
                        </div>
                      )}
                    </div>
                  )}

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.executionStatus ?? 'Execution Status'}</label>
                    <div className="flex items-center gap-2 flex-wrap">
                      <Badge status={getStatusBadge(selectedStatus, labels).status}>{getStatusBadge(selectedStatus, labels).label}</Badge>
                      {canStopNode && !readOnly && (
                        <Button size="sm" variant="danger" onClick={() => void onStopNode?.(selectedNode)}>
                          ⏹ {labels.stopNode ?? 'Stop Node'}
                        </Button>
                      )}
                      {canContinue && !readOnly && (
                        <Button size="sm" variant="outline" onClick={() => void onContinueRun?.(selectedNode)}>
                          ⏯ {labels.continueRun ?? 'Continue'}
                        </Button>
                      )}
                    </div>
                    {typeof effectiveProgress(selectedNode) === 'number' && (
                      <div className="mt-2 w-full bg-gray-800 rounded-full h-1.5">
                        <div className="bg-blue-500 h-1.5 rounded-full" style={{ width: `${Math.min(100, Math.max(0, (effectiveProgress(selectedNode) ?? 0) * 100))}%` }} />
                      </div>
                    )}
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.position ?? 'Position (x, y)'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-400">
                      {Math.round(selectedNode.x)}, {Math.round(selectedNode.y)}
                    </div>
                  </div>

                  <details>
                    <summary className="text-gray-400 cursor-pointer">{labels.nodeDef ?? 'NodeDef (JSON)'}</summary>
                    <textarea
                      key={`${selectedNode.id}-nodedef`}
                      defaultValue={safeJsonStringify(selectedNode.nodeDef)}
                      onBlur={(e) => handleConfigJsonChange('nodeDef', e.target.value)}
                      disabled={readOnly}
                      rows={8}
                      className="mt-1 w-full p-2 rounded bg-gray-900 border border-gray-800 text-gray-200 focus:outline-none focus:border-blue-500 font-mono disabled:opacity-50"
                    />
                  </details>

                  {!readOnly && onNodeConfigChange && (
                    <Button size="sm" variant="primary" onClick={() => void handleSaveConfig()} loading={loading}>
                      {labels.applyConfig ?? 'Apply Config'}
                    </Button>
                  )}
                </>
              )}

              {activeTab === 'log' && (
                <div className="space-y-2 h-full flex flex-col min-h-0">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400">
                      {labels.status ?? 'Status'}: {logs.status || '-'}
                      {nodeIsActive && <span className="ml-2 text-cyan-400 text-[10px]">⟳ {labels.autoRefresh ?? 'auto'}</span>}
                    </span>
                    <Button size="sm" variant="ghost" onClick={() => void loadLogs()} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  <div className="flex-1 min-h-0 h-96">
                    <LogViewer
                      logs={logs.logs}
                      loading={panelLoading}
                      emptyText={labels.noLogs ?? 'No logs'}
                      labels={{
                        searchPlaceholder: labels.logSearchPlaceholder,
                        copy: labels.logCopy,
                        copied: labels.logCopied,
                        wrap: labels.logWrap,
                        autoScroll: labels.logAutoScroll,
                        lines: labels.logLines,
                      }}
                    />
                  </div>
                </div>
              )}

              {activeTab === 'cloudLog' && (
                <div className="space-y-2 h-full flex flex-col min-h-0">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400">
                      {labels.status ?? 'Status'}: {cloudLogs.status || '-'}
                    </span>
                    <Button size="sm" variant="ghost" onClick={() => void loadCloudLogs()} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  {cloudLogs.config === false ? (
                    <div role="alert" className="rounded border border-amber-700/50 bg-amber-900/20 px-3 py-2 text-[11px] text-amber-300">
                      {labels.cloudLogNotConfigured ?? '未配置云日志：暂未连接 SLS 工具，请登录容器查看日志。'}{' '}
                      <a className="underline" href="https://help.aliyun.com/zh/sls/getting-started" target="_blank" rel="noreferrer">
                        {labels.cloudLogHelp ?? '查看帮助文档'}
                      </a>
                    </div>
                  ) : (
                    <div className="flex-1 min-h-0 h-96">
                      <LogViewer logs={cloudLogs.logs} loading={panelLoading} emptyText={labels.noLogs ?? 'No logs'} />
                    </div>
                  )}
                </div>
              )}

              {activeTab === 'output' && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    {outputChoices.length > 1 ? (
                      <select
                        aria-label={labels.selectOutput ?? 'Select output'}
                        value={selectedOutputId ?? ''}
                        onChange={(e) => {
                          setSelectedOutputId(e.target.value);
                          void loadOutput(e.target.value);
                        }}
                        className="flex-1 p-1 rounded bg-gray-900 border border-gray-700 text-gray-200 text-[11px]"
                      >
                        {outputChoices.map((o) => (
                          <option key={o.id} value={o.id}>
                            {o.name ? `${o.name} (${o.id})` : o.id}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <span className="text-gray-400">{labels.nodeOutput ?? 'Node Output'}</span>
                    )}
                    <Button size="sm" variant="ghost" onClick={() => void loadOutput(selectedOutputId)} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  <div className="p-2 rounded bg-gray-900 border border-gray-800 text-[11px] text-gray-300 max-h-[32rem] overflow-auto">
                    <ResultVisualization
                      output={output}
                      labels={{ noOutput: labels.noOutput ?? 'No output' }}
                      outputId={selectedOutputId}
                      codeName={selectedNode.codeName}
                      {...(outputViewProps?.(selectedNode, selectedOutputId) || {})}
                    />
                  </div>
                </div>
              )}

              {activeTab === 'binning' && (
                <div className="space-y-2 h-full flex flex-col min-h-0">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400">{labels.binningTab ?? 'Binning Modification'}</span>
                    <Button size="sm" variant="ghost" onClick={() => void loadBinning()} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  {isModelParamsNode(selectedNode) ? (
                    <LinearModelEditor
                      data={modelParams}
                      readOnly={readOnly}
                      source={modificationSource}
                      onSourceChange={(src) => {
                        setModificationSource(src);
                        void loadBinning(src);
                      }}
                      onSave={async (data) => {
                        if (onSaveModelParams) await onSaveModelParams(selectedNode, data);
                      }}
                    />
                  ) : binningData ? (
                    <div className="flex-1 min-h-0">
                      <BinningModification
                        data={binningData}
                        source={modificationSource}
                        onSourceChange={(src) => {
                          setModificationSource(src);
                          void loadBinning(src);
                        }}
                        onMerge={onBinningMerge ? (data) => onBinningMerge(selectedNode, data) : undefined}
                        onSave={async (data) => {
                          if (onSaveBinningData) await onSaveBinningData(selectedNode, data);
                        }}
                        readOnly={readOnly}
                      />
                    </div>
                  ) : (
                    <div className="text-gray-500 text-center py-8">{panelLoading ? 'Loading...' : 'No binning data available'}</div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      <ComponentInterpreter
        component={interpreterComponent}
        fetchMetadata={handleInterpretFetch}
        onClose={() => setInterpreterComponent(null)}
        computeMode={computeMode}
        translate={
          interpreterComponent
            ? attrTranslate?.({
                id: '',
                name: interpreterComponent.name,
                category: interpreterComponent.domain,
                icon: interpreterComponent.icon || '',
                status: 'Ready',
                x: 0,
                y: 0,
                codeName: `${interpreterComponent.domain}/${interpreterComponent.name}`,
                nodeDef: { version: interpreterComponent.version },
              } as DAGNode)
            : undefined
        }
        labels={{
          title: labels.interpreterTitle ?? '组件解释',
          description: labels.interpreterDesc ?? '描述',
          inputs: labels.interpreterInputs ?? '输入',
          outputs: labels.interpreterOutputs ?? '输出',
          attributes: labels.interpreterAttrs ?? '可配置属性',
          loading: labels.interpreterLoading ?? '加载组件定义中...',
          noDefinition: labels.interpreterNoDef ?? '暂无组件定义',
          allowedTypes: labels.interpreterTypes ?? '类型',
        }}
      />

      {contextMenu && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setContextMenu(null)} onContextMenu={(e) => { e.preventDefault(); setContextMenu(null); }} />
          <div className="fixed z-50 min-w-[160px] py-1 rounded-lg bg-gray-900 border border-gray-700 shadow-xl text-xs" style={{ left: contextMenu.x, top: contextMenu.y }}>
            {!readOnly && onRunSingle && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void handleRunSingle(contextMenu.node); }}>
                ▶️ {labels.runSingle ?? 'Run Single'}
              </button>
            )}
            {!readOnly && onRunDown && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void handleRunDown(contextMenu.node); }}>
                ⬇️ {labels.runDown ?? 'Run Downstream'}
              </button>
            )}
            {!readOnly && onRunUp && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void handleRunUp(contextMenu.node); }}>
                ⬆️ {labels.runUp ?? 'Run Upstream'}
              </button>
            )}
            {!readOnly && onStopNode && ['Running', 'Pending'].includes(effectiveStatus(contextMenu.node)) && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void onStopNode(contextMenu.node); }}>
                ⏹ {labels.stopNode ?? 'Stop Node'}
              </button>
            )}
            {!readOnly && onContinueRun && CONTINUE_RUN_CODE_NAMES.includes(contextMenu.node.codeName || '') && ['Stopped', 'Failed'].includes(effectiveStatus(contextMenu.node)) && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void onContinueRun(contextMenu.node); }}>
                ⏯ {labels.continueRun ?? 'Continue'}
              </button>
            )}
            {onNodeLogs && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); handleTabChange('log'); }}>
                📜 {labels.logs ?? 'Logs'}
              </button>
            )}
            {onNodeOutput && (
              <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); handleTabChange('output'); }}>
                📊 {labels.output ?? 'Output'}
              </button>
            )}
            {!readOnly && (
              <>
                <div className="my-1 border-t border-gray-700" />
                <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); handleCopyNode(contextMenu.node); }}>
                  📋 {labels.copy ?? 'Copy'} <span className="text-gray-500 ml-2">⌘C</span>
                </button>
                <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void handleDuplicateNode(contextMenu.node); }}>
                  📄 {labels.duplicate ?? 'Duplicate'} <span className="text-gray-500 ml-2">⌘D</span>
                </button>
                {hasClipboard && (
                  <button type="button" className="w-full px-3 py-1.5 text-left text-gray-300 hover:bg-gray-800" onClick={() => { setContextMenu(null); void handlePasteNode(); }}>
                    📌 {labels.paste ?? 'Paste'} <span className="text-gray-500 ml-2">⌘V</span>
                  </button>
                )}
                <div className="my-1 border-t border-gray-700" />
                <button type="button" className="w-full px-3 py-1.5 text-left text-red-400 hover:bg-red-900/30" onClick={() => { setContextMenu(null); handleDeleteNode(contextMenu.node.id); }}>
                  🗑️ {labels.delete ?? 'Delete'} <span className="text-gray-500 ml-2">Del</span>
                </button>
              </>
            )}
          </div>
        </>
      )}
    </div>
  );
};
