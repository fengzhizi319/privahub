/**
 * Java-contract extension APIs: node results (result manager list, TEE
 * download approval, batch delete).
 */
import { javaPost } from './core';

/** Java `ListNodeResultRequest`. */
export interface ListNodeResultRequestJava {
  ownerId: string;
  pageSize: number;
  pageNumber: number;
  nodeNamesFilter?: string[] | null;
  kindFilters?: string[];
  dataVendorFilter?: string;
  nameFilter?: string;
  /** 'ascending' | 'descending' | '' */
  timeSortingRule?: string;
  teeNodeId?: string;
}

/** Java `NodeResultsVO` (+ nodeId/nodeName from `NodeAllResultsVO`). */
export interface NodeResultJava {
  domainDataId?: string;
  datasourceId?: string;
  datasourceType?: string;
  productName?: string;
  datatableType?: string;
  sourceProjectId?: string;
  sourceProjectName?: string;
  relativeUri?: string;
  jobId?: string;
  trainFlow?: string;
  pullFromTeeStatus?: string;
  pullFromTeeErrMsg?: string;
  gmtCreate?: string;
  computeMode?: string;
  nodeId?: string;
  nodeName?: string;
}

export interface NodeResultPageJava {
  list: NodeResultJava[];
  total: number;
}

/** node/result/list flattened to rows (legacy result-manager did the same). */
export async function listNodeResultsJava(req: ListNodeResultRequestJava): Promise<NodeResultPageJava> {
  const data = await javaPost<{
    nodeAllResultsVOList?: { nodeId?: string; nodeName?: string; nodeResultsVO?: NodeResultJava }[];
    totalNodeResultNums?: number;
  } | null>('node/result/list', req);
  const rows = (data?.nodeAllResultsVOList || []).map((r) => ({
    ...(r.nodeResultsVO || {}),
    nodeId: r.nodeId,
    nodeName: r.nodeName,
  }));
  return { list: rows, total: Number(data?.totalNodeResultNums ?? rows.length) || 0 };
}

/** Java `PullStatusVO` participants. */
export interface PullStatusPartyJava {
  nodeID?: string;
  nodeName?: string;
  /** NOT_INITIATED | REVIEWING | APPROVED | REJECTED */
  status?: string;
  /** Java PullStatusVO.VoteInfo: {voteID, nodeID, action, reason}. */
  voteInfos?: { voteID?: string; nodeID?: string; action?: string; reason?: string }[];
}

export interface PullStatusJava {
  resourceID?: string;
  resourceType?: string;
  taskID?: string;
  graphID?: string;
  jobID?: string;
  parties?: PullStatusPartyJava[];
}

export function pullTeeStatusJava(req: {
  projectID: string;
  jobID: string;
  taskID?: string;
  resourceID: string;
  resourceType?: string;
}): Promise<PullStatusJava> {
  return javaPost<PullStatusJava>('approval/pull/status', req).then((d) => d || {});
}

/** approval/pull/status 与 TEE_DOWNLOAD 审批允许的资源类型（后端校验）。 */
export type TeeResourceType = 'model' | 'rule' | 'table';

/**
 * 结果 / 输出类型 → TEE 资源类型。后端只接受 model / rule / table；
 * report、read_data 或空类型无法映射，返回 undefined（调用方应隐藏 TEE 申请入口）。
 * 兼容：node/result/list 的 datatableType（table / model / rule / report）、
 * DistData type（sf.table.* / sf.model.* / sf.rule.* / sf.serving.model）以及 dag-next 的 kind。
 */
export function toTeeResourceType(type?: string | null): TeeResourceType | undefined {
  const t = String(type || '').trim().toLowerCase();
  if (!t) return undefined;
  if (t === 'table' || t.startsWith('sf.table')) return 'table';
  if (t === 'model' || t === 'serving' || t.startsWith('sf.model') || t.startsWith('sf.serving')) return 'model';
  if (t === 'rule' || t.startsWith('sf.rule')) return 'rule';
  return undefined;
}

/** Java `TeeDownLoadVoteConfig` (approval/create voteType TEE_DOWNLOAD). */
export interface TeeDownloadVoteConfigJava {
  jobID?: string;
  taskID?: string;
  resourceType?: string;
  resourceID?: string;
  projectID?: string;
  graphID?: string;
}

export function applyTeeDownloadJava(initiatorId: string, voteConfig: TeeDownloadVoteConfigJava): Promise<unknown> {
  // 后端要求 taskID / resourceID / jobID / resourceType / projectID / graphID 全部非空。
  const missing = (['taskID', 'resourceID', 'jobID', 'resourceType', 'projectID', 'graphID'] as const).filter((k) => !voteConfig[k]);
  if (missing.length) return Promise.reject(new Error(`TEE_DOWNLOAD missing ${missing.join(', ')}`));
  return javaPost('approval/create', { initiatorId, voteType: 'TEE_DOWNLOAD', voteConfig });
}

/** datatable/delete (`DeleteDatatableRequest`) — used to delete table results. */
export function deleteResultTableJava(body: {
  nodeId: string;
  datatableId: string;
  datasourceId?: string;
  relativeUri?: string;
  type?: string;
}): Promise<unknown> {
  return javaPost('datatable/delete', body);
}
