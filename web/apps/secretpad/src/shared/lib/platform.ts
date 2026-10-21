import { useQuery } from '@tanstack/react-query';
import { listMyInstNodesJava } from '@secretpad/api-client';
import { useAuthStore } from '@/features/auth/model/auth-store';

export enum Platform {
  CENTER = 'CENTER',
  EDGE = 'EDGE',
  AUTONOMY = 'AUTONOMY',
  TEST = 'TEST',
  P2P = 'P2P',
}

export enum PadMode {
  TEE = 'TEE',
  MPC = 'MPC',
  ALL_IN_ONE = 'ALL-IN-ONE',
}

export interface AccessType {
  types?: Platform[];
  modes?: PadMode[];
}

export const EMBEDDED_NODES = ['alice', 'bob', 'tee'];

export const ALL_PLATFORMS = [Platform.CENTER, Platform.EDGE, Platform.AUTONOMY, Platform.P2P, Platform.TEST];
export const ALL_MODES = [PadMode.ALL_IN_ONE, PadMode.MPC, PadMode.TEE];

/** Minimal user shape the permission helpers need (pure, testable). */
export interface PlatformUser {
  platformType?: string;
  ownerType?: string;
  ownerId?: string;
  platformNodeId?: string;
  deployMode?: string;
}

export interface PlatformContext {
  platformType: Platform;
  ownerType: string;
  ownerId: string;
  deployMode: PadMode;
}

function normalizeMode(mode?: string): PadMode {
  const m = (mode || '').toUpperCase();
  return (ALL_MODES as string[]).includes(m) ? (m as PadMode) : PadMode.ALL_IN_ONE;
}

export function toPlatformContext(user: PlatformUser | null | undefined, fallbackType?: string, fallbackNode?: string): PlatformContext {
  const platformType = ((user?.platformType || fallbackType || Platform.CENTER).toUpperCase() as Platform) || Platform.CENTER;
  return {
    platformType,
    ownerType: (user?.ownerType || 'CENTER').toUpperCase(),
    ownerId: user?.ownerId || user?.platformNodeId || fallbackNode || '',
    deployMode: normalizeMode(user?.deployMode),
  };
}

/** CENTER platform, but logged in with an EDGE (node) account. */
export function isEdgeAccountOnCenter(ctx: PlatformContext): boolean {
  return ctx.platformType === Platform.CENTER && ctx.ownerType === 'EDGE';
}

/**
 * Built-in nodes (alice/bob/tee) can be entered directly only by a CENTER
 * account on a CENTER platform (legacy edge-auth / p2p-edge-center-auth).
 */
export function canAccessEmbeddedNode(ctx: PlatformContext, nodeId?: string): boolean {
  return (
    ctx.platformType === Platform.CENTER && ctx.ownerType === 'CENTER' && !!nodeId && EMBEDDED_NODES.includes(nodeId)
  );
}

/** Whether the current account may enter the node-context view of `nodeId`. */
export function canEnterNode(ctx: PlatformContext, nodeId?: string): boolean {
  if (!nodeId) return false;
  switch (ctx.platformType) {
    case Platform.CENTER:
      // CENTER admins manage every node (built-in ones included); EDGE accounts on CENTER only their own.
      return ctx.ownerType === 'CENTER' || nodeId === ctx.ownerId;
    case Platform.EDGE:
    case Platform.AUTONOMY:
    case Platform.P2P:
      return nodeId === ctx.ownerId;
    case Platform.TEST:
      return true;
    default:
      return false;
  }
}

/**
 * Node shown on `/p2p/my-node` (legacy edge-auth / p2p-edge-center-auth).
 * A CENTER admin owns no node (its ownerId is the platform id, e.g.
 * `kuscia-system`, which node/get rejects), so it may open my-node only for a
 * built-in node passed as `?ownerId=`; `null` means "not allowed, redirect".
 * Every other account sees its own node (AUTONOMY switches among its
 * institution nodes on the page itself).
 */
export function resolveMyNodeId(ctx: PlatformContext, requested?: string): string | null {
  if (ctx.platformType === Platform.CENTER && ctx.ownerType === 'CENTER') {
    return requested && canAccessEmbeddedNode(ctx, requested) ? requested : null;
  }
  if (ctx.platformType === Platform.TEST) return requested || ctx.ownerId;
  return ctx.ownerId;
}

/**
 * What the caller knows about the target node, used to widen the AUTONOMY
 * write scope: there `ownerId` is the institution id while node pickers hand
 * out node ids.
 */
export interface NodeWriteScope {
  /** `NodeVO.instId` of the target node (node/list, node/get, inst/node/list). */
  instId?: string;
  /** Node ids of the account's institution (`inst/node/list`). */
  instNodeIds?: readonly string[];
}

/**
 * Write permission on node-owned resources (datasources, datatables, routes…),
 * mirroring the Go/Java DataResourceAuth NODE_ID rule
 * (`ownerId == nodeId || node.inst_id == ownerId`):
 * - CENTER admins and TEST can write everywhere;
 * - everybody else on their own node, plus — for AUTONOMY, whose ownerId is the
 *   institution id — every node that belongs to their institution.
 * Without a nodeId the check degrades to "is not a read-only account".
 */
export function canWriteNode(ctx: PlatformContext, nodeId?: string, scope?: NodeWriteScope): boolean {
  if (ctx.platformType === Platform.TEST) return true;
  if (ctx.platformType === Platform.CENTER && ctx.ownerType === 'CENTER') return true;
  if (!nodeId) return true;
  if (nodeId === ctx.ownerId) return true;
  if (!ctx.ownerId) return false;
  if (scope?.instId && scope.instId === ctx.ownerId) return true;
  return ctx.platformType === Platform.AUTONOMY && !!scope?.instNodeIds?.includes(nodeId);
}

export function hasAccess(ctx: PlatformContext, access: AccessType = {}): boolean {
  const types = access.types ?? ALL_PLATFORMS;
  const modes = access.modes ?? ALL_MODES;
  return types.includes(ctx.platformType) && modes.includes(ctx.deployMode);
}

/** TEE features (push to TEE, TEE nodes, TEE download) are visible only in TEE / ALL-IN-ONE deployments. */
export function supportsTee(ctx: PlatformContext): boolean {
  return ctx.deployMode === PadMode.TEE || ctx.deployMode === PadMode.ALL_IN_ONE;
}

/** MPC features are hidden in pure TEE deployments. */
export function supportsMpc(ctx: PlatformContext): boolean {
  return ctx.deployMode === PadMode.MPC || ctx.deployMode === PadMode.ALL_IN_ONE;
}

export function usePlatformContext(): PlatformContext {
  const { user, platform } = useAuthStore();
  return toPlatformContext(user, platform.platformType, platform.nodeId);
}

export function usePlatform() {
  const ctx = usePlatformContext();
  const { platformType, ownerId, deployMode, ownerType } = ctx;
  return {
    platformType,
    ownerType,
    ownerId,
    deployMode,
    isCenter: platformType === Platform.CENTER,
    isEdge: platformType === Platform.EDGE,
    isAutonomy: platformType === Platform.AUTONOMY,
    isTest: platformType === Platform.TEST,
    /** P2P-style (decentralised) deployments: P2P and AUTONOMY. */
    isP2p: platformType === Platform.P2P || platformType === Platform.AUTONOMY,
    /** EDGE account logged into a CENTER platform (node management hidden). */
    isEdgeAccountOnCenter: isEdgeAccountOnCenter(ctx),
    /** CENTER platform with a CENTER admin account. */
    isCenterAdmin: platformType === Platform.CENTER && ownerType === 'CENTER',
    supportsTee: supportsTee(ctx),
    supportsMpc: supportsMpc(ctx),
    canWriteNode: (nodeId?: string, scope?: NodeWriteScope) => canWriteNode(ctx, nodeId, scope),
    canEnterNode: (nodeId?: string) => canEnterNode(ctx, nodeId),
  };
}

export function useHasAccess(access: AccessType = {}): boolean {
  return hasAccess(usePlatformContext(), access);
}

export function useCanAccessEmbeddedNode(nodeId?: string): boolean {
  return canAccessEmbeddedNode(usePlatformContext(), nodeId);
}

/**
 * Node ids of the AUTONOMY account's institution (`inst/node/list`); empty
 * elsewhere. Shares the `my-inst-nodes` cache with `/p2p/my-node`.
 */
export function useInstNodeIds(): readonly string[] {
  const ctx = usePlatformContext();
  const q = useQuery({
    queryKey: ['my-inst-nodes'],
    queryFn: listMyInstNodesJava,
    enabled: ctx.platformType === Platform.AUTONOMY,
    staleTime: 60_000,
  });
  const data = ctx.platformType === Platform.AUTONOMY ? q.data : undefined;
  return (data || []).map((n) => n.nodeId || '').filter(Boolean);
}

/** Write permission hook — see {@link canWriteNode}; AUTONOMY resolves institution nodes itself. */
export function useCanWrite(nodeId?: string, instId?: string): boolean {
  const instNodeIds = useInstNodeIds();
  return canWriteNode(usePlatformContext(), nodeId, { instId, instNodeIds });
}
