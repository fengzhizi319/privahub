/**
 * Java-contract extension APIs: graph / DAG workstream (canvas, node status,
 * outputs, logs, run records, components).
 *
 * Contract truth: legacy `services/secretpad/typings.d.ts`
 * (`GraphController`, `ProjectController` job part, `CloudLogController`).
 * All adapters are tolerant to both the old Go snake_case shapes and the Java
 * camelCase shapes while the backend is being aligned.
 */
import { javaPost } from './core';
import { toJobPage } from './scheduled';
import type { JavaPage } from './core';
import type { ProjectJobSummaryVOJava } from './scheduled';

type Rec = Record<string, unknown>;

const s = (v: unknown): string | undefined => (v === undefined || v === null || v === '' ? undefined : String(v));
const n = (v: unknown): number | undefined => {
  if (v === undefined || v === null || v === '') return undefined;
  const x = Number(v);
  return Number.isNaN(x) ? undefined : x;
};

/* ------------------------------------------------------------------ */
/* Graph detail                                                        */
/* ------------------------------------------------------------------ */

/** Java `GraphNodeDetail`. */
export interface GraphNodeDetailJava {
  graphNodeId: string;
  codeName?: string;
  label?: string;
  x?: number;
  y?: number;
  inputs?: string[];
  outputs?: string[];
  nodeDef?: Rec;
  status?: string;
  jobId?: string;
  taskId?: string;
  progress?: number;
  results?: { kind?: string; refId?: string }[];
  parties?: { nodeId?: string; nodeName?: string }[];
}

export interface GraphEdgeJava {
  edgeId?: string;
  source?: string;
  sourceAnchor?: string;
  target?: string;
  targetAnchor?: string;
}

export interface GraphDataSourceConfigJava {
  editEnable?: boolean;
  nodeId?: string;
  dataSourceId?: string;
  nodeName?: string;
  dataSourceName?: string;
}

/** Java `GraphDetailVO`. */
export interface GraphDetailJava {
  projectId?: string;
  graphId?: string;
  name?: string;
  nodes: GraphNodeDetailJava[];
  edges: GraphEdgeJava[];
  maxParallelism?: number;
  dataSourceConfig?: GraphDataSourceConfigJava[];
}

export function normalizeGraphNode(raw: Rec): GraphNodeDetailJava {
  let nodeDef = raw.nodeDef ?? raw.node_def;
  if (typeof nodeDef === 'string') {
    try {
      nodeDef = JSON.parse(nodeDef);
    } catch {
      nodeDef = undefined;
    }
  }
  const list = (v: unknown): string[] | undefined => {
    if (Array.isArray(v)) return v.map((x) => (x === null || x === undefined ? '' : String(x)));
    if (typeof v === 'string' && v.startsWith('[')) {
      try {
        return (JSON.parse(v) as unknown[]).map(String);
      } catch {
        return undefined;
      }
    }
    return undefined;
  };
  return {
    graphNodeId: String(raw.graphNodeId ?? raw.graph_node_id ?? raw.id ?? ''),
    codeName: s(raw.codeName ?? raw.code_name),
    label: s(raw.label),
    x: n(raw.x),
    y: n(raw.y),
    inputs: list(raw.inputs),
    outputs: list(raw.outputs),
    nodeDef: nodeDef as Rec | undefined,
    status: s(raw.status),
    jobId: s(raw.jobId ?? raw.job_id),
    taskId: s(raw.taskId ?? raw.task_id),
    progress: n(raw.progress ?? raw.statusProcess),
    results: raw.results as GraphNodeDetailJava['results'],
    parties: raw.parties as GraphNodeDetailJava['parties'],
  };
}

export function normalizeGraphDetail(raw: unknown): GraphDetailJava {
  const r = (raw || {}) as Rec;
  const edges = (Array.isArray(r.edges) ? (r.edges as Rec[]) : []).map((e) => ({
    edgeId: s(e.edgeId ?? e.edge_id ?? e.id),
    source: s(e.source),
    sourceAnchor: s(e.sourceAnchor ?? e.source_anchor),
    target: s(e.target),
    targetAnchor: s(e.targetAnchor ?? e.target_anchor),
  }));
  return {
    projectId: s(r.projectId ?? r.project_id),
    graphId: s(r.graphId ?? r.graph_id),
    name: s(r.name),
    nodes: (Array.isArray(r.nodes) ? (r.nodes as Rec[]) : []).map(normalizeGraphNode),
    edges,
    maxParallelism: n(r.maxParallelism ?? r.max_parallelism),
    dataSourceConfig: (r.dataSourceConfig ?? r.data_source_config) as GraphDataSourceConfigJava[] | undefined,
  };
}

export async function getGraphDetailJava(projectId: string, graphId: string): Promise<GraphDetailJava> {
  return normalizeGraphDetail(await javaPost('graph/detail', { projectId, graphId }));
}

/** POST graph/update (Java `FullUpdateGraphRequest`). */
export async function fullUpdateGraphJava(body: {
  projectId: string;
  graphId: string;
  nodes: Rec[];
  edges: GraphEdgeJava[];
  maxParallelism?: number;
  dataSourceConfig?: GraphDataSourceConfigJava[];
}): Promise<void> {
  await javaPost('graph/update', body);
}

/** POST graph/node/update (Java `UpdateGraphNodeRequest`). */
export async function updateGraphNodeJava(body: { projectId: string; graphId: string; node: Rec }): Promise<void> {
  await javaPost('graph/node/update', body);
}

/** POST graph/create → graphId (used for copy pipeline). */
export async function createGraphJava(body: { projectId: string; name: string; nodes?: Rec[]; edges?: GraphEdgeJava[] }): Promise<string> {
  const data = await javaPost<{ graphId?: string; graph_id?: string }>('graph/create', { nodes: [], edges: [], ...body });
  return String(data?.graphId ?? data?.graph_id ?? '');
}

/**
 * POST graph/node/max_index `{projectId, graphId, currentIndex}` → maxIndex.
 * 与服务端约定默认值从 32 开始（旧版 getMaxNodeIndex）。
 */
export async function refreshGraphNodeMaxIndexJava(projectId: string, graphId: string, currentIndex?: number): Promise<number> {
  const data = await javaPost<unknown>('graph/node/max_index', { projectId, graphId, currentIndex });
  const raw = typeof data === 'number' ? data : n((data as Rec | undefined)?.maxIndex ?? (data as Rec | undefined)?.max_index);
  return raw || 32;
}

/* ------------------------------------------------------------------ */
/* Run / status                                                        */
/* ------------------------------------------------------------------ */

/** POST graph/start; `breakpoint: true` = continue run from checkpoint. */
export async function startGraphJava(projectId: string, graphId: string, nodes: string[], breakpoint?: boolean): Promise<string> {
  const data = await javaPost<{ jobId?: string; job_id?: string }>('graph/start', { projectId, graphId, nodes, ...(breakpoint ? { breakpoint } : {}) });
  return String(data?.jobId ?? data?.job_id ?? '');
}

/** POST graph/stop（graphNodeId 为空时停止整张图）。 */
export async function stopGraphNodeJava(projectId: string, graphId: string, graphNodeId?: string): Promise<void> {
  await javaPost('graph/stop', { projectId, graphId, ...(graphNodeId ? { graphNodeId } : {}) });
}

/** Java `GraphNodeStatusVO`. */
export interface GraphNodeStatusJava {
  graphNodeId: string;
  status?: string;
  taskId?: string;
  jobId?: string;
  progress?: number;
  errMsg?: string;
  parties?: { nodeId?: string; nodeName?: string }[];
}

/** Java `GraphStatus`. */
export interface GraphStatusJava {
  finished: boolean;
  jobId?: string;
  nodes: GraphNodeStatusJava[];
}

export function normalizeGraphStatus(raw: unknown): GraphStatusJava {
  const r = (raw || {}) as Rec;
  const nodes = (Array.isArray(r.nodes) ? (r.nodes as Rec[]) : []).map((x) => ({
    graphNodeId: String(x.graphNodeId ?? x.graph_node_id ?? ''),
    status: s(x.status),
    taskId: s(x.taskId ?? x.task_id),
    jobId: s(x.jobId ?? x.job_id),
    progress: n(x.progress ?? x.statusProcess),
    errMsg: s(x.errMsg ?? x.err_msg),
    parties: x.parties as GraphNodeStatusJava['parties'],
  }));
  let finished: boolean;
  if (typeof r.finished === 'boolean') finished = r.finished;
  else {
    const jobStatus = String(r.status ?? '').toUpperCase();
    finished = !['RUNNING', 'INITIALIZED', 'PENDING'].includes(jobStatus);
  }
  return { finished, jobId: s(r.jobId ?? r.job_id), nodes };
}

/** POST graph/node/status → per-node latest task status. */
export async function listGraphNodeStatusJava(projectId: string, graphId: string): Promise<GraphStatusJava> {
  return normalizeGraphStatus(await javaPost('graph/node/status', { projectId, graphId }));
}

/* ------------------------------------------------------------------ */
/* Logs / outputs                                                      */
/* ------------------------------------------------------------------ */

export interface NodeLogsJava {
  status?: string;
  logs: string[];
  config?: boolean;
  nodeParties?: { nodeId?: string; nodeName?: string }[];
}

function toLogs(raw: unknown): NodeLogsJava {
  const r = (raw || {}) as Rec;
  const logs = Array.isArray(r.logs) ? (r.logs as unknown[]).map(String) : typeof r.logs === 'string' ? String(r.logs).split('\n') : [];
  return { status: s(r.status), logs, config: typeof r.config === 'boolean' ? r.config : undefined, nodeParties: r.nodeParties as NodeLogsJava['nodeParties'] };
}

export async function getGraphNodeLogsJava(projectId: string, graphId: string, graphNodeId: string): Promise<NodeLogsJava> {
  return toLogs(await javaPost('graph/node/logs', { projectId, graphId, graphNodeId }));
}

/** POST cloud_log/sls (Java `GraphNodeCloudLogsRequest`). */
export async function getCloudLogsJava(req: { projectId: string; graphNodeId?: string; jobId?: string; taskId?: string; nodeId?: string; queryParties?: boolean }): Promise<NodeLogsJava> {
  return toLogs(await javaPost('cloud_log/sls', req));
}

/** POST graph/node/output → raw `GraphNodeOutputVO` (rendered by dag-next NodeResultView). */
export async function getGraphNodeOutputJava(req: { projectId: string; graphId: string; graphNodeId: string; outputId: string }): Promise<Rec | null> {
  return (await javaPost<Rec>('graph/node/output', req)) ?? null;
}

/* ------------------------------------------------------------------ */
/* Jobs (run records)                                                  */
/* ------------------------------------------------------------------ */

/** POST project/job/list filtered by graph. */
export async function listGraphJobsJava(req: { projectId: string; graphId?: string; pageNum: number; pageSize: number; searchKey?: string; status?: string; startTime?: string; endTime?: string }): Promise<JavaPage<ProjectJobSummaryVOJava>> {
  return toJobPage<ProjectJobSummaryVOJava>(await javaPost('project/job/list', req), req.pageSize);
}

/** Java `ProjectJobVO` with graph snapshot. */
export interface ProjectJobJava {
  jobId?: string;
  status?: string;
  errMsg?: string;
  gmtCreate?: string;
  gmtModified?: string;
  gmtFinished?: string;
  finished?: boolean;
  graph: GraphDetailJava;
}

export function normalizeProjectJob(raw: unknown): ProjectJobJava {
  const r = (raw || {}) as Rec;
  return {
    jobId: s(r.jobId ?? r.job_id),
    status: s(r.status),
    errMsg: s(r.errMsg ?? r.err_msg),
    gmtCreate: s(r.gmtCreate ?? r.gmt_create),
    gmtModified: s(r.gmtModified),
    gmtFinished: s(r.gmtFinished),
    finished: typeof r.finished === 'boolean' ? r.finished : undefined,
    graph: normalizeGraphDetail(r.graph),
  };
}

/** POST project/job/get → job with graph snapshot (record replay). */
export async function getProjectJobJava(projectId: string, jobId: string, graphNodeId?: string): Promise<ProjectJobJava> {
  return normalizeProjectJob(await javaPost('project/job/get', { projectId, jobId, ...(graphNodeId ? { graphNodeId } : {}) }));
}

/** POST project/job/stop. */
export async function stopProjectJobJava(projectId: string, jobId: string): Promise<void> {
  await javaPost('project/job/stop', { projectId, jobId });
}

export async function getJobTaskLogsJava(req: { projectId: string; jobId: string; taskId: string }): Promise<NodeLogsJava> {
  return toLogs(await javaPost('project/job/task/logs', req));
}

export async function getJobTaskOutputJava(req: { projectId: string; jobId: string; taskId: string; outputId: string }): Promise<Rec | null> {
  return (await javaPost<Rec>('project/job/task/output', req)) ?? null;
}

/* ------------------------------------------------------------------ */
/* Project data for the DAG (tables, datasources, outputs)             */
/* ------------------------------------------------------------------ */

export interface ProjectDatatableBaseJava {
  datatableId: string;
  datatableName?: string;
  /** ODPS 分区表（partition.type === 'odps' 且有分区字段）。 */
  isPartitionTable?: boolean;
}

export interface ProjectDetailJava {
  projectId: string;
  projectName?: string;
  computeMode?: string;
  computeFunc?: string;
  teeNodeId?: string;
  status?: string;
  nodes: { nodeId: string; nodeName?: string; nodeType?: string; datatables: ProjectDatatableBaseJava[] }[];
}

const isOdpsPartition = (p: unknown): boolean => {
  const x = p as { type?: string; fields?: unknown } | undefined;
  return x?.type === 'odps' && !!x.fields && (!Array.isArray(x.fields) || x.fields.length > 0);
};

/** POST project/get → ProjectVO with `nodes[].datatables`. */
export async function getProjectDetailJava(projectId: string): Promise<ProjectDetailJava> {
  const r = ((await javaPost<Rec>('project/get', { projectId })) || {}) as Rec;
  const nodes = (Array.isArray(r.nodes) ? (r.nodes as Rec[]) : []).map((x) => ({
    nodeId: String(x.nodeId ?? x.node_id ?? ''),
    nodeName: s(x.nodeName ?? x.node_name ?? x.name),
    nodeType: s(x.nodeType),
    datatables: (Array.isArray(x.datatables) ? (x.datatables as Rec[]) : []).map((d) => ({
      datatableId: String(d.datatableId ?? d.datatable_id ?? ''),
      datatableName: s(d.datatableName ?? d.datatable_name ?? d.name),
      ...(isOdpsPartition(d.partition) ? { isPartitionTable: true } : {}),
    })),
  }));
  return {
    projectId: String(r.projectId ?? projectId),
    projectName: s(r.projectName ?? r.name),
    computeMode: s(r.computeMode ?? r.compute_mode),
    computeFunc: s(r.computeFunc),
    teeNodeId: s(r.teeNodeId),
    status: s(r.status),
    nodes,
  };
}

export interface ProjectTableColumnJava {
  colName: string;
  colType?: string;
  colComment?: string;
  isAssociateKey?: boolean;
  isGroupKey?: boolean;
  isLabelKey?: boolean;
  isProtection?: boolean;
}

/** POST project/datatable/get → `ProjectDatatableVO.configs` (column schema). */
export async function getProjectDatatableColumnsJava(req: { projectId: string; nodeId: string; datatableId: string; type?: string }): Promise<ProjectTableColumnJava[]> {
  const r = ((await javaPost<Rec>('project/datatable/get', { type: 'CSV', ...req })) || {}) as Rec;
  const inner = (r.datatableVO as Rec | undefined) ?? {};
  // Java ProjectDatatableVO: configs 在顶层；兼容 datatableVO.schema / columns。
  const configs = (r.configs ?? inner.configs ?? r.columns ?? inner.schema ?? inner.columns) as Rec[] | undefined;
  return (Array.isArray(configs) ? configs : []).map((c) => ({
    colName: String(c.colName ?? c.col_name ?? c.name ?? c.featureName ?? ''),
    colType: s(c.colType ?? c.col_type ?? c.type ?? c.featureType),
    colComment: s(c.colComment ?? c.featureDescription),
    isAssociateKey: c.isAssociateKey as boolean | undefined,
    isGroupKey: c.isGroupKey as boolean | undefined,
    isLabelKey: c.isLabelKey as boolean | undefined,
    isProtection: c.isProtection as boolean | undefined,
  }));
}

/** POST project/datasource/list → per node datasources (advanced config). */
export async function listProjectGraphDatasourcesJava(projectId: string): Promise<{ nodeId: string; nodeName?: string; dataSources: { dataSourceId: string; dataSourceName?: string; type?: string }[] }[]> {
  const data = await javaPost<unknown>('project/datasource/list', { projectId });
  const arr = Array.isArray(data) ? (data as Rec[]) : (((data as Rec | undefined)?.list as Rec[]) ?? []);
  return arr.map((x) => ({
    nodeId: String(x.nodeId ?? ''),
    nodeName: s(x.nodeName),
    dataSources: (Array.isArray(x.dataSources) ? (x.dataSources as Rec[]) : []).map((d) => ({
      dataSourceId: String(d.dataSourceId ?? d.datasourceId ?? ''),
      dataSourceName: s(d.dataSourceName ?? d.name),
      type: s(d.type),
    })),
  }));
}

/* ------------------------------------------------------------------ */
/* Components                                                          */
/* ------------------------------------------------------------------ */

export interface ComponentSummaryJava {
  app: string;
  domain: string;
  name: string;
  version?: string;
  desc?: string;
}

/**
 * POST component/list. Java: `Map<app, CompListVO{name, desc, version, comps[]}>`;
 * old Go: flat array of components or `{compListVOList}`.
 */
export async function listComponentsJava(): Promise<ComponentSummaryJava[]> {
  const data = await javaPost<unknown>('component/list', {});
  const out: ComponentSummaryJava[] = [];
  const pushComp = (c: Rec, app: string) => {
    const codeName = s(c.code_name ?? c.codeName);
    const domain = s(c.domain) ?? (codeName ? codeName.split('/')[0] : undefined) ?? 'unknown';
    const name = s(c.name) ?? (codeName ? codeName.split('/').slice(1).join('/') : undefined) ?? 'unknown';
    out.push({ app, domain, name, version: s(c.version), desc: s(c.desc ?? c.description) });
  };
  // keyed=true 表示 app 来自 Map 的 key（Java：Map<app, CompListVO>），优先于 CompListVO.name。
  const visit = (v: unknown, app: string, keyed = false) => {
    if (Array.isArray(v)) {
      v.forEach((item) => {
        const it = item as Rec;
        if (Array.isArray(it?.comps)) (it.comps as Rec[]).forEach((c) => pushComp(c, keyed ? app : s(it.app) ?? s(it.name) ?? app));
        else if (it) pushComp(it, app);
      });
    } else if (v && typeof v === 'object') {
      const o = v as Rec;
      if (Array.isArray(o.comps)) (o.comps as Rec[]).forEach((c) => pushComp(c, keyed ? app : s(o.app) ?? s(o.name) ?? app));
      else if (Array.isArray(o.compListVOList)) visit(o.compListVOList, app);
      else if (Array.isArray(o.list)) visit(o.list, app);
      else Object.entries(o).forEach(([k, val]) => visit(val, k, true));
    }
  };
  visit(data, 'secretflow');
  return out;
}

/** POST component/batch → Java: ordered `ComponentDef[]`（无 code_name）；旧 Go: `Map<"domain/name", ComponentDef>`。统一返回按 `domain/name` 索引的映射。 */
export async function batchComponentsJava(requests: { app: string; domain: string; name: string; version?: string }[]): Promise<Record<string, Rec>> {
  const data = await javaPost<unknown>('component/batch', requests);
  if (Array.isArray(data)) {
    const out: Record<string, Rec> = {};
    (data as Rec[]).forEach((d) => {
      const key = `${d.domain}/${d.name}`;
      out[key] = d;
    });
    return out;
  }
  return (data || {}) as Record<string, Rec>;
}

/** POST component/i18n (raw; nested per app in Java, flat in old Go). */
export async function getComponentI18nJava(): Promise<Rec> {
  return ((await javaPost<Rec>('component/i18n', {})) || {}) as Rec;
}
