/**
 * Java-contract extension APIs: node / nodeRoute / inst / p2p node / edge
 * account password. Field names follow the legacy Java SecretPad contract
 * (typings.d.ts `NodeVO`, `NodeRouterVO`, `InstTokenVO`, ...).
 */
import { sessionFetch } from '../session';
import { javaPost, toPage, type JavaPage } from './core';

/** `NodeInstanceDTO`. */
export interface NodeInstanceJava {
  name?: string;
  status?: string;
  version?: string;
  lastHeartbeatTime?: string;
  lastTransitionTime?: string;
}

/** Node resource usage (tolerant; `{name, allocatable, capacity}` as kuscia reports it). */
export interface NodeResourceJava {
  name?: string;
  allocatable?: string;
  capacity?: string;
}

/** Java `NodeVO` (node/get, node/page, inst/node/list, nodeRoute/listNode). */
export interface NodeDetailJava {
  nodeId?: string;
  nodeName?: string;
  instId?: string;
  instName?: string;
  controlNodeId?: string;
  masterNodeId?: string;
  description?: string;
  netAddress?: string;
  /** 'configured' | 'unconfirmed' */
  cert?: string;
  certText?: string;
  nodeAuthenticationCode?: string;
  token?: string;
  tokenStatus?: string;
  nodeRole?: string;
  nodeStatus?: string;
  type?: string;
  mode?: number;
  gmtCreate?: string;
  gmtModified?: string;
  nodeInstances?: NodeInstanceJava[];
  resources?: NodeResourceJava[];
  resultCount?: number;
  protocol?: string;
  instToken?: string;
  allowDeletion?: boolean;
  isMainNode?: boolean;
}

/** Java `NodeTokenVO`. */
export interface NodeTokenJava {
  token?: string;
  tokenStatus?: string;
  lastTransitionTime?: string;
}

/** Java `InstTokenVO`. */
export interface InstTokenJava {
  nodeId?: string;
  nodeName?: string;
  instToken?: string;
  createTime?: string;
}

/** Java `InstVO`. */
export interface InstInfoJava {
  instId?: string;
  instName?: string;
  localNodeId?: string;
}

/** Java `NodeRouterVO`. */
export interface NodeRouterJava {
  routeId?: string;
  srcNodeId?: string;
  dstNodeId?: string;
  srcNode?: NodeDetailJava;
  dstNode?: NodeDetailJava;
  srcNetAddress?: string;
  dstNetAddress?: string;
  status?: string;
  gmtCreate?: string;
  gmtModified?: string;
  isProjectJobRunning?: boolean;
  routeType?: string;
}

/** Java `PageNodeRequest` / `PageNodeRouteRequest`. */
export interface PageNodeRequestJava {
  page: number;
  size: number;
  sort?: Record<string, 'ASC' | 'DESC'>;
  search?: string;
  /** nodeRoute/page only: node whose routes are listed. */
  ownerId?: string;
}

/** Java `P2pCreateNodeRequest`. */
export interface P2pCreateNodeRequestJava {
  name?: string;
  /** Node capability: 0 tee | 1 mpc | 2 tee&mpc (Java default mpc). */
  mode: number;
  masterNodeId?: string;
  certText?: string;
  dstNodeId?: string;
  srcNetAddress?: string;
  srcNodeId?: string;
  dstNetAddress?: string;
  dstInstId?: string;
  dstInstName?: string;
}

/** Java `NodeRouteVoteConfig` (approval/create voteType NODE_ROUTE). */
export interface NodeRouteVoteConfigJava {
  srcNodeId: string;
  desNodeId: string;
  srcNodeAddr: string;
  desNodeAddr: string;
  /** Legacy UI flag: single direction route (not in the Java DTO, ignored there). */
  isSingle?: boolean;
}

/** Java `ResetNodeUserPwdRequest`. */
export interface ResetNodeUserPwdRequestJava {
  nodeId: string;
  name: string;
  passwordHash: string;
  newPasswordHash: string;
}

const asList = <T>(payload: unknown): T[] => (Array.isArray(payload) ? (payload as T[]) : toPage<T>(payload, ['nodes']).list);

// ------------------------------- node -------------------------------

export function pageNodesJava(req: PageNodeRequestJava): Promise<JavaPage<NodeDetailJava>> {
  return javaPost('node/page', req).then((p) => toPage<NodeDetailJava>(p));
}

export function getNodeJava(nodeId: string): Promise<NodeDetailJava> {
  return javaPost<NodeDetailJava>('node/get', { nodeId }).then((n) => n || {});
}

export function createNodeJava(body: { name: string; mode: number }): Promise<string> {
  return javaPost<string>('node/create', body);
}

export function updateNodeJava(body: { nodeId: string; netAddress: string }): Promise<string> {
  return javaPost<string>('node/update', body);
}

export function deleteNodeJava(nodeId: string): Promise<void> {
  return javaPost<void>('node/delete', { nodeId });
}

export function refreshNodeJava(nodeId: string): Promise<NodeDetailJava> {
  return javaPost<NodeDetailJava>('node/refresh', { nodeId }).then((n) => n || {});
}

export function getNodeTokenJava(nodeId: string): Promise<NodeTokenJava> {
  return javaPost<NodeTokenJava>('node/token', { nodeId }).then((n) => n || {});
}

export function newNodeTokenJava(nodeId: string): Promise<NodeTokenJava> {
  return javaPost<NodeTokenJava>('node/newToken', { nodeId }).then((n) => n || {});
}

// ------------------------------- inst -------------------------------

export function getInstJava(): Promise<InstInfoJava> {
  return javaPost<InstInfoJava>('inst/get', {}).then((n) => n || {});
}

export function listMyInstNodesJava(): Promise<NodeDetailJava[]> {
  return javaPost('inst/node/list', {}).then((p) => asList<NodeDetailJava>(p));
}

/** AUTONOMY: add a compute node to the institution (mode fixed to mpc like legacy). */
export function addInstNodeJava(name: string, mode = 1): Promise<InstTokenJava> {
  return javaPost<InstTokenJava>('inst/node/add', { name, mode }).then((n) => n || {});
}

export function deleteInstNodeJava(nodeId: string): Promise<void> {
  return javaPost<void>('inst/node/delete', { nodeId });
}

export function getInstNodeTokenJava(nodeId: string): Promise<InstTokenJava> {
  return javaPost<InstTokenJava>('inst/node/token', { nodeId }).then((n) => n || {});
}

export function newInstNodeTokenJava(nodeId: string): Promise<InstTokenJava> {
  return javaPost<InstTokenJava>('inst/node/newToken', { nodeId }).then((n) => n || {});
}

/**
 * inst/node/register (legacy util.sh post_kuscia_node). Java:
 * `@RequestParam("json_data") String` + multipart `certFile` / `keyFile` / `token`.
 * json_data is sent as a multipart form field (the Go handler reads the form
 * first and only falls back to the query string for older clients).
 */
export async function registerInstNodeJava(
  jsonData: string,
  files: { certFile?: File | null; keyFile?: File | null; token?: File | null },
): Promise<void> {
  const form = new FormData();
  form.append('json_data', jsonData);
  form.append('certFile', files.certFile || new Blob([]), files.certFile?.name || 'client.crt');
  form.append('keyFile', files.keyFile || new Blob([]), files.keyFile?.name || 'client.pem');
  form.append('token', files.token || new Blob([]), files.token?.name || 'token');
  const headers: Record<string, string> = {};
  const response = await sessionFetch('/api/v1alpha1/inst/node/register', { method: 'POST', headers, body: form });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const json = (await response.json()) as { status?: { code: number; msg?: string } };
  if (json.status && json.status.code !== 0) throw new Error(json.status.msg || `API error ${json.status.code}`);
}

// ----------------------------- nodeRoute -----------------------------

export function pageCooperativeRoutesJava(req: PageNodeRequestJava): Promise<JavaPage<NodeRouterJava>> {
  return javaPost('nodeRoute/page', req).then((p) => toPage<NodeRouterJava>(p));
}

export function getNodeRouteJava(routerId: string): Promise<NodeRouterJava> {
  return javaPost<NodeRouterJava>('nodeRoute/get', { routerId }).then((n) => n || {});
}

export function updateNodeRouteJava(body: {
  routerId: string;
  srcNetAddress?: string;
  dstNetAddress?: string;
  routeType?: string;
}): Promise<string> {
  return javaPost<string>('nodeRoute/update', body);
}

export function deleteNodeRouteJava(routerId: string): Promise<void> {
  return javaPost<void>('nodeRoute/delete', { routerId });
}

export function refreshNodeRouteJava(routerId: string): Promise<NodeRouterJava> {
  return javaPost<NodeRouterJava>('nodeRoute/refresh', { routerId }).then((n) => n || {});
}

/** Candidate cooperative nodes (CENTER/EDGE). */
export function listRouteNodesJava(): Promise<NodeDetailJava[]> {
  return javaPost('nodeRoute/listNode', {}).then((p) => asList<NodeDetailJava>(p));
}

/** CENTER/EDGE cooperative node request goes through approval (voteType NODE_ROUTE). */
export function createNodeRouteApprovalJava(initiatorId: string, voteConfig: NodeRouteVoteConfigJava): Promise<unknown> {
  return javaPost('approval/create', { initiatorId, voteType: 'NODE_ROUTE', voteConfig });
}

// ------------------------------ p2p node ------------------------------

export function createP2pNodeJava(body: P2pCreateNodeRequestJava): Promise<string> {
  return javaPost<string>('p2p/node/create', body);
}

export function deleteP2pNodeJava(routerId: string): Promise<void> {
  return javaPost<void>('p2p/node/delete', { routerId });
}

// --------------------------- edge account pwd ---------------------------

/**
 * Reset a node (edge) account password: Java RemoteUserController, used on
 * every platform (user/node/resetPassword is inner-port only).
 */
export function resetRemoteUserPwdJava(body: ResetNodeUserPwdRequestJava): Promise<string> {
  return javaPost<string>('user/remote/resetPassword', body);
}
