/**
 * Java-contract extension APIs: data (datasources, datatables, data upload,
 * datatable ↔ project authorisation).
 *
 * Field names mirror the legacy Java contract (`typings.d.ts`):
 * `DataSourceController`, `DatatableController`, `DataController`,
 * `ProjectController` (datatable part) and `P2PProjectController.list`.
 */
import { javaPost, toPage } from './core';

/* ------------------------------------------------------------------ */
/* Constants                                                           */
/* ------------------------------------------------------------------ */

/** Java `DataSourceTypeEnum`. */
export const DatasourceTypes = {
  OSS: 'OSS',
  HTTP: 'HTTP',
  ODPS: 'ODPS',
  LOCAL: 'LOCAL',
  MYSQL: 'MYSQL',
} as const;
export type DatasourceTypeJava = (typeof DatasourceTypes)[keyof typeof DatasourceTypes];

/** Remote datasource types listed on the datasource page (legacy default `types` filter). */
export const REMOTE_DATASOURCE_TYPES: DatasourceTypeJava[] = ['OSS', 'HTTP', 'ODPS', 'MYSQL'];
/** All datasource types a datatable may belong to. */
export const ALL_DATATABLE_SOURCE_TYPES: DatasourceTypeJava[] = ['OSS', 'HTTP', 'LOCAL', 'ODPS', 'MYSQL'];

/** Built-in local datasource id / name (legacy `default-data-source`). */
export const DEFAULT_DATASOURCE_ID = 'default-data-source';
/** Built-in HTTP datasource id (legacy `http-data-source`). */
export const HTTP_DATASOURCE_ID = 'http-data-source';

/** Push-to-TEE status (`DatatableVO.pushToTeeStatus`). */
export const PushToTeeStatus = { RUNNING: 'RUNNING', SUCCESS: 'SUCCESS', FAILED: 'FAILED' } as const;

/* ------------------------------------------------------------------ */
/* Datasource                                                          */
/* ------------------------------------------------------------------ */

export interface DatasourceRelatedNodeJava {
  nodeId?: string;
  nodeName?: string;
  status?: string;
}

export interface DatasourceListInfoJava {
  nodes?: DatasourceRelatedNodeJava[];
  datasourceId?: string;
  name?: string;
  type?: string;
  relatedDatas?: string[];
}

export interface DatasourceListRequestJava {
  page?: number;
  size?: number;
  ownerId?: string;
  name?: string;
  status?: string;
  types?: string[];
}

export interface DatasourceListResultJava {
  infos: DatasourceListInfoJava[];
  total: number;
}

export interface CreateDatasourceRequestJava {
  ownerId?: string;
  nodeIds?: string[];
  type?: string;
  name?: string;
  dataSourceInfo: Record<string, unknown>;
}

export interface CreateDatasourceVOJava {
  datasourceId?: string;
  failedCreatedNodes?: Record<string, unknown>;
}

export interface DatasourceDetailJava {
  nodes?: DatasourceRelatedNodeJava[];
  datasourceId?: string;
  name?: string;
  type?: string;
  status?: string;
  info?: Record<string, unknown>;
}

/** POST datasource/list → `{infos,total}` (Java `DatasourceListVO`). */
export async function listDatasourcesJava(req: DatasourceListRequestJava): Promise<DatasourceListResultJava> {
  const data = await javaPost<{ infos?: DatasourceListInfoJava[]; total?: number } | DatasourceListInfoJava[]>(
    'datasource/list',
    req,
  );
  const page = toPage<DatasourceListInfoJava>(data, ['infos']);
  return { infos: page.list, total: page.total };
}

/** POST datasource/create → `CreateDatasourceVO` (keeps `failedCreatedNodes`). */
export async function createDatasourceJava(req: CreateDatasourceRequestJava): Promise<CreateDatasourceVOJava> {
  return (await javaPost<CreateDatasourceVOJava>('datasource/create', req)) || {};
}

/** POST datasource/delete (`DeleteDatasourceRequest{ownerId,datasourceId,type}`). */
export async function deleteDatasourceJava(req: { ownerId?: string; datasourceId?: string; type?: string }): Promise<void> {
  await javaPost('datasource/delete', req);
}

/** POST datasource/detail (`DatasourceDetailRequest{ownerId,datasourceId,type}`). */
export async function getDatasourceDetailJava(req: {
  ownerId?: string;
  datasourceId?: string;
  type?: string;
}): Promise<DatasourceDetailJava> {
  return (await javaPost<DatasourceDetailJava>('datasource/detail', req)) || {};
}

/** POST datasource/nodes → `DatasourceNodesVO.nodes`. */
export async function getDatasourceNodesJava(req: {
  ownerId?: string;
  datasourceId?: string;
}): Promise<DatasourceRelatedNodeJava[]> {
  const data = await javaPost<{ nodes?: DatasourceRelatedNodeJava[] }>('datasource/nodes', req);
  return data?.nodes || [];
}

/* ------------------------------------------------------------------ */
/* Datatable                                                           */
/* ------------------------------------------------------------------ */

export interface TableColumnJava {
  colName?: string;
  colType?: string;
  colComment?: string;
}

/** Java `TableColumnConfigParam`. */
export interface DatatableColumnConfigJava {
  colName?: string;
  colType?: string;
  colComment?: string;
  isAssociateKey?: boolean;
  isGroupKey?: boolean;
  isLabelKey?: boolean;
  isProtection?: boolean;
}

export interface DatatableAuthProjectJava {
  projectId?: string;
  name?: string;
  computeMode?: string;
  associateKeys?: string[];
  groupKeys?: string[];
  labelKeys?: string[];
  gmtCreate?: string;
}

export interface OdpsPartitionJava {
  type?: string;
  fields?: { name?: string; type?: string; comment?: string }[];
}

/** Java `DatatableVO`. */
export interface DatatableVOJava {
  datatableId?: string;
  datatableName?: string;
  status?: string;
  pushToTeeStatus?: string;
  pushToTeeErrMsg?: string;
  datasourceId?: string;
  datasourceType?: string;
  datasourceName?: string;
  nodeId?: string;
  relativeUri?: string;
  type?: string;
  description?: string;
  schema?: TableColumnJava[];
  authProjects?: DatatableAuthProjectJava[];
  partition?: OdpsPartitionJava;
  nullStrs?: string[];
}

/** Java `DatatableNodeVO`. */
export interface DatatableNodeVOJava {
  datatableVO?: DatatableVOJava;
  nodeName?: string;
  nodeId?: string;
}

/** Flattened row: `DatatableVO` + the owning node (as the legacy list did). */
export type DatatableRowJava = DatatableVOJava & { nodeId?: string; nodeName?: string };

export interface ListDatatableRequestJava {
  pageSize?: number;
  pageNumber?: number;
  nodeNamesFilter?: string[] | null;
  statusFilter?: string;
  datatableNameFilter?: string;
  types?: string[] | null;
  ownerId?: string;
  teeNodeId?: string;
}

/** Java `AllDatatableListVO`. */
export interface AllDatatableListVOJava {
  datatableNodeVOList: DatatableNodeVOJava[];
  totalDatatableNums: number;
}

export interface CreateDatatableRequestJava {
  ownerId?: string;
  nodeIds?: string[];
  datatableName?: string;
  datasourceId?: string;
  datasourceName?: string;
  datasourceType?: string;
  desc?: string;
  relativeUri?: string;
  columns?: TableColumnJava[];
  partition?: OdpsPartitionJava;
  nullStrs?: string[];
  /** Legacy frontend also sent the datasource type as `type`. */
  type?: string;
}

/** Java `CreateDatatableVO` (datatable/create response). */
export interface CreateDatatableVOJava {
  dataTableNodeInfos?: { nodeId?: string; domainDataId?: string }[];
  /** nodeId → failure reason. */
  failedCreatedNodes?: Record<string, unknown>;
}

/** Java `DatatableSchema` (data/create). */
export interface DatatableSchemaJava {
  featureName?: string;
  featureType?: string;
  featureDescription?: string;
}

/** Java `CreateDataRequest`. */
export interface CreateDataRequestJava {
  nodeId?: string;
  name?: string;
  realName?: string;
  tableName?: string;
  description?: string;
  datasourceType?: string;
  datasourceName?: string;
  datatableSchema?: DatatableSchemaJava[];
  nullStrs?: string[];
}

/** Flatten `DatatableNodeVO` into a row carrying nodeId/nodeName. */
export function flattenDatatableNode(item: DatatableNodeVOJava): DatatableRowJava {
  return { ...(item.datatableVO || {}), nodeId: item.nodeId || item.datatableVO?.nodeId, nodeName: item.nodeName };
}

/** POST datatable/list → `AllDatatableListVO`. */
export async function listDatatablesJava(req: ListDatatableRequestJava): Promise<AllDatatableListVOJava> {
  const data = await javaPost<Partial<AllDatatableListVOJava> | DatatableNodeVOJava[]>('datatable/list', req);
  if (Array.isArray(data)) return { datatableNodeVOList: data, totalDatatableNums: data.length };
  const list = data?.datatableNodeVOList || [];
  return { datatableNodeVOList: list, totalDatatableNums: Number(data?.totalDatatableNums ?? list.length) || 0 };
}

/** POST datatable/create → `OssDatatableVO{domainDataId,failedCreatedNodes}`. */
export async function createDatatableJava(req: CreateDatatableRequestJava): Promise<CreateDatatableVOJava> {
  return (await javaPost<CreateDatatableVOJava>('datatable/create', req)) || {};
}

/** POST datatable/get → `DatatableNodeVO`. */
export async function getDatatableJava(req: {
  nodeId?: string;
  datatableId?: string;
  type?: string;
  datasourceType?: string;
  teeNodeId?: string;
}): Promise<DatatableNodeVOJava> {
  return (await javaPost<DatatableNodeVOJava>('datatable/get', req)) || {};
}

/** POST datatable/delete (`DeleteDatatableRequest`). */
export async function deleteDatatableJava(req: {
  nodeId?: string;
  datatableId?: string;
  teeNodeId?: string;
  datasourceId?: string;
  relativeUri?: string;
  type?: string;
  datasourceType?: string;
}): Promise<void> {
  await javaPost('datatable/delete', req);
}

/** POST datatable/pushToTee (`PushDatatableToTeeRequest`). */
export async function pushDatatableToTeeJava(req: {
  nodeId?: string;
  datatableId?: string;
  teeNodeId?: string;
  datasourceId?: string;
  relativeUri?: string;
}): Promise<void> {
  await javaPost('datatable/pushToTee', req);
}

/** POST data/create (`CreateDataRequest`) → created datatable id (string). */
export async function createDataJava(req: CreateDataRequestJava): Promise<string> {
  return (await javaPost<string>('data/create', req)) || '';
}

/* ------------------------------------------------------------------ */
/* Datatable ↔ project authorisation                                  */
/* ------------------------------------------------------------------ */

export interface DatatableAuthRequestJava {
  projectId?: string;
  nodeId?: string;
  datatableId?: string;
  configs?: DatatableColumnConfigJava[];
  teeNodeId?: string;
  datasourceId?: string;
  type?: string;
}

/** POST project/datatable/add (`AddProjectDatatableRequest`). */
export async function authDatatableToProjectJava(req: DatatableAuthRequestJava): Promise<void> {
  await javaPost('project/datatable/add', req);
}

/** POST project/datatable/delete (`DeleteProjectDatatableRequest`). */
export async function cancelDatatableAuthJava(req: {
  projectId?: string;
  nodeId?: string;
  datatableId?: string;
  type?: string;
}): Promise<void> {
  await javaPost('project/datatable/delete', req);
}

/** POST project/update/tableConfig (`AddProjectDatatableRequest`). */
export async function updateProjectTableConfigJava(req: DatatableAuthRequestJava): Promise<void> {
  await javaPost('project/update/tableConfig', req);
}

/**
 * POST project/datatable/get → the project-scoped datatable whose `configs`
 * hold the per-column authorisation (Java returns `Object`; the legacy UI
 * read `data.configs`).
 */
export async function getProjectDatatableConfigJava(req: {
  projectId?: string;
  nodeId?: string;
  datatableId?: string;
  type?: string;
}): Promise<DatatableColumnConfigJava[]> {
  const data = await javaPost<{ configs?: DatatableColumnConfigJava[] }>('project/datatable/get', req);
  return data?.configs || [];
}

export interface AuthorizableProjectJava {
  projectId: string;
  projectName: string;
  computeMode?: string;
  status?: string;
  nodes?: { nodeId?: string; nodeName?: string }[];
}

/**
 * Projects a datatable can be authorised to. P2P/AUTONOMY: `p2p/project/list`
 * filtered to APPROVED projects (optionally containing `nodeId`); otherwise
 * `project/list`.
 */
export async function listAuthorizableProjectsJava(opts: { p2p: boolean; nodeId?: string }): Promise<AuthorizableProjectJava[]> {
  const raw = await javaPost<unknown>(opts.p2p ? 'p2p/project/list' : 'project/list', {});
  const list = toPage<AuthorizableProjectJava>(raw, ['projects']).list;
  const mapped = list.map((p) => ({ ...p, projectId: p.projectId || '', projectName: p.projectName || '' }));
  if (!opts.p2p) return mapped;
  return mapped.filter(
    (p) => p.status === 'APPROVED' && (!opts.nodeId || (p.nodes || []).some((n) => n.nodeId === opts.nodeId)),
  );
}
