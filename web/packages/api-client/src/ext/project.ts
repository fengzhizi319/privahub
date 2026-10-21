/**
 * Java-contract extension APIs: project (create wizard, project list, guide).
 *
 * Field names follow `services/secretpad/typings.d.ts` of the legacy Java
 * SecretPad (`CreateProjectRequest`, `CreateProjectVO`, `ProjectVO`, `NodeVO`,
 * `AddProjectDatatableRequest`, ...).
 */
import { javaPost } from './core';

/** Java `CreateProjectRequest`. */
export interface CreateProjectRequestJava {
  name: string;
  description?: string;
  /** `MPC` | `TEE` */
  computeMode: string;
  /** TEE node domainId (only for computeMode=TEE). */
  teeNodeId?: string;
  /** `DAG` | `PSI` | `ALL` */
  computeFunc?: string;
}

/** Java `CreateProjectVO`. */
export interface CreateProjectVOJava {
  projectId?: string;
}

/** Java `NodeDatatableVO` (subset). */
export interface NodeDatatableVOJava {
  datatableId?: string;
  datatableName?: string;
}

/** Java `NodeRouteVO` (subset). */
export interface NodeRouteVOJava {
  routeId?: string;
  srcNodeId?: string;
  dstNodeId?: string;
  status?: string;
}

/** Java `NodeVO` (subset used by the create-project wizard / guide node card). */
export interface NodeVOJava {
  nodeId: string;
  nodeName?: string;
  instId?: string;
  instName?: string;
  nodeStatus?: string;
  /** `embedded` | `normal` */
  type?: string;
  mode?: number;
  datatables?: NodeDatatableVOJava[];
  nodeRoutes?: NodeRouteVOJava[];
}

/** Java `ProjectVO` (subset). */
export interface ProjectVOJava {
  projectId: string;
  projectName?: string;
  description?: string;
  computeMode?: string;
  computeFunc?: string;
  teeNodeId?: string;
  graphCount?: number;
  jobCount?: number;
  gmtCreate?: string;
  nodes?: { nodeId?: string; nodeName?: string }[];
}

/** Java `TableColumnConfigParam`. */
export interface TableColumnConfigJava {
  colName: string;
  isAssociateKey?: boolean;
  isGroupKey?: boolean;
  isLabelKey?: boolean;
  isProtection?: boolean;
}

/** Java `AddProjectDatatableRequest`. */
export interface AddProjectDatatableRequestJava {
  projectId: string;
  nodeId: string;
  datatableId: string;
  configs?: TableColumnConfigJava[];
  teeNodeId?: string;
  datasourceId?: string;
  type?: string;
}

function normalizeNode(n: Record<string, unknown>): NodeVOJava {
  return {
    ...(n as object),
    nodeId: String(n.nodeId ?? n.node_id ?? ''),
    nodeName: (n.nodeName ?? n.name ?? n.nodeId) as string | undefined,
    nodeStatus: (n.nodeStatus ?? n.status) as string | undefined,
    datatables: Array.isArray(n.datatables) ? (n.datatables as NodeDatatableVOJava[]) : [],
    nodeRoutes: Array.isArray(n.nodeRoutes) ? (n.nodeRoutes as NodeRouteVOJava[]) : [],
  } as NodeVOJava;
}

function toArray(payload: unknown, keys: string[]): Record<string, unknown>[] {
  if (Array.isArray(payload)) return payload as Record<string, unknown>[];
  const p = (payload || {}) as Record<string, unknown>;
  for (const k of keys) if (Array.isArray(p[k])) return p[k] as Record<string, unknown>[];
  return [];
}

/** POST project/create — Java body `{name, description, computeMode, teeNodeId, computeFunc}`. */
export async function createProjectJava(body: CreateProjectRequestJava): Promise<CreateProjectVOJava> {
  const data = await javaPost<CreateProjectVOJava>('project/create', body);
  return data || {};
}

/** POST project/list → `ProjectVO[]`. */
export async function listProjectsJava(): Promise<ProjectVOJava[]> {
  const data = await javaPost<unknown>('project/list', {});
  return toArray(data, ['list', 'projects', 'data']).map((p) => ({
    ...(p as object),
    projectId: String(p.projectId ?? p.project_id ?? ''),
    projectName: (p.projectName ?? p.name) as string | undefined,
  })) as ProjectVOJava[];
}

/** POST node/list → Java `NodeVO[]` (with `datatables` / `nodeRoutes`). */
export async function listNodesJava(): Promise<NodeVOJava[]> {
  const data = await javaPost<unknown>('node/list', {});
  return toArray(data, ['list', 'nodes', 'data']).map(normalizeNode);
}

/** POST project/tee/list → TEE `NodeVO[]`. */
export async function listTeeNodesJava(): Promise<NodeVOJava[]> {
  const data = await javaPost<unknown>('project/tee/list', {});
  return toArray(data, ['list', 'nodes', 'data']).map(normalizeNode);
}

/** POST project/node/add `{projectId, nodeId}`. */
export async function addProjectNodeJava(projectId: string, nodeId: string): Promise<void> {
  await javaPost('project/node/add', { projectId, nodeId });
}

/** POST project/inst/add `{projectId, instId}`. */
export async function addProjectInstJava(projectId: string, instId: string): Promise<void> {
  await javaPost('project/inst/add', { projectId, instId });
}

/** POST project/datatable/add (Java `AddProjectDatatableRequest`). */
export async function addProjectDatatableJava(body: AddProjectDatatableRequestJava): Promise<void> {
  await javaPost('project/datatable/add', body);
}

/** POST project/update `{projectId, name, description}`. */
export async function updateProjectJava(body: { projectId: string; name?: string; description?: string }): Promise<void> {
  await javaPost('project/update', body);
}

/**
 * POST node/result/list and return only `totalNodeResultNums` — the dashboard
 * uses it as the "results" stat. Returns 0 when the endpoint yields nothing.
 */
export async function countNodeResultsJava(ownerId?: string): Promise<number> {
  // 后端对缺少 ownerId 等畸形请求体返回 202011100，这里直接不发请求。
  if (!ownerId) return 0;
  const data = await javaPost<{ totalNodeResultNums?: number; nodeAllResultsVOList?: unknown[] }>('node/result/list', {
    ownerId,
    pageSize: 1,
    pageNumber: 1,
  });
  return Number(data?.totalNodeResultNums ?? data?.nodeAllResultsVOList?.length ?? 0) || 0;
}
