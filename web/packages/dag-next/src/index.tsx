import React, { useEffect, useRef, useState } from 'react';
import { Button, Badge } from '@secretpad/design-system';
import { AttributeForm } from './attribute-form';
import { ComponentInterpreter } from './component-interpreter';
import { LogViewer } from './log-viewer';

// 统一导出属性动态表单，供宿主应用（如需要独立使用）引用。
export { AttributeForm, buildAttrTree } from './attribute-form';
export type { AttributeDef, AttributeValue, AttributeFormLabels, AttributeFormProps } from './attribute-form';
// 统一导出组件解释器。
export { ComponentInterpreter } from './component-interpreter';
export type { ComponentInterpreterProps, ComponentInterpreterLabels, InterpreterMetadata, IoPortMeta } from './component-interpreter';
// 统一导出日志查看器。
export { LogViewer } from './log-viewer';
export type { LogViewerProps, LogViewerLabels } from './log-viewer';

export type DAGNodeStatus = 'Ready' | 'Running' | 'Success' | 'Failed' | 'Staging' | 'Stopped';

export interface DAGNode {
  id: string;
  name: string;
  category: string;
  icon: string;
  status: DAGNodeStatus;
  x: number;
  y: number;
  config?: Record<string, any>;
  codeName?: string;
  nodeDef?: Record<string, any>;
  inputs?: string[];
  outputs?: string[];
  progress?: number;
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
  inputs?: Array<{ name?: string; type?: string; desc?: string; types?: string[] }>;
  outputs?: Array<{ name?: string; type?: string; desc?: string; types?: string[] }>;
  attrs?: Array<Record<string, unknown>>;
}

export interface DAGCanvasProps {
  title?: string;
  initialNodes?: DAGNode[];
  initialEdges?: DAGEdge[];
  components?: DAGComponentDef[];
  componentGroups?: Record<string, DAGComponentDef[]>;
  i18nMap?: Record<string, string>;
  readOnly?: boolean;
  loading?: boolean;
  labels?: {
    operatorLibrary?: string;
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
    /** 高级配置抽屉标题。 */
    advancedConfig?: string;
    /** 无属性占位。 */
    noAttrs?: string;
    /** 可选标记。 */
    optional?: string;
    /** 必填标记。 */
    required?: string;
    /** 联合组“未选择”。 */
    none?: string;
    /** 列表输入占位。 */
    listPlaceholder?: string;
    /** 组件解释器标题。 */
    interpreterTitle?: string;
    /** 解释器：描述。 */
    interpreterDesc?: string;
    /** 解释器：输入。 */
    interpreterInputs?: string;
    /** 解释器：输出。 */
    interpreterOutputs?: string;
    /** 解释器：属性。 */
    interpreterAttrs?: string;
    /** 解释器：加载中。 */
    interpreterLoading?: string;
    /** 解释器：无定义。 */
    interpreterNoDef?: string;
    /** 解释器：允许类型。 */
    interpreterTypes?: string;
    /** 算子详情按钮提示。 */
    interpretComponent?: string;
    /** 日志查看器：搜索占位。 */
    logSearchPlaceholder?: string;
    /** 日志查看器：复制。 */
    logCopy?: string;
    /** 日志查看器：已复制。 */
    logCopied?: string;
    /** 日志查看器：自动换行。 */
    logWrap?: string;
    /** 日志查看器：自动滚动。 */
    logAutoScroll?: string;
    /** 日志查看器：行数控件。 */
    logLines?: string;
  };
  onNodeSelect?: (node: DAGNode | null) => void;
  onNodeMove?: (node: DAGNode) => void | Promise<void>;
  onNodeConfigChange?: (node: DAGNode) => void | Promise<void>;
  onNodeLogs?: (node: DAGNode) => Promise<string[] | { status?: string; logs?: string[] }>;
  onNodeOutput?: (node: DAGNode) => Promise<Record<string, any> | null>;
  onGetComponentDef?: (node: DAGNode) => Promise<ComponentMetadata | null>;
  onSaveGraph?: (nodes: DAGNode[], edges: DAGEdge[]) => void | Promise<void>;
  onRunGraph?: (nodes: DAGNode[], edges: DAGEdge[]) => void | Promise<void>;
  onAddNode?: (component: DAGComponentDef) => DAGNode | Promise<DAGNode>;
  onConnect?: (sourceId: string, targetId: string) => DAGEdge | Promise<DAGEdge> | null | undefined;
}

const NODE_WIDTH = 144; // w-36
const NODE_HEIGHT = 64; // approximate

function getStatusBadge(status: DAGNodeStatus): { status: 'success' | 'processing' | 'error' | 'default'; label: string } {
  switch (status) {
    case 'Success':
      return { status: 'success', label: 'Success' };
    case 'Running':
      return { status: 'processing', label: 'Running' };
    case 'Failed':
      return { status: 'error', label: 'Failed' };
    case 'Stopped':
      return { status: 'error', label: 'Stopped' };
    default:
      return { status: 'default', label: 'Ready' };
  }
}

function safeJsonStringify(value: unknown, fallback = '{}'): string {
  try {
    return JSON.stringify(value ?? null, null, 2);
  } catch {
    return fallback;
  }
}

function safeJsonParse(value: string, fallback: Record<string, any> = {}): Record<string, any> {
  try {
    return JSON.parse(value);
  } catch {
    return fallback;
  }
}

function renderOutput(output: Record<string, any> | null, noOutputLabel = 'No output'): React.ReactNode {
  if (!output) return <span className="text-gray-500">{noOutputLabel}</span>;
  if (output.type === 'table' && output.meta && Array.isArray(output.meta.rows)) {
    const rows = output.meta.rows as Record<string, unknown>[];
    const columns = rows.length > 0 ? Object.keys(rows[0]) : [];
    return (
      <div className="space-y-2">
        <div className="text-gray-400 text-[10px]">type: {output.type} · codeName: {output.codeName}</div>
        <div className="overflow-auto">
          <table className="w-full text-left text-[10px] border border-gray-800">
            <thead className="bg-gray-900 text-gray-400">
              <tr>
                {columns.map((col) => (
                  <th key={col} className="p-2 border-b border-gray-800">{col}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, idx) => (
                <tr key={idx} className="border-b border-gray-800 last:border-0">
                  {columns.map((col) => (
                    <td key={col} className="p-2 text-gray-300">{String(row[col] ?? '')}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  }
  if (output.tabs && typeof output.tabs === 'object') {
    const tabs = Object.entries(output.tabs as Record<string, unknown>);
    return (
      <div className="space-y-3">
        {tabs.map(([name, content]) => (
          <div key={name}>
            <div className="text-gray-400 text-[10px] mb-1">{name}</div>
            <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-300 overflow-auto whitespace-pre-wrap">
              {typeof content === 'string' ? content : safeJsonStringify(content)}
            </div>
          </div>
        ))}
      </div>
    );
  }
  return (
    <pre className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-300 h-96 overflow-auto whitespace-pre-wrap">
      {safeJsonStringify(output)}
    </pre>
  );
}

export const DAGNextWorkspace: React.FC<DAGCanvasProps> = ({
  title = 'DAG Pipeline Editor',
  initialNodes = [],
  initialEdges = [],
  components = [],
  componentGroups,
  i18nMap = {},
  readOnly = false,
  loading = false,
  onNodeSelect,
  onNodeMove,
  onNodeConfigChange,
  onNodeLogs,
  onNodeOutput,
  onGetComponentDef,
  onSaveGraph,
  onRunGraph,
  onAddNode,
  onConnect,
  labels = {},
}) => {
  const [nodes, setNodes] = useState<DAGNode[]>(initialNodes);
  const [edges, setEdges] = useState<DAGEdge[]>(initialEdges);
  const [selectedNode, setSelectedNode] = useState<DAGNode | null>(null);
  const [zoom, setZoom] = useState(100);
  const [activeTab, setActiveTab] = useState<'config' | 'log' | 'output'>('config');
  const [isDragging, setIsDragging] = useState(false);
  const [dragNodeId, setDragNodeId] = useState<string | null>(null);
  const [connectSourceId, setConnectSourceId] = useState<string | null>(null);
  const [logs, setLogs] = useState<{ status?: string; logs: string[] }>({ logs: [] });
  const [output, setOutput] = useState<Record<string, any> | null>(null);
  const [componentDef, setComponentDef] = useState<ComponentMetadata | null>(null);
  const [panelLoading, setPanelLoading] = useState(false);
  const [pendingConnection, setPendingConnection] = useState(false);
  /** 组件解释器当前解释的算子（null 表示关闭）。 */
  const [interpreterComponent, setInterpreterComponent] = useState<DAGComponentDef | null>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const dragOffsetRef = useRef({ x: 0, y: 0 });

  useEffect(() => {
    setNodes(initialNodes);
  }, [initialNodes]);

  useEffect(() => {
    setEdges(initialEdges);
  }, [initialEdges]);

  useEffect(() => {
    if (!selectedNode || !onGetComponentDef) {
      setComponentDef(null);
      return;
    }
    let cancelled = false;
    onGetComponentDef(selectedNode)
      .then((def) => {
        if (!cancelled) setComponentDef(def);
      })
      .catch(() => {
        if (!cancelled) setComponentDef(null);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedNode, onGetComponentDef]);

  useEffect(() => {
    if (!selectedNode) return;
    const found = nodes.find((n) => n.id === selectedNode.id);
    if (found) {
      setSelectedNode(found);
    }
  }, [nodes, selectedNode?.id]);

  const handleSelectNode = (node: DAGNode) => {
    setSelectedNode(node);
    setActiveTab('config');
    setLogs({ logs: [] });
    setOutput(null);
    if (onNodeSelect) onNodeSelect(node);
  };

  const handleMouseDown = (e: React.MouseEvent, node: DAGNode) => {
    if (readOnly || !canvasRef.current) return;
    e.stopPropagation();
    const rect = canvasRef.current.getBoundingClientRect();
    dragOffsetRef.current = {
      x: e.clientX - rect.left - node.x,
      y: e.clientY - rect.top - node.y,
    };
    setDragNodeId(node.id);
    setIsDragging(true);
    handleSelectNode(node);
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!isDragging || !dragNodeId || !canvasRef.current) return;
    const rect = canvasRef.current.getBoundingClientRect();
    const x = e.clientX - rect.left - dragOffsetRef.current.x;
    const y = e.clientY - rect.top - dragOffsetRef.current.y;
    setNodes((prev) =>
      prev.map((n) => (n.id === dragNodeId ? { ...n, x: Math.max(0, x), y: Math.max(0, y) } : n))
    );
  };

  const handleMouseUp = async () => {
    if (!isDragging || !dragNodeId) {
      setIsDragging(false);
      setDragNodeId(null);
      return;
    }
    const moved = nodes.find((n) => n.id === dragNodeId);
    if (moved && onNodeMove) {
      await onNodeMove(moved);
    }
    setIsDragging(false);
    setDragNodeId(null);
  };

  const handleAddComponent = async (component: DAGComponentDef) => {
    if (readOnly) return;
    let newNode: DAGNode;
    if (onAddNode) {
      newNode = await onAddNode(component);
    } else {
      const codeName = `${component.domain}/${component.name}`;
      const label = i18nMap[component.name] || i18nMap[codeName] || component.name;
      newNode = {
        id: `node-${Date.now().toString(36)}`,
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
        outputs: [],
      };
    }
    setNodes((prev) => [...prev, newNode]);
    handleSelectNode(newNode);
  };

  const handleCanvasClick = () => {
    if (pendingConnection) return;
    setSelectedNode(null);
    setConnectSourceId(null);
    if (onNodeSelect) onNodeSelect(null);
  };

  const handleNodeClickForConnect = (node: DAGNode) => {
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
    handleConnect(connectSourceId, node.id);
    setConnectSourceId(null);
    setPendingConnection(false);
  };

  const handleConnect = async (sourceId: string, targetId: string) => {
    if (readOnly) return;
    let edge: DAGEdge | null | undefined;
    if (onConnect) {
      edge = await onConnect(sourceId, targetId);
    }
    if (edge === undefined || edge === null) {
      const sourceNode = nodes.find((n) => n.id === sourceId);
      const outputIndex = (sourceNode?.outputs?.length || 0);
      const sourceAnchor = `${sourceId}-output-${outputIndex}`;
      const targetAnchor = `${targetId}-input-${edges.filter((e) => e.target === targetId).length}`;
      edge = {
        id: `edge-${Date.now().toString(36)}`,
        source: sourceId,
        target: targetId,
        sourceAnchor,
        targetAnchor,
      };
    }
    if (!edge) return;
    setEdges((prev) => [...prev, edge!]);
    setNodes((prev) =>
      prev.map((n) => {
        if (n.id === sourceId) {
          return { ...n, outputs: [...(n.outputs || []), edge!.sourceAnchor || `output-${n.outputs?.length || 0}`] };
        }
        if (n.id === targetId) {
          return { ...n, inputs: [...(n.inputs || []), edge!.sourceAnchor || `output-unknown`] };
        }
        return n;
      })
    );
  };

  const handleDeleteEdge = (edgeId: string) => {
    if (readOnly) return;
    const edge = edges.find((e) => e.id === edgeId);
    if (!edge) return;
    setEdges((prev) => prev.filter((e) => e.id !== edgeId));
    setNodes((prev) =>
      prev.map((n) => {
        if (n.id === edge.source) {
          return { ...n, outputs: (n.outputs || []).filter((o) => o !== edge.sourceAnchor) };
        }
        if (n.id === edge.target) {
          return { ...n, inputs: (n.inputs || []).filter((i) => i !== edge.sourceAnchor) };
        }
        return n;
      })
    );
  };

  const handleDeleteNode = (nodeId: string) => {
    if (readOnly) return;
    setNodes((prev) => prev.filter((n) => n.id !== nodeId));
    setEdges((prev) => prev.filter((e) => e.source !== nodeId && e.target !== nodeId));
    if (selectedNode?.id === nodeId) {
      setSelectedNode(null);
    }
  };

  const handleSaveConfig = async () => {
    if (!selectedNode || !onNodeConfigChange) return;
    await onNodeConfigChange(selectedNode);
  };

  const handleLoadLogs = async () => {
    if (!selectedNode || !onNodeLogs) return;
    setPanelLoading(true);
    try {
      const result = await onNodeLogs(selectedNode);
      if (Array.isArray(result)) {
        setLogs({ logs: result });
      } else {
        setLogs({ status: result.status, logs: result.logs || [] });
      }
    } finally {
      setPanelLoading(false);
    }
  };

  const handleLoadOutput = async () => {
    if (!selectedNode || !onNodeOutput) return;
    setPanelLoading(true);
    try {
      const result = await onNodeOutput(selectedNode);
      setOutput(result);
    } finally {
      setPanelLoading(false);
    }
  };

  const handleTabChange = (tab: 'config' | 'log' | 'output') => {
    setActiveTab(tab);
    if (tab === 'log') handleLoadLogs();
    if (tab === 'output') handleLoadOutput();
  };

  const handleRun = async () => {
    if (!onRunGraph) return;
    await onRunGraph(nodes, edges);
  };

  const handleSave = async () => {
    if (!onSaveGraph) return;
    await onSaveGraph(nodes, edges);
  };

  const handleConfigChange = (value: string) => {
    if (!selectedNode) return;
    const parsed = safeJsonParse(value);
    setSelectedNode({ ...selectedNode, config: parsed });
    setNodes((prev) =>
      prev.map((n) => (n.id === selectedNode.id ? { ...n, config: parsed } : n))
    );
  };

  const handleNodeDefChange = (value: string) => {
    if (!selectedNode) return;
    const parsed = safeJsonParse(value);
    setSelectedNode({ ...selectedNode, nodeDef: parsed });
    setNodes((prev) =>
      prev.map((n) => (n.id === selectedNode.id ? { ...n, nodeDef: parsed } : n))
    );
  };

  /**
   * 高级配置抽屉回写入口：AttributeForm 序列化出新的 nodeDef 对象后，
   * 同步更新当前选中节点与画布节点列表（随后由“应用配置”按钮持久化）。
   */
  const handleNodeDefObjectChange = (nodeDef: Record<string, any>) => {
    if (!selectedNode) return;
    setSelectedNode({ ...selectedNode, nodeDef });
    setNodes((prev) =>
      prev.map((n) => (n.id === selectedNode.id ? { ...n, nodeDef } : n))
    );
  };

  const groups = componentGroups || (components.length > 0 ? { Components: components } : {});

  /**
   * 组件解释器的定义拉取回调：复用画布的 onGetComponentDef，
   * 以 codeName 构造一个最小伪节点来查询组件完整定义。
   */
  const handleInterpretFetch = async (component: DAGComponentDef) => {
    if (!onGetComponentDef) return null;
    const codeName = `${component.domain}/${component.name}`;
    const pseudoNode: DAGNode = {
      id: `__interpreter__${codeName}`,
      name: component.name,
      category: component.domain,
      icon: component.icon || '⚙️',
      status: 'Ready',
      x: 0,
      y: 0,
      codeName,
    };
    const meta = await onGetComponentDef(pseudoNode);
    if (!meta) return null;
    return {
      desc: meta.desc,
      version: meta.version ?? component.version,
      domain: meta.domain ?? component.domain,
      inputs: meta.inputs,
      outputs: meta.outputs,
      attrs: meta.attrs,
    };
  };

  const renderComponentPalette = () => (
    <div className="w-56 bg-gray-950/80 border-r border-gray-800 flex flex-col">
      <div className="p-3 border-b border-gray-800 font-semibold text-xs text-gray-400 uppercase tracking-wider">
        {labels.operatorLibrary ?? 'Operator Library'}
      </div>
      <div className="flex-1 overflow-y-auto p-2 space-y-4 text-xs">
        {Object.entries(groups).map(([group, items]) => (
          <div key={group}>
            <div className="px-2 py-1 text-gray-500 font-semibold uppercase text-[10px]">{group}</div>
            {items.map((component, idx) => {
              const codeName = `${component.domain}/${component.name}`;
              const label = i18nMap[component.name] || i18nMap[codeName] || component.name;
              return (
                <div
                  key={`${component.domain}-${component.name}-${idx}`}
                  className="p-2 mt-1 rounded bg-gray-900 border border-gray-800 hover:border-blue-500 hover:bg-gray-850 cursor-pointer flex items-center gap-2 transition-all group"
                  title={component.desc || codeName}
                >
                  {/* 点击主体区域：添加算子到画布 */}
                  <div className="flex items-center gap-2 flex-1 min-w-0" onClick={() => handleAddComponent(component)}>
                    <span>{component.icon || '⚙️'}</span>
                    <span className="truncate">{label}</span>
                  </div>
                  {/* 解释器入口：查看算子定义详情 */}
                  <button
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
        {Object.keys(groups).length === 0 && (
          <div className="text-gray-600 px-2">{labels.noOperators ?? 'No operators available'}</div>
        )}
      </div>
    </div>
  );

  const renderEdge = (e: DAGEdge) => {
    const srcNode = nodes.find((n) => n.id === e.source);
    const tgtNode = nodes.find((n) => n.id === e.target);
    if (!srcNode || !tgtNode) return null;
    const x1 = srcNode.x + NODE_WIDTH;
    const y1 = srcNode.y + NODE_HEIGHT / 2;
    const x2 = tgtNode.x;
    const y2 = tgtNode.y + NODE_HEIGHT / 2;
    const cx = (x1 + x2) / 2;
    return (
      <g key={e.id}>
        <path
          d={`M ${x1} ${y1} C ${cx} ${y1}, ${cx} ${y2}, ${x2} ${y2}`}
          fill="none"
          stroke="#3b82f6"
          strokeWidth="2"
          strokeDasharray="4 2"
          className="pointer-events-auto cursor-pointer hover:stroke-red-500"
          onClick={() => handleDeleteEdge(e.id)}
        />
      </g>
    );
  };

  const renderNode = (node: DAGNode) => {
    const isSelected = selectedNode?.id === node.id;
    const isConnectSource = connectSourceId === node.id;
    const badge = getStatusBadge(node.status);
    return (
      <div
        key={node.id}
        onMouseDown={(e) => handleMouseDown(e, node)}
        onClick={(e) => {
          e.stopPropagation();
          handleNodeClickForConnect(node);
        }}
        style={{ left: `${node.x}px`, top: `${node.y}px` }}
        className={`absolute w-36 p-3 rounded-lg bg-gray-950 border-2 shadow-lg transition-all z-10 select-none ${
          isSelected
            ? 'border-blue-500 shadow-blue-500/20 ring-2 ring-blue-500/30'
            : isConnectSource
            ? 'border-amber-500 ring-2 ring-amber-500/30'
            : 'border-gray-800 hover:border-gray-700'
        } ${isDragging && dragNodeId === node.id ? 'cursor-grabbing' : readOnly ? 'cursor-default' : 'cursor-grab'}`}
      >
        <div className="flex items-center gap-2 mb-1">
          <span className="text-base">{node.icon}</span>
          <span className="font-semibold text-xs text-gray-200 truncate">{node.name}</span>
        </div>
        <div className="flex items-center justify-between text-[10px] text-gray-400">
          <span>{node.category}</span>
          <Badge status={badge.status}>
            <span className="text-[9px]">{badge.label}</span>
          </Badge>
        </div>
        {!readOnly && (
          <button
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

  return (
    <div className="flex flex-col h-full w-full bg-gray-900 text-gray-100 rounded-xl overflow-hidden border border-gray-800 shadow-2xl">
      {/* Canvas Top Bar */}
      <div className="h-12 bg-gray-950 border-b border-gray-800 px-4 flex items-center justify-between text-xs">
        <div className="flex items-center gap-3 min-w-0">
          <span className="font-semibold text-blue-400 truncate">⚡ {title}</span>
          <span className="text-gray-600">|</span>
          <span className="text-gray-400 truncate">{nodes.length} nodes · {edges.length} edges</span>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          {!readOnly && onConnect && (
            <Button
              size="sm"
              variant={pendingConnection ? 'primary' : 'ghost'}
              onClick={() => {
                setPendingConnection((p) => !p);
                setConnectSourceId(null);
              }}
            >
              {pendingConnection ? (labels.clickTarget ?? 'Click target') : (labels.connect ?? 'Connect')}
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => setZoom((z) => Math.max(50, z - 10))}>🔍 -</Button>
          <span className="font-mono text-gray-400 text-xs w-10 text-center">{zoom}%</span>
          <Button size="sm" variant="ghost" onClick={() => setZoom((z) => Math.min(150, z + 10))}>🔍 +</Button>
          <div className="h-4 w-px bg-gray-800 mx-1" />
          {onSaveGraph && (
            <Button size="sm" variant="outline" loading={loading} onClick={handleSave}>
              💾 {labels.save ?? 'Save'}
            </Button>
          )}
          {onRunGraph && (
            <Button size="sm" variant="primary" loading={loading} onClick={handleRun}>
              ▶ {labels.run ?? 'Run'}
            </Button>
          )}
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden relative">
        {renderComponentPalette()}

        {/* Center Canvas Area */}
        <div
          ref={canvasRef}
          className="flex-1 bg-gray-900 relative overflow-hidden bg-[radial-gradient(#374151_1px,transparent_1px)] [background-size:16px_16px]"
          onMouseMove={handleMouseMove}
          onMouseUp={handleMouseUp}
          onMouseLeave={handleMouseUp}
          onClick={handleCanvasClick}
          style={{ cursor: pendingConnection ? 'crosshair' : 'default' }}
        >
          {/* SVG Connecting Edges */}
          <svg className="absolute inset-0 w-full h-full pointer-events-none z-0">
            {edges.map(renderEdge)}
          </svg>

          {/* Render Nodes */}
          {nodes.map(renderNode)}

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
        </div>

        {/* Right Configuration Inspector */}
        {selectedNode && (
          <div className="w-80 bg-gray-950 border-l border-gray-800 flex flex-col">
            <div className="flex border-b border-gray-800 text-xs font-medium">
              <button
                onClick={() => handleTabChange('config')}
                className={`flex-1 py-3 text-center border-b-2 ${activeTab === 'config' ? 'border-blue-500 text-blue-400' : 'border-transparent text-gray-400'}`}
              >
                {labels.parameters ?? 'Parameters'}
              </button>
              {onNodeLogs && (
                <button
                  onClick={() => handleTabChange('log')}
                  className={`flex-1 py-3 text-center border-b-2 ${activeTab === 'log' ? 'border-blue-500 text-blue-400' : 'border-transparent text-gray-400'}`}
                >
                  {labels.logs ?? 'Logs'}
                </button>
              )}
              {onNodeOutput && (
                <button
                  onClick={() => handleTabChange('output')}
                  className={`flex-1 py-3 text-center border-b-2 ${activeTab === 'output' ? 'border-blue-500 text-blue-400' : 'border-transparent text-gray-400'}`}
                >
                  {labels.output ?? 'Output'}
                </button>
              )}
            </div>

            <div className="p-4 flex-1 overflow-y-auto text-xs space-y-4">
              {activeTab === 'config' && (
                <>
                  <div>
                    <label className="text-gray-400 block mb-1">{labels.nodeIdentifier ?? 'Node Identifier'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-300 truncate">
                      {selectedNode.id}
                    </div>
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.operatorName ?? 'Operator Name'}</label>
                    <input
                      type="text"
                      value={selectedNode.name}
                      onChange={(e) => {
                        const name = e.target.value;
                        setSelectedNode({ ...selectedNode, name });
                        setNodes((prev) => prev.map((n) => (n.id === selectedNode.id ? { ...n, name } : n)));
                      }}
                      disabled={readOnly}
                      className="w-full p-2 rounded bg-gray-900 border border-gray-800 text-gray-200 focus:outline-none focus:border-blue-500 disabled:opacity-50"
                    />
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.codeName ?? 'Code Name'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-400 truncate">
                      {selectedNode.codeName || '-'}
                    </div>
                  </div>

                  {componentDef && (
                    <div className="p-3 rounded bg-gray-900 border border-gray-800 space-y-2">
                      {componentDef.desc && (
                        <div className="text-gray-400 text-[10px] leading-relaxed">{componentDef.desc}</div>
                      )}
                      {componentDef.inputs && componentDef.inputs.length > 0 && (
                        <div>
                          <div className="text-gray-500 text-[10px] font-semibold mb-1">Inputs</div>
                          <div className="text-gray-400 text-[10px] font-mono truncate">
                            {componentDef.inputs.map((i, idx) => i.name || `input-${idx}`).join(', ')}
                          </div>
                        </div>
                      )}
                      {componentDef.outputs && componentDef.outputs.length > 0 && (
                        <div>
                          <div className="text-gray-500 text-[10px] font-semibold mb-1">Outputs</div>
                          <div className="text-gray-400 text-[10px] font-mono truncate">
                            {componentDef.outputs.map((o, idx) => o.name || `output-${idx}`).join(', ')}
                          </div>
                        </div>
                      )}
                      {componentDef.attrs && componentDef.attrs.length > 0 && (
                        <div className="p-3 rounded bg-gray-950 border border-gray-800">
                          {/* 高级配置抽屉：按组件属性定义动态生成表单，替代原始 JSON 编辑 */}
                          <AttributeForm
                            key={selectedNode.id}
                            defs={componentDef.attrs}
                            nodeDef={selectedNode.nodeDef}
                            readOnly={readOnly}
                            onNodeDefChange={handleNodeDefObjectChange}
                            labels={{
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
                    <Badge status={getStatusBadge(selectedNode.status).status}>
                      {selectedNode.status}
                    </Badge>
                    {typeof selectedNode.progress === 'number' && (
                      <div className="mt-2 w-full bg-gray-800 rounded-full h-1.5">
                        <div
                          className="bg-blue-500 h-1.5 rounded-full"
                          style={{ width: `${Math.min(100, Math.max(0, selectedNode.progress * 100))}%` }}
                        />
                      </div>
                    )}
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.position ?? 'Position (x, y)'}</label>
                    <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-gray-400">
                      {Math.round(selectedNode.x)}, {Math.round(selectedNode.y)}
                    </div>
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.frontendConfig ?? 'Frontend Config (JSON)'}</label>
                    <textarea
                      value={safeJsonStringify(selectedNode.config)}
                      onChange={(e) => handleConfigChange(e.target.value)}
                      disabled={readOnly}
                      rows={6}
                      className="w-full p-2 rounded bg-gray-900 border border-gray-800 text-gray-200 focus:outline-none focus:border-blue-500 font-mono disabled:opacity-50"
                    />
                  </div>

                  <div>
                    <label className="text-gray-400 block mb-1">{labels.nodeDef ?? 'NodeDef (JSON)'}</label>
                    <textarea
                      value={safeJsonStringify(selectedNode.nodeDef)}
                      onChange={(e) => handleNodeDefChange(e.target.value)}
                      disabled={readOnly}
                      rows={8}
                      className="w-full p-2 rounded bg-gray-900 border border-gray-800 text-gray-200 focus:outline-none focus:border-blue-500 font-mono disabled:opacity-50"
                    />
                  </div>

                  {!readOnly && onNodeConfigChange && (
                    <Button size="sm" variant="primary" onClick={handleSaveConfig} loading={loading}>
                      {labels.applyConfig ?? 'Apply Config'}
                    </Button>
                  )}
                </>
              )}

              {activeTab === 'log' && (
                <div className="space-y-2 h-full flex flex-col min-h-0">
                  {/* 状态 + 手动刷新（LogViewer 内部已提供搜索/筛选/复制等工具栏）。 */}
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400">{labels.status ?? 'Status'}: {logs.status || '-'}</span>
                    <Button size="sm" variant="ghost" onClick={handleLoadLogs} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  {/* Monaco 风格日志查看器：行号 + 级别高亮 + 搜索 + 筛选。 */}
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

              {activeTab === 'output' && (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-gray-400">{labels.nodeOutput ?? 'Node Output'}</span>
                    <Button size="sm" variant="ghost" onClick={handleLoadOutput} loading={panelLoading}>
                      {labels.refresh ?? 'Refresh'}
                    </Button>
                  </div>
                  <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-300 h-96 overflow-auto whitespace-pre-wrap">
                    {renderOutput(output, labels.noOutput)}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* 组件解释器：查看算子定义详情 */}
      <ComponentInterpreter
        component={interpreterComponent}
        fetchMetadata={handleInterpretFetch}
        onClose={() => setInterpreterComponent(null)}
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
    </div>
  );
};
